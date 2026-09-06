package models

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/persistence"
	"github.com/pressly/goose/v3"
)

func TestSetupRejectsPermanentTrustErrorsWithoutRetry(t *testing.T) {
	previousDB, previousConfig, previousStore := db, conf, secretStore
	t.Cleanup(func() {
		db, conf, secretStore = previousDB, previousConfig, previousStore
		if previousConfig != nil {
			_ = goose.SetDialect(previousConfig.DBName)
		}
	})
	invalid := filepath.Join(t.TempDir(), "invalid-ca.pem")
	if err := os.WriteFile(invalid, []byte("synthetic invalid CA"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing-ca.pem"), invalid} {
		started := time.Now()
		err := Setup(&config.Config{DBName: "mysql", DBPath: "synthetic-sensitive-connection-string", DBSSLCaPath: path})
		if !errors.Is(err, persistence.ErrTrust) || err.Error() != persistence.ErrTrust.Error() {
			t.Fatal("permanent trust failure lost its secret-free classification")
		}
		if time.Since(started) >= 3*time.Second {
			t.Fatal("permanent trust configuration entered the connection retry delay")
		}
	}
}
