//go:build !windows && !linux

package api

import (
	"os/exec"
)

func startRenderedCommand(_ *exec.Cmd) (func() error, error) {
	return nil, errRenderedImportUnavailable
}
