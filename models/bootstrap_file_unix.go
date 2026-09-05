//go:build !windows

package models

import "os"

func createPrivateBootstrapFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
