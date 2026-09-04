package models

import (
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
)

// CleanupSecurityRetention purges expired credential ciphertext and audit
// events older than the configured retention window. Policy findings remain.
func CleanupSecurityRetention(now time.Time) (int64, int64, error) {
	credentials, err := DeleteExpiredCredentialValues(now)
	if err != nil {
		return 0, 0, err
	}
	days := config.DefaultAuditRetentionDays
	if conf != nil && conf.Audit.RetentionDays > 0 {
		days = conf.Audit.RetentionDays
	}
	auditEvents, err := audit.DeleteBefore(now.UTC().Add(-time.Duration(days) * 24 * time.Hour))
	return credentials, auditEvents, err
}
