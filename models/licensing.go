package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"gorm.io/gorm"
)

var (
	licenseManagerMu sync.RWMutex
	licenseManager   *licensing.Manager
)

// ConfigureLicenseManager installs the process-wide entitlement source. A nil
// manager disables enforcement for compatibility tooling and tests; official
// Community startup configures a manager before serving requests.
func ConfigureLicenseManager(manager *licensing.Manager) {
	licenseManagerMu.Lock()
	defer licenseManagerMu.Unlock()
	licenseManager = manager
}

func currentLicenseManager() *licensing.Manager {
	licenseManagerMu.RLock()
	defer licenseManagerMu.RUnlock()
	return licenseManager
}

func enforceGroupLicense(tx *gorm.DB, group *Group) error {
	manager := currentLicenseManager()
	if manager == nil {
		return nil
	}
	lease, state, verifyErr := manager.Snapshot(time.Now().UTC())
	if verifyErr != nil && state != licensing.StateInvalid {
		return fmt.Errorf("verify Community license: %w", verifyErr)
	}
	existing, err := managedUserEmailsExcludingGroup(tx, group.Id)
	if err != nil {
		return fmt.Errorf("count managed users: %w", err)
	}
	proposed := make([]string, 0, len(group.Targets))
	for _, target := range group.Targets {
		proposed = append(proposed, target.Email)
	}
	if err := licensing.EnforceManagedUsers(state, lease.Entitlements.ManagedUsers, existing, proposed); err != nil {
		return err
	}
	return nil
}

func managedUserEmailsExcludingGroup(tx *gorm.DB, excludedGroupID int64) ([]string, error) {
	values := []string{}
	query := tx.Table("targets").
		Distinct("targets.email").
		Joins("JOIN group_targets gt ON gt.target_id = targets.id")
	if excludedGroupID > 0 {
		query = query.Where("gt.group_id <> ?", excludedGroupID)
	}
	if err := query.Pluck("targets.email", &values).Error; err != nil {
		return nil, err
	}
	return values, nil
}

func enforceCampaignLicense(tx *gorm.DB) error {
	manager := currentLicenseManager()
	if manager == nil {
		return nil
	}
	lease, state, verifyErr := manager.Snapshot(time.Now().UTC())
	if verifyErr != nil && state != licensing.StateInvalid {
		return fmt.Errorf("verify Community license: %w", verifyErr)
	}
	var active int64
	if err := tx.Table("campaigns").Where("status <> ?", CampaignComplete).Count(&active).Error; err != nil {
		return fmt.Errorf("count active campaigns: %w", err)
	}
	return licensing.EnforceActiveCampaigns(state, lease.Entitlements.ActiveCampaigns, int(active), true)
}
