package licensing_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
)

func TestDistributedCommunityConfiguration(t *testing.T) {
	path, err := filepath.Abs("../../config.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := licensing.LoadTrustedKeyring(cfg.License.KeyringFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys["DP-COM-20260916-26f22bd9"] == nil {
		t.Fatal("distributed config does not load the supplied community trust anchor")
	}
	if _, err := licensing.NewClient(cfg.License.ServiceURL, "0.11.0-dev", nil); err != nil {
		t.Fatal(err)
	}
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "state.json"), keys, 0, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if manager.RefreshToken() != "" {
		t.Fatal("public configuration must not activate an installation")
	}
}
