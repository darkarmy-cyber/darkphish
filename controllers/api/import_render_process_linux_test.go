//go:build linux

package api

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRenderedCgroupRootMustStayWithinCgroupMount(t *testing.T) {
	mount := t.TempDir()
	inside := filepath.Join(mount, "system.slice", "darkphish.service")
	if got, err := validatedRenderedCgroupRoot(mount, inside); err != nil || got != inside {
		t.Fatalf("delegated root = %q, %v; want %q", got, err, inside)
	}
	if _, err := validatedRenderedCgroupRoot(mount, filepath.Join(mount, "..", "other")); err == nil {
		t.Fatal("cgroup root outside the cgroup mount was accepted")
	}
}

func TestRenderedCgroupRootSurvivesServiceLeafReexec(t *testing.T) {
	mount := t.TempDir()
	delegated := filepath.Join(mount, "system.slice", "darkphish.service")
	for _, suffix := range []string{
		renderedServiceCgroupLeaf,
		renderedServiceCgroupLeaf + "/" + renderedServiceCgroupLeaf,
	} {
		proc := []byte("0::/system.slice/darkphish.service/" + suffix + "\n")
		got, err := renderedCgroupRootFromProc(mount, proc)
		if err != nil || got != delegated {
			t.Fatalf("delegated root for %q = %q, %v; want %q", suffix, got, err, delegated)
		}
	}
}

func TestRenderedCgroupRequiresDelegatedResourceControllers(t *testing.T) {
	root := t.TempDir()
	controllers := filepath.Join(root, "cgroup.controllers")
	subtreeControl := filepath.Join(root, "cgroup.subtree_control")
	if err := os.WriteFile(controllers, []byte("cpu memory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subtreeControl, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cgroup.procs"), []byte("123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareRenderedCgroupRoot(root); !errors.Is(err, errRenderedImportUnavailable) {
		t.Fatalf("renderer accepted a subtree without delegated PID control: %v", err)
	}
	if err := os.WriteFile(controllers, []byte("cpu memory pids"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareRenderedCgroupRoot(root); err != nil {
		t.Fatalf("renderer rejected delegated CPU, memory, and PID controls: %v", err)
	}
	leafProcess, err := os.ReadFile(filepath.Join(root, "darkphish-main", "cgroup.procs"))
	if err != nil || string(leafProcess) != "123" {
		t.Fatalf("service process was not moved to the stable leaf: %q, %v", leafProcess, err)
	}
	if !cgroupHasControllers(subtreeControl, "cpu", "memory", "pids") {
		t.Fatal("delegated controllers were not enabled for renderer children")
	}
	if got := renderedCPUMax(); got != "40000 100000" {
		t.Fatalf("cpu.max = %q, want 40000 100000", got)
	}
}

func TestRenderedCommandStartsInsideResourceCgroupOrFailsClosed(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	cleanup, err := startRenderedCommand(cmd)
	if err != nil {
		if !errors.Is(err, errRenderedImportUnavailable) || cmd.Process != nil {
			t.Fatalf("resource containment did not fail closed: process=%v err=%v", cmd.Process, err)
		}
		return
	}
	defer func() { _ = cleanup() }()
	cgroup, err := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/cgroup")
	if err != nil || !strings.Contains(string(cgroup), "darkphish-render-") {
		_ = cmd.Cancel()
		_ = cmd.Wait()
		t.Fatalf("renderer was not created inside its resource cgroup: %q err=%v", cgroup, err)
	}
	_ = cmd.Cancel()
	_ = cmd.Wait()
}

func TestRenderedPIDLimitCoversChromiumAndStaysBounded(t *testing.T) {
	// The pids controller counts threads. Chromium 158 holds 140 before the
	// first DevTools command and aborted with SIGTRAP under the earlier limit
	// of 128. The limit keeps at least twice the measured peak of real pages
	// and stays a bound on what a renderer can spawn.
	if maxRenderedPIDs < 2*chromiumMeasuredPeakThreads {
		t.Fatalf("renderer PID limit %d leaves no headroom above the measured %d threads", maxRenderedPIDs, chromiumMeasuredPeakThreads)
	}
	if maxRenderedPIDs > 1024 {
		t.Fatalf("unexpected renderer PID limit: %d", maxRenderedPIDs)
	}
}
