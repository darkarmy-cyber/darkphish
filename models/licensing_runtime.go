package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/licensing"
)

var (
	licenseExchangeMu sync.Mutex
	licenseClientMu   sync.RWMutex
	licenseClient     *licensing.Client
)

var ErrLicensingNotConfigured = errors.New("Licensing is not configured")

type LicenseStatus struct {
	Configured          bool   `json:"configured"`
	Edition             string `json:"edition"`
	State               string `json:"state"`
	LicenseID           string `json:"license_id,omitempty"`
	InstallationID      string `json:"installation_id,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
	GraceUntil          int64  `json:"grace_until,omitempty"`
	ManagedUsers        int    `json:"managed_users"`
	ManagedUsersLimit   int    `json:"managed_users_limit"`
	ActiveCampaigns     int    `json:"active_campaigns"`
	ActiveCampaignLimit int    `json:"active_campaigns_limit"`
}

func ConfigureLicenseClient(client *licensing.Client) {
	licenseClientMu.Lock()
	defer licenseClientMu.Unlock()
	licenseClient = client
}

func currentLicenseClient() *licensing.Client {
	licenseClientMu.RLock()
	defer licenseClientMu.RUnlock()
	return licenseClient
}

func GetLicenseStatus(now time.Time) (LicenseStatus, error) {
	manager := currentLicenseManager()
	if manager == nil {
		return LicenseStatus{Edition: licensing.EditionCommunity, State: string(licensing.StateMissing)}, ErrLicensingNotConfigured
	}
	lease, state, verifyErr := manager.Snapshot(now.UTC())
	status := LicenseStatus{
		Configured:          currentLicenseClient() != nil,
		Edition:             lease.Edition,
		State:               string(state),
		LicenseID:           lease.LicenseID,
		InstallationID:      manager.InstallationID(),
		ExpiresAt:           lease.ExpiresAt,
		GraceUntil:          lease.GraceUntil,
		ManagedUsersLimit:   lease.Entitlements.ManagedUsers,
		ActiveCampaignLimit: lease.Entitlements.ActiveCampaigns,
	}
	currentEmails, err := managedUserEmailsExcludingGroup(db, 0)
	if err != nil {
		return status, fmt.Errorf("count managed users: %w", err)
	}
	status.ManagedUsers = licensing.ProjectManagedUsers(currentEmails, nil)
	var active int64
	if err := db.Table("campaigns").Where("status <> ?", CampaignComplete).Count(&active).Error; err != nil {
		return status, fmt.Errorf("count active campaigns: %w", err)
	}
	status.ActiveCampaigns = int(active)
	if verifyErr != nil && state != licensing.StateInvalid {
		return status, verifyErr
	}
	return status, nil
}

func ActivateCommunityLicense(ctx context.Context, licenseKey string, now time.Time) (LicenseStatus, error) {
	licenseExchangeMu.Lock()
	defer licenseExchangeMu.Unlock()
	manager := currentLicenseManager()
	client := currentLicenseClient()
	if manager == nil || client == nil {
		return LicenseStatus{}, ErrLicensingNotConfigured
	}
	response, err := client.Activate(ctx, licenseKey, manager.InstallationID())
	if err != nil {
		return LicenseStatus{}, err
	}
	if _, _, err := manager.InstallLease(response.Lease, response.RefreshToken, now.UTC()); err != nil {
		return LicenseStatus{}, fmt.Errorf("verify activation lease: %w", err)
	}
	return GetLicenseStatus(now)
}

func RefreshCommunityLicense(ctx context.Context, now time.Time) (LicenseStatus, error) {
	licenseExchangeMu.Lock()
	defer licenseExchangeMu.Unlock()
	manager := currentLicenseManager()
	client := currentLicenseClient()
	if manager == nil || client == nil {
		return LicenseStatus{}, ErrLicensingNotConfigured
	}
	refreshToken := manager.RefreshToken()
	if refreshToken == "" {
		return LicenseStatus{}, errors.New("Community license has not been activated")
	}
	response, err := client.Refresh(ctx, refreshToken, manager.InstallationID())
	if err != nil {
		return LicenseStatus{}, err
	}
	if strings.TrimSpace(response.RefreshToken) == "" {
		response.RefreshToken = refreshToken
	}
	if _, _, err := manager.InstallLease(response.Lease, response.RefreshToken, now.UTC()); err != nil {
		return LicenseStatus{}, fmt.Errorf("verify refreshed lease: %w", err)
	}
	return GetLicenseStatus(now)
}
