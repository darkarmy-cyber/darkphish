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

func startRenderedCommand(cmd *exec.Cmd) (func() error, error) {
	// Rendered imports are optional and fail closed unless the service has a
	// delegated, writable cgroup v2 subtree with the memory controller enabled.
	const cgroupRoot = "/sys/fs/cgroup"
	controllers, err := os.ReadFile(filepath.Join(cgroupRoot, "cgroup.controllers"))
	hasMemoryController := false
	for _, controller := range strings.Fields(string(controllers)) {
		if controller == "memory" {
			hasMemoryController = true
			break
		}
	}
	if err != nil || !hasMemoryController {
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
