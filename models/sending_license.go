package models

import (
	"errors"
	"time"
)

// ErrSendingLicenseRequired is safe to display to operators. Do not expose
// signing, installation or activation details through campaign/test-email APIs.
var ErrSendingLicenseRequired = errors.New("Activate a valid DarkPhish license in Settings > Licensing before launching campaigns or sending test emails. Contact your administrator if you cannot access Licensing.")

// CheckSendingLicense is fail-closed even before runtime initialization. Unlike
// offline model compatibility helpers, no sending path may accept a nil manager.
// A signed offline grace period remains usable under the existing lease policy.
func CheckSendingLicense() error {
	manager := currentLicenseManager()
	if manager == nil {
		return ErrSendingLicenseRequired
	}
	_, state, err := manager.Snapshot(time.Now().UTC())
	if err != nil || !state.AllowsExpansion() {
		return ErrSendingLicenseRequired
	}
	return nil
}
