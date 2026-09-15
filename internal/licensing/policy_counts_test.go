package licensing

import (
	"errors"
	"testing"
)

func TestManagedUserCountGrandfathering(t *testing.T) {
	if err := EnforceManagedUserCounts(StateActive, 100, 120, 110); err != nil {
		t.Fatalf("120 -> 110 reduction rejected: %v", err)
	}
	if err := EnforceManagedUserCounts(StateActive, 100, 120, 120); err != nil {
		t.Fatalf("retaining existing over-limit footprint rejected: %v", err)
	}
	if err := EnforceManagedUserCounts(StateActive, 100, 120, 121); !errors.Is(err, ErrManagedUserLimitReached) {
		t.Fatalf("120 -> 121 err=%v want limit error", err)
	}
	if err := EnforceManagedUserCounts(StateActive, 100, 99, 100); err != nil {
		t.Fatalf("99 -> 100 rejected: %v", err)
	}
	if err := EnforceManagedUserCounts(StateActive, 100, 100, 101); !errors.Is(err, ErrManagedUserLimitReached) {
		t.Fatalf("100 -> 101 err=%v want limit error", err)
	}
}
