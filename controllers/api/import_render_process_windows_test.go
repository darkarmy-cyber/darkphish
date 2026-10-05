//go:build windows

package api

import (
	"os/exec"
	"testing"
)

func TestRenderedImportFailsClosedOnWindows(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	configureRenderedCommand(cmd)
	if cleanup, err := startRenderedCommand(cmd); err == nil || cleanup != nil {
		t.Fatal("Windows renderer unexpectedly enabled without bounded profile storage")
	}
}
