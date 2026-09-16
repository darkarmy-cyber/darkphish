package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/models"
)

// Missing service configuration leaves a real, unactivated manager installed.
// It must never select the nil-manager compatibility path used by offline tools.
func configureLicensing(c *config.Config) (func(), error) {
	if (c.License.ServiceURL == "") != (c.License.KeyringFile == "") {
		return nil, errors.New("license.service_url and license.keyring_file must be configured together")
	}
	interval := c.License.RefreshIntervalSeconds
	if interval == 0 {
		interval = 3600
	}
	if interval < 60 || interval > 3600 {
		return nil, errors.New("invalid license refresh interval")
	}
	var keys map[string]ed25519.PublicKey
	var client *licensing.Client
	var err error
	if c.License.KeyringFile != "" {
		keys, err = licensing.LoadTrustedKeyring(c.License.KeyringFile)
		if err != nil {
			return nil, err
		}
		client, err = licensing.NewClient(c.License.ServiceURL, semanticVersion(), nil)
		if err != nil {
			return nil, err
		}
	}
	manager, err := licensing.OpenManager(c.License.StatePath, keys, time.Minute, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	models.ConfigureLicenseManager(manager)
	models.ConfigureLicenseClient(client)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		licensing.RunRefresh(ctx, time.Duration(interval)*time.Second, func(ctx context.Context) {
			if client == nil || manager.RefreshToken() == "" {
				return
			}
			if _, err := models.RefreshCommunityLicense(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
				log.Warn("Community license refresh failed; retaining the last verified lease until its signed expiry")
			}
		})
	}()
	return func() { cancel(); <-done }, nil
}
