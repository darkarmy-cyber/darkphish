//go:build linux

package update

import (
	"errors"
	"syscall"
)

func validateFileMetadata(path string) error {
	return validateCapabilities(path, syscall.Getxattr)
}

func validateCapabilities(path string, getAttribute func(string, string, []byte) (int, error)) error {
	size, err := getAttribute(path, "security.capability", nil)
	if errors.Is(err, syscall.ENODATA) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	if err != nil {
		return err
	}
	if size > 0 {
		return errors.New("file capabilities require a manual update; use systemd ambient capabilities for supervised updates")
	}
	return nil
}
