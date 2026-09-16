package main

import (
	"errors"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"github.com/darkarmy-cyber/darkphish/models"
	"path/filepath"
	"testing"
)

func TestUnconfiguredStartupEnforcesMissingLicense(t *testing.T) {
	stop, err := configureLicensing(&config.Config{License: config.LicenseConfig{StatePath: filepath.Join(t.TempDir(), "state.json")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(); models.ConfigureLicenseManager(nil); models.ConfigureLicenseClient(nil) })
	if err := models.CheckCampaignLicense(); !errors.Is(err, licensing.ErrActiveCampaignLimitReached) {
		t.Fatalf("unconfigured startup disabled enforcement: %v", err)
	}
}

func TestStartupRejectsIncompleteLicenseConfiguration(t *testing.T) {
	for _, cfg := range []config.LicenseConfig{
		{ServiceURL: "https://license.example.test"},
		{KeyringFile: "missing.json"},
		{RefreshIntervalSeconds: 1},
		{ServiceURL: "https://license.example.test", KeyringFile: "missing.json"},
	} {
		if stop, err := configureLicensing(&config.Config{License: cfg}); err == nil {
			stop()
			t.Fatal("invalid license configuration accepted")
		}
	}
}
