//go:build linux

package api

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	renderedCgroupRootOnce sync.Once
	renderedCgroupRootPath string
	renderedCgroupRootErr  error
	renderedCgroupSetupMu  sync.Mutex
)

const renderedServiceCgroupLeaf = "darkphish-main"

func configureRenderedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

func renderedCgroupRoot() (string, error) {
	renderedCgroupRootOnce.Do(func() {
		renderedCgroupRootPath, renderedCgroupRootErr = discoverRenderedCgroupRoot()
	})
	return renderedCgroupRootPath, renderedCgroupRootErr
}

func discoverRenderedCgroupRoot() (string, error) {
	const cgroupMount = "/sys/fs/cgroup"
	configured := strings.TrimSpace(os.Getenv("DARKPHISH_RENDER_CGROUP_ROOT"))
	if configured != "" {
		return validatedRenderedCgroupRoot(cgroupMount, configured)
	}
	selfCgroup, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", errRenderedImportUnavailable
	}
	return renderedCgroupRootFromProc(cgroupMount, selfCgroup)
}

func renderedCgroupRootFromProc(cgroupMount string, selfCgroup []byte) (string, error) {
	for _, line := range strings.Split(string(selfCgroup), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" {
			continue
		}
		candidate := filepath.Join(cgroupMount, strings.TrimPrefix(parts[2], "/"))
		// The application and update supervisor are moved into this stable leaf
		// before controllers are enabled. A supervisor exec loses in-process
		// caches, so recover the original delegated boundary from the leaf path.
		for filepath.Base(candidate) == renderedServiceCgroupLeaf {
			candidate = filepath.Dir(candidate)
		}
		return validatedRenderedCgroupRoot(cgroupMount, candidate)
	}
	return "", errRenderedImportUnavailable
}

func validatedRenderedCgroupRoot(cgroupMount, candidate string) (string, error) {
	mount, err := filepath.Abs(cgroupMount)
	if err != nil {
		return "", errRenderedImportUnavailable
	}
	root, err := filepath.Abs(candidate)
	if err != nil {
		return "", errRenderedImportUnavailable
	}
	relative, err := filepath.Rel(mount, root)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errRenderedImportUnavailable
	}
	return root, nil
}

func cgroupHasControllers(path string, required ...string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	controllers := make(map[string]bool)
	for _, controller := range strings.Fields(string(data)) {
		controllers[strings.TrimPrefix(controller, "+")] = true
	}
	for _, controller := range required {
		if !controllers[controller] {
			return false
		}
	}
	return true
}

func prepareRenderedCgroupRoot(cgroupRoot string) error {
	renderedCgroupSetupMu.Lock()
	defer renderedCgroupSetupMu.Unlock()

	required := []string{"cpu", "memory", "pids"}
	subtreeControl := filepath.Join(cgroupRoot, "cgroup.subtree_control")
	if cgroupHasControllers(subtreeControl, required...) {
		return nil
	}
	if !cgroupHasControllers(filepath.Join(cgroupRoot, "cgroup.controllers"), required...) {
		return errRenderedImportUnavailable
	}

	// cgroup v2 does not allow domain controllers on a populated non-root
	// cgroup. Keep the delegated service root empty and run the service itself
	// in a stable leaf before enabling controllers for renderer siblings.
	serviceLeaf := filepath.Join(cgroupRoot, renderedServiceCgroupLeaf)
	if err := os.Mkdir(serviceLeaf, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return errRenderedImportUnavailable
	}
	processes, err := os.ReadFile(filepath.Join(cgroupRoot, "cgroup.procs"))
	if err != nil {
		return errRenderedImportUnavailable
	}
	for _, pid := range strings.Fields(string(processes)) {
		if err := os.WriteFile(filepath.Join(serviceLeaf, "cgroup.procs"), []byte(pid), 0o600); err != nil {
			return errRenderedImportUnavailable
		}
	}
	if err := os.WriteFile(subtreeControl, []byte("+cpu +memory +pids"), 0o600); err != nil {
		return errRenderedImportUnavailable
	}
	if !cgroupHasControllers(subtreeControl, required...) {
		return errRenderedImportUnavailable
	}
	return nil
}

func renderedCPUMax() string {
	return strconv.FormatUint(uint64(maxRenderedCPUPercent()*1000), 10) + " 100000"
}

func startRenderedCommand(cmd *exec.Cmd) (func() error, error) {
	// Rendered imports are optional and fail closed unless the service has a
	// delegated, writable cgroup v2 subtree with memory, CPU, and PID controllers.
	cgroupRoot, err := renderedCgroupRoot()
	if err != nil || prepareRenderedCgroupRoot(cgroupRoot) != nil {
		return nil, errRenderedImportUnavailable
	}
	cgroup, err := os.MkdirTemp(cgroupRoot, "darkphish-render-")
	if err != nil {
		return nil, errRenderedImportUnavailable
	}
	removeCgroup := func() error { return os.Remove(cgroup) }
	fail := func() (func() error, error) {
		_ = removeCgroup()
		return nil, errRenderedImportUnavailable
	}
	if err := os.WriteFile(filepath.Join(cgroup, "memory.max"), []byte(strconv.FormatInt(maxRenderedMemoryBytes, 10)), 0o600); err != nil {
		return fail()
	}
	if err := os.WriteFile(filepath.Join(cgroup, "memory.swap.max"), []byte("0"), 0o600); err != nil {
		return fail()
	}
	if err := os.WriteFile(filepath.Join(cgroup, "memory.oom.group"), []byte("1"), 0o600); err != nil {
		return fail()
	}
	if err := os.WriteFile(filepath.Join(cgroup, "cpu.max"), []byte(renderedCPUMax()), 0o600); err != nil {
		return fail()
	}
	if err := os.WriteFile(filepath.Join(cgroup, "pids.max"), []byte(strconv.Itoa(maxRenderedPIDs)), 0o600); err != nil {
		return fail()
	}
	if _, err := os.Stat(filepath.Join(cgroup, "cgroup.kill")); err != nil {
		return fail()
	}
	cgroupDir, err := os.Open(cgroup)
	if err != nil {
		return fail()
	}
	configureRenderedCommand(cmd)
	cmd.SysProcAttr.UseCgroupFD = true
	cmd.SysProcAttr.CgroupFD = int(cgroupDir.Fd())
	if err := cmd.Start(); err != nil {
		_ = cgroupDir.Close()
		return fail()
	}
	_ = cgroupDir.Close()
	var cleanupOnce sync.Once
	var cleanupErr error
	cleanup := func() error {
		cleanupOnce.Do(func() {
			if err := os.WriteFile(filepath.Join(cgroup, "cgroup.kill"), []byte("1"), 0o600); err != nil {
				cleanupErr = err
				_ = cmd.Cancel()
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				events, err := os.ReadFile(filepath.Join(cgroup, "cgroup.events"))
				if err != nil {
					cleanupErr = err
					break
				}
				if strings.Contains(string(events), "populated 0") {
					break
				}
				if time.Now().After(deadline) {
					cleanupErr = errRenderedImportUnavailable
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := removeCgroup(); err != nil && cleanupErr == nil {
				cleanupErr = err
			}
		})
		return cleanupErr
	}
	return cleanup, nil
}
