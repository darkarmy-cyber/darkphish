package databasetest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
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
		DBMaxOpenConns: 5, DBMaxIdleConns: 2, DBConnMaxLifetimeMinutes: 5,
		Secrets: config.SecretsConfig{Provider: "local", ActiveKeyID: "CI", Keys: map[string]string{"CI": "0123456789abcdef0123456789abcdef"}},
		Audit:   config.AuditConfig{CheckpointInterval: 2, ActiveSigningKeyID: "CI", SigningKeys: map[string]string{"CI": "abcdef0123456789abcdef0123456789"}},
	}
	if err := models.Setup(conf); err != nil {
		t.Fatal(err)
	}
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
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

	pat, raw, err := models.CreatePersonalAccessToken(admin.Id, "database integration", []string{"credentials:view", "campaigns:read"}, time.Now().UTC().Add(time.Hour))
	if err != nil || raw == "" || pat.TokenHash == "" {
		t.Fatalf("PAT persistence failed: raw=%t hash=%t err=%v", raw != "", pat.TokenHash != "", err)
	}
	if _, err := models.AuthenticatePersonalAccessToken(raw); err != nil {
		t.Fatalf("PAT authentication failed: %v", err)
	}

	group := models.Group{Name: "database group", UserId: admin.Id, Targets: []models.Target{{BaseRecipient: models.BaseRecipient{Email: "database@example.test"}}}}
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
	smtp := models.SMTP{Name: "database smtp", UserId: admin.Id, Host: "example.test:25", FromAddress: "sender@example.test"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}
	campaign := models.Campaign{
		Name: "database campaign", UserId: admin.Id, Template: template, Page: page, SMTP: smtp, Groups: []models.Group{group},
		CredentialCaptureMode: models.CredentialModeEncryptedReview, CredentialRetentionHours: 24,
		CredentialPolicy: models.CredentialPolicy{MinLength: 12, MaxLength: 128, MinDigits: 1},
	}
	if err := models.PostCampaign(&campaign, admin.Id); err != nil {
		t.Fatal(err)
	}
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
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatalf("audit persistence verification failed: %v", err)
	}
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
