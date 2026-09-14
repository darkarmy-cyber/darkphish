//go:build !linux

package main

import (
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/update"
)

func superviseUpdates(_ *config.Config) (bool, error) { return false, nil }

func recoverPendingUpdate() (bool, error) { return false, nil }
func configureUpdates(_ *config.Config) *update.Service {
	return update.NewService(semanticVersion(), "One-click update supports Linux amd64/arm64 with SQLite only", nil)
}
