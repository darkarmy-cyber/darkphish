//go:build windows

package api

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRenderedCommandUsesSuspendedJobAssignment(t *testing.T) {
	cpu := renderedCPURateControl()
	if cpu.ControlFlags != jobObjectCPURateControlEnable|jobObjectCPURateControlHardCap || cpu.CPURate != 4000 {
		t.Fatalf("unexpected renderer CPU limit: %#v", cpu)
	}
	if aggregate := cpu.CPURate * maxConcurrentRenderedImports; aggregate > maxRenderedAggregateCPUPercent*100 {
		t.Fatalf("aggregate renderer CPU limit %d exceeds reserved budget", aggregate)
	}
	configured := exec.Command("cmd.exe", "/c", "exit", "0")
	configureRenderedCommand(configured)
	if configured.SysProcAttr == nil || configured.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("renderer is not created suspended before Job Object assignment")
	}

	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	cleanup, err := startRenderedCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
