//go:build linux

package api

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestRenderedCommandStartsInsideMemoryCgroupOrFailsClosed(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	cleanup, err := startRenderedCommand(cmd)
	if err != nil {
		if !errors.Is(err, errRenderedImportUnavailable) || cmd.Process != nil {
			t.Fatalf("memory containment did not fail closed: process=%v err=%v", cmd.Process, err)
		}
		return
	}
	defer func() { _ = cleanup() }()
	cgroup, err := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/cgroup")
	if err != nil || !strings.Contains(string(cgroup), "darkphish-render-") {
		_ = cmd.Cancel()
		_ = cmd.Wait()
		t.Fatalf("renderer was not created inside its memory cgroup: %q err=%v", cgroup, err)
	}
	_ = cmd.Cancel()
	_ = cmd.Wait()
}
