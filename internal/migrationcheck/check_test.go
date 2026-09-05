package migrationcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/config"
)

func TestInspectReportsLegacyHazardsWithoutSecretValues(t *testing.T) {
	secret := "must-never-appear-in-output"
	t.Setenv("GOPHISH_INITIAL_ADMIN_PASSWORD", secret)
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	if err := os.WriteFile(filepath.Join(directory, "gophish.db"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	conf := &config.Config{
		DBPath:    "gophish.db",
		AdminConf: config.AdminServer{CertPath: "gophish_admin.crt"},
		Secrets:   config.SecretsConfig{EncryptionKey: secret},
	}
	findings := Inspect(configPath, conf, 2, 3)
	if len(findings) < 6 {
		t.Fatalf("expected migration findings, got %#v", findings)
	}
	for _, finding := range findings {
		if strings.Contains(finding.Recommendation, secret) {
			t.Fatal("legacy environment secret was exposed")
		}
	}
}
