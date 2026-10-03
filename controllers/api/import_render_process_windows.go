//go:build windows

package api

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func configureRenderedCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			_ = cmd.Process.Kill()
		}
		return nil
	}
}
