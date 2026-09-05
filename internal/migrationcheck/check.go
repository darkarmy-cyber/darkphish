package migrationcheck

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkarmy-cyber/darkphish/config"
)

type Finding struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Recommendation string `json:"recommendation"`
}

func Inspect(configPath string, conf *config.Config, legacyAPIKeys, plaintextSecrets int64) []Finding {
	findings := []Finding{}
	legacyNames := map[string]struct{}{}
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "GOPHISH_") {
			legacyNames[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(legacyNames))
	for name := range legacyNames {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		findings = append(findings, Finding{Code: "legacy_environment", Severity: "deprecated", Recommendation: "Replace " + name + " with the documented DARKPHISH_* equivalent; the value was not inspected or printed."})
	}
	if strings.EqualFold(filepath.Base(conf.DBPath), "gophish.db") {
		findings = append(findings, Finding{Code: "legacy_database_path", Severity: "deprecated", Recommendation: "Plan a controlled rename to darkphish.db while the service is stopped and update db_path."})
	}
	for _, certificate := range []string{conf.AdminConf.CertPath, conf.AdminConf.KeyPath, conf.PhishConf.CertPath, conf.PhishConf.KeyPath} {
		if strings.Contains(strings.ToLower(filepath.Base(certificate)), "gophish") {
			findings = append(findings, Finding{Code: "legacy_certificate_name", Severity: "deprecated", Recommendation: "Issue or rename the legacy Gophish-named certificate/key using explicit operator-controlled file operations."})
			break
		}
	}
	if conf.Secrets.EncryptionKey != "" {
		findings = append(findings, Finding{Code: "single_encryption_key", Severity: "deprecated", Recommendation: "Configure secrets.active_key_id plus a versioned keys map, then run darkphish secrets migrate."})
	}
	if legacyAPIKeys > 0 {
		findings = append(findings, Finding{Code: "legacy_api_key", Severity: "unsupported", Recommendation: "Create scoped expiring personal access tokens; legacy users.api_key values are not accepted for authentication."})
	}
	if plaintextSecrets > 0 {
		findings = append(findings, Finding{Code: "plaintext_secret", Severity: "migration_required", Recommendation: "Configure a local or Vault key provider and run darkphish secrets migrate before production use."})
	}
	configDir := filepath.Dir(configPath)
	for _, name := range []string{"gophish_admin.crt", "gophish_admin.key", "gophish.db"} {
		if _, err := os.Stat(filepath.Join(configDir, name)); err == nil {
			findings = append(findings, Finding{Code: "legacy_file", Severity: "deprecated", Recommendation: "Review legacy file " + name + "; this command will not rename or delete it."})
		}
	}
	return findings
}
