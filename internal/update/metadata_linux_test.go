//go:build linux

package update

import (
	"syscall"
	"testing"
)

func TestFileCapabilitiesAndUninspectableMetadataDisableApply(t *testing.T) {
	for _, tc := range []struct {
		size    int
		err     error
		allowed bool
	}{{20, nil, false}, {0, syscall.EACCES, false}, {0, syscall.ENODATA, true}, {0, syscall.ENOTSUP, true}} {
		err := validateCapabilities("darkphish", func(path, name string, value []byte) (int, error) {
			if path != "darkphish" || name != "security.capability" {
				t.Fatal("did not inspect executable capabilities")
			}
			return tc.size, tc.err
		})
		if (err == nil) != tc.allowed {
			t.Fatal("incorrect capability eligibility", tc, err)
		}
	}
}
