package models

import (
	"errors"
	"github.com/darkarmy-cyber/darkphish/internal/licensetest"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"testing"
)

func TestSendingRequiresVerifiedLicense(t *testing.T) {
	t.Cleanup(func() { ConfigureLicenseManager(nil) })
	ConfigureLicenseManager(nil)
	if !errors.Is(CheckSendingLicense(), ErrSendingLicenseRequired) {
		t.Fatal("nil manager permits sending")
	}
	for _, state := range []licensing.State{licensing.StateMissing, licensing.StateInvalid, licensing.StateExpired, licensing.StateActive, licensing.StateGrace} {
		t.Run(string(state), func(t *testing.T) {
			ConfigureLicenseManager(licensetest.Manager(t, state))
			if allowed := CheckSendingLicense() == nil; allowed != state.AllowsExpansion() {
				t.Fatalf("sending=%v state=%s", allowed, state)
			}
		})
	}
}
