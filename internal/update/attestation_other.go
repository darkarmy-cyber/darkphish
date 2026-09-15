//go:build !linux

package update

import "errors"

func AttestationSupport() error {
	return errors.New("one-click update supports Linux only")
}
