package databasetest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/persistence"
	"github.com/darkarmy-cyber/darkphish/models"
	"golang.org/x/crypto/bcrypt"
)

func migrationPath(t *testing.T, database string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve integration-test path")
	}
	return filepath.Join(filepath.Dir(file), "..", "db", "db_"+database, "migrations")
}

func exerciseSecurityModel(t *testing.T, database, dsn string) {
	t.Helper()
	conf := &config.Config{
		DBName: database, DBPath: dsn, MigrationsPath: migrationPath(t, database),
		BootstrapDirectory: t.TempDir(),
		DBMaxOpenConns:     5, DBMaxIdleConns: 2, DBConnMaxLifetimeMinutes: 5,
		Secrets: config.SecretsConfig{Provider: "local", ActiveKeyID: "CI", Keys: map[string]string{"CI": "0123456789abcdef0123456789abcdef"}},
		Audit:   config.AuditConfig{CheckpointInterval: 2, ActiveSigningKeyID: "CI", SigningKeys: map[string]string{"CI": "abcdef0123456789abcdef0123456789"}},
	}
	if err := models.Setup(conf); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = models.Close() })
	connection, err := sql.Open(persistence.DriverName(database), dsn)
	if err != nil {
		t.Fatal("open verification database")
	}
	t.Cleanup(func() { _ = connection.Close() })
	if database == "mysql" {
		var mode string
		if err := connection.QueryRow("SELECT @@SESSION.sql_mode").Scan(&mode); err != nil || !strings.Contains(mode, "STRICT_") {
			t.Fatal("MySQL integration requires strict SQL mode")
		}
	}
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	if admin.LastLogin != nil {
		t.Fatal("new administrator must have NULL last_login")
	}
	bootstrap, err := os.ReadFile(filepath.Join(conf.BootstrapDirectory, "darkphish_initial_admin_password"))
	if err != nil || auth.ValidatePassword(strings.TrimSpace(string(bootstrap)), admin.Hash) != nil {
		t.Fatal("backend-aware bootstrap password did not authenticate the new administrator")
	}
	password := "database-integration-password"
	admin.Hash, err = auth.GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	admin.PasswordChangeRequired = false
	if err := models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}
	models.RemoveInitialAdminPasswordFile()
	lastLogin := time.Now().UTC().Truncate(time.Second)
	admin.LastLogin = &lastLogin
	if err := models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}
	admin, err = models.GetUser(admin.Id)
	if err != nil || admin.LastLogin == nil || !admin.LastLogin.Equal(lastLogin) {
		t.Fatal("last_login did not survive a database round trip")
	}

	pat, raw, err := models.CreatePersonalAccessTokenWithAudit(admin.Id, "database integration", []string{"credentials:view", "campaigns:read"}, time.Now().UTC().Add(time.Hour), audit.Event{Result: "success", Metadata: "{}"})
	if err != nil || raw == "" || pat.TokenHash == "" {
		t.Fatalf("PAT persistence failed: raw=%t hash=%t err=%v", raw != "", pat.TokenHash != "", err)
	}
	if _, err := models.AuthenticatePersonalAccessToken(raw); err != nil {
		t.Fatalf("PAT authentication failed: %v", err)
	}
	if err := models.RevokePersonalAccessTokenWithAudit(pat.ID, admin.Id, audit.Event{Result: "success", Metadata: "{}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := models.AuthenticatePersonalAccessToken(raw); err == nil {
		t.Fatal("revoked PAT remained usable")
	}

	group := models.Group{Name: "database group", UserId: admin.Id, Targets: []models.Target{{BaseRecipient: models.BaseRecipient{Email: "database@example.test"}}}}
	exerciseGroupInputIsolation(t, admin.Id)
	if err := models.PostGroup(&group); err != nil {
		t.Fatal(err)
	}
	template := models.Template{Name: "database template", UserId: admin.Id, Subject: "test", Text: "test", HTML: "<p>test</p>"}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatal(err)
	}
	page := models.Page{Name: "database page", UserId: admin.Id, HTML: `<form><input type="password"></form>`, CaptureCredentials: true, CapturePasswords: true}
	if err := models.PostPage(&page); err != nil {
		t.Fatal(err)
	}
	smtp := models.SMTP{Name: "database smtp", UserId: admin.Id, Host: "example.test:25", FromAddress: "sender@example.test", Password: "Synthetic-integration-password-9!"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}
	for _, modified := range []time.Time{group.ModifiedDate, template.ModifiedDate, page.ModifiedDate, smtp.ModifiedDate} {
		if modified.IsZero() {
			t.Fatal("model-created records require real modification timestamps")
		}
	}
	campaign := models.Campaign{
		Name: "database campaign", UserId: admin.Id, Template: template, Page: page, SMTP: smtp, Groups: []models.Group{group},
		CredentialCaptureMode: models.CredentialModeEncryptedReview, CredentialRetentionHours: 24,
		CredentialPolicy: models.CredentialPolicy{MinLength: 12, MaxLength: 128, MinDigits: 1},
	}
	associationsBefore := associationFingerprint(t, connection)
	if err := models.PostCampaign(&campaign, admin.Id); err != nil {
		t.Fatal(err)
	}
	if associationFingerprint(t, connection) != associationsBefore {
		t.Fatal("campaign creation rewrote existing associations or protected SMTP bytes")
	}
	exerciseSummaryOwnership(t, campaign, group)
	credential := "Database-synthetic-credential-9!"
	if err := models.RecordCredentialSubmission(campaign, campaign.Results[0], credential); err != nil {
		t.Fatal(err)
	}
	reveal, err := models.RevealCredential(campaign.Id, campaign.Results[0].RId, admin.Id)
	if err != nil || reveal.Credential != credential {
		t.Fatalf("credential review failed: %v", err)
	}

	reviewerRole, err := models.GetRoleBySlug(models.RoleSecurityReviewer)
	if err != nil {
		t.Fatal(err)
	}
	reviewerHash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := models.User{Username: "database-reviewer", Hash: reviewerHash, ApiKey: "disabled-database-reviewer", Role: reviewerRole, RoleID: reviewerRole.ID}
	if err := models.PutUser(&reviewer); err != nil {
		t.Fatal(err)
	}
	reviewer, err = models.GetUser(reviewer.Id)
	if err != nil || reviewer.LastLogin != nil {
		t.Fatal("new reviewer must have NULL last_login")
	}
	encoded, err := json.Marshal(reviewer)
	if err != nil || !strings.Contains(string(encoded), `"last_login":null`) {
		t.Fatal("never-logged-in reviewer serialization is not nullable")
	}
	if allowed, err := models.CanReviewCredential(reviewer, campaign.Id, time.Now().UTC()); err != nil || allowed {
		t.Fatal("unassigned reviewer gained credential access")
	}
	if _, err := models.AssignCampaignReviewer(campaign.Id, reviewer.Id, admin, nil, audit.Event{Result: "success", Metadata: "{}"}); err != nil {
		t.Fatal(err)
	}
	allowed, err := models.CanReviewCredential(reviewer, campaign.Id, time.Now().UTC())
	if err != nil || !allowed {
		t.Fatalf("reviewer assignment not enforced: allowed=%v err=%v", allowed, err)
	}
	binding, err := models.NewSessionBinding()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.ReauthenticatePrivileged(context.Background(), reviewer, binding, models.ReauthenticationProof{Method: "password", Secret: password}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if fresh, err := models.IsPrivilegedSessionFresh(reviewer.Id, binding, time.Now().UTC()); err != nil || !fresh {
		t.Fatalf("privileged session is not fresh: %v %v", fresh, err)
	}
	if fresh, err := models.IsPrivilegedSessionFresh(reviewer.Id, binding, time.Now().UTC().Add(6*time.Minute)); err != nil || fresh {
		t.Fatal("privileged session did not expire")
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	reviewer.Hash = string(legacy)
	if err := models.PutUser(&reviewer); err != nil {
		t.Fatal(err)
	}
	if _, err := models.ReauthenticatePrivileged(context.Background(), reviewer, binding, models.ReauthenticationProof{Method: "password", Secret: password}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	reviewer, err = models.GetUser(reviewer.Id)
	if err != nil || !strings.HasPrefix(reviewer.Hash, "$argon2id$") {
		t.Fatal("legacy bcrypt privileged authentication did not upgrade")
	}
	if _, err := models.RevealCredential(campaign.Id, campaign.Results[0].RId, reviewer.Id); err != nil {
		t.Fatal("assigned reviewer could not reveal encrypted credential")
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := connection.QueryRow("SELECT COUNT(*) FROM audit_outbox WHERE dispatched_at IS NULL").Scan(&pending); err != nil || pending != 0 {
		t.Fatal("audit outbox was not durably delivered")
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatalf("audit persistence verification failed: %v", err)
	}
	checkpoint, err := models.CreateAuditCheckpoint()
	if err != nil || checkpoint.Signature == "" {
		t.Fatal("signed checkpoint persistence failed")
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatalf("persisted checkpoint timestamp invalidated signature: %v", err)
	}
	stopFailure := rejectOutbox(t, connection, database)
	_, failedRaw, err := models.CreatePersonalAccessTokenWithAudit(admin.Id, "rollback integration", []string{"campaigns:read"}, time.Now().UTC().Add(time.Hour), audit.Event{Result: "success", Metadata: "{}"})
	if err == nil || failedRaw != "" {
		t.Fatal("failed transaction returned a raw PAT")
	}
	if err := models.RemoveCampaignReviewer(campaign.Id, reviewer.Id, admin, audit.Event{Result: "success", Metadata: "{}"}); err == nil {
		t.Fatal("reviewer removal succeeded despite outbox failure")
	}
	stopFailure()
	if allowed, err := models.CanReviewCredential(reviewer, campaign.Id, time.Now().UTC()); err != nil || !allowed {
		t.Fatal("failed reviewer removal did not roll back")
	}
	var tokens int
	if err := connection.QueryRow("SELECT COUNT(*) FROM personal_access_tokens WHERE name='rollback integration'").Scan(&tokens); err != nil || tokens != 0 {
		t.Fatal("failed PAT creation did not roll back")
	}
	exercisePersistenceContract(t, database, dsn, admin.Id)
	exerciseSecurityRollback(t, conf, connection, admin, reviewer, campaign, binding, credential)
}

func rejectOutbox(t *testing.T, db *sql.DB, backend string) func() {
	t.Helper()
	var create, drop []string
	switch backend {
	case "mysql":
		create = []string{"CREATE TRIGGER reject_test_outbox BEFORE INSERT ON audit_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected outbox failure'"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_outbox"}
	case "postgres":
		create = []string{"CREATE FUNCTION reject_test_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected outbox failure'; END; $$", "CREATE TRIGGER reject_test_outbox BEFORE INSERT ON audit_outbox FOR EACH ROW EXECUTE FUNCTION reject_test_outbox()"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_outbox ON audit_outbox", "DROP FUNCTION IF EXISTS reject_test_outbox()"}
	case "sqlite3":
		create = []string{"CREATE TRIGGER reject_test_outbox BEFORE INSERT ON audit_outbox BEGIN SELECT RAISE(ABORT, 'injected outbox failure'); END"}
		drop = []string{"DROP TRIGGER IF EXISTS reject_test_outbox"}
	default:
		t.Fatal("unknown failure-injection backend")
	}
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		for _, query := range drop {
			if _, err := db.Exec(query); err != nil {
				t.Error(fmt.Errorf("remove test trigger: %w", err))
			}
		}
		cleaned = true
	}
	t.Cleanup(cleanup)
	for _, query := range create {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("create failure-injection trigger: %v", err)
		}
	}
	return cleanup
}

func TestSQLiteSecurityModel(t *testing.T) {
	exerciseSecurityModel(t, "sqlite3", filepath.Join(t.TempDir(), "security.db"))
}

func TestMySQLSecurityModel(t *testing.T) {
	dsn := os.Getenv("DARKPHISH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("DARKPHISH_TEST_MYSQL_DSN is not configured")
	}
	exerciseSecurityModel(t, "mysql", dsn)
}

func TestPostgreSQLSecurityModel(t *testing.T) {
	dsn := os.Getenv("DARKPHISH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DARKPHISH_TEST_POSTGRES_DSN is not configured")
	}
	exerciseSecurityModel(t, "postgres", dsn)
}
