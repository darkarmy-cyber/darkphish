//go:build !windows

package licensing

import "os"

func createPrivateStateTemp(dir string) (*os.File, error) {
	return os.CreateTemp(dir, ".darkphish-license-*")
}
