//go:build windows

package api

import "os/exec"

// Rendered imports stay fail-closed on Windows until Darkphish has a
// verifiable, capacity-bounded profile filesystem equivalent to the Linux
// tmpfs requirement. Static Import Site remains available.
func configureRenderedCommand(*exec.Cmd) {}

func startRenderedCommand(*exec.Cmd) (func() error, error) {
	return nil, errRenderedImportUnavailable
}
