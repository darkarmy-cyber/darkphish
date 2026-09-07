// Package compatibilitytest exercises the same synthetic database in two
// separate processes. CI first compiles this fixture against pinned v0.4 source,
// then compiles it against the candidate. It deliberately imports no ORM.
package compatibilitytest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
)

const fixturePassword = "synthetic-fixture-password-not-a-real-account"
const fixtureCredential = "Synthetic-Compatibility-Only-42!"

type fixtureState struct {
	OwnerID, ReviewerID, CampaignID int64
	ResultID, Token, Binding        string
	Schema, Content, Ciphertext     string
}

func TestVersionedDatabaseCompatibility(t *testing.T) {
	phase := os.Getenv("DARKPHISH_COMPAT_PHASE")
	if phase == "" {
		t.Skip("two-process v0.4 compatibility fixture is run explicitly by CI")
	}
	if phase != "seed" && phase != "check" {
		t.Fatal("invalid compatibility phase")
	}
	backend, dsn, directory := os.Getenv("DARKPHISH_COMPAT_BACKEND"), os.Getenv("DARKPHISH_COMPAT_DSN"), os.Getenv("DARKPHISH_COMPAT_DIRECTORY")
	if backend == "" || dsn == "" || directory == "" {
		t.Fatal("explicit disposable backend, DSN and fixture directory required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve migration fixture")
	}
	conf := &config.Config{DBName: backend, DBPath: dsn, MigrationsPath: filepath.Join(filepath.Dir(file), "..", "db", "db_"+backend, "migrations"), BootstrapDirectory: directory,
		DBMaxOpenConns: 5, DBMaxIdleConns: 2, DBConnMaxLifetimeMinutes: 5,
		Secrets: config.SecretsConfig{Provider: "local", ActiveKeyID: "fixture", Keys: map[string]string{"fixture": "0123456789abcdef0123456789abcdef"}},
		Audit:   config.AuditConfig{CheckpointInterval: 2, ActiveSigningKeyID: "fixture", SigningKeys: map[string]string{"fixture": "abcdef0123456789abcdef0123456789"}},
	}
	if backend == "sqlite3" {
		conf.DBMaxOpenConns = 1
		conf.DBMaxIdleConns = 1
	}
	t.Setenv("DARKPHISH_INITIAL_ADMIN_PASSWORD", fixturePassword)
	metadata := filepath.Join(directory, "synthetic-fixture.json")
	if phase == "seed" {
		if _, err := os.Stat(metadata); !os.IsNotExist(err) {
			t.Fatal("refusing to overwrite an existing fixture")
		}
		if err := models.Setup(conf); err != nil {
			t.Fatal(err)
		}
		defer models.Close()
		connection := verificationConnection(t, backend, dsn)
		state := seedFixture(t)
		state.Schema = schemaSnapshot(t, connection, backend)
		state.Content = contentSnapshot(t, connection, backend)
		state.Ciphertext = querySnapshot(t, connection, "SELECT encrypted_value FROM encrypted_credentials ORDER BY id")
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metadata, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("seeded synthetic database with legacy runtime; no real credentials or external services used")
		return
	}
	encoded, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal("read synthetic fixture metadata")
	}
	var state fixtureState
	if err := json.Unmarshal(encoded, &state); err != nil {
		t.Fatal("invalid fixture metadata")
	}
	connection := verificationConnection(t, backend, dsn)
	if state.Schema != schemaSnapshot(t, connection, backend) || state.Content != contentSnapshot(t, connection, backend) {
		t.Fatal("fixture changed before candidate startup")
	}
	if err := models.Setup(conf); err != nil {
		t.Fatal(err)
	}
	defer models.Close()
	if state.Schema != schemaSnapshot(t, connection, backend) {
		t.Fatal("candidate startup changed the v0.4 schema")
	}
	if state.Content != contentSnapshot(t, connection, backend) {
		t.Fatal("candidate startup silently rewrote legacy data")
	}
	if state.Ciphertext != querySnapshot(t, connection, "SELECT encrypted_value FROM encrypted_credentials ORDER BY id") {
		t.Fatal("credential ciphertext changed during startup")
	}
	verifyFixture(t, state)
	campaign, err := models.GetCampaign(state.CampaignID, state.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := campaign.UpdateStatus(models.CampaignComplete); err != nil {
		t.Fatal(err)
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.CreateAuditCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatal(err)
	}
	if err := models.Close(); err != nil {
		t.Fatal(err)
	}
	if err := models.Setup(conf); err != nil {
		t.Fatal(err)
	}
	campaign, err = models.GetCampaign(state.CampaignID, state.OwnerID)
	if err != nil || campaign.Status != models.CampaignComplete {
		t.Fatal("normal write did not survive candidate restart")
	}
	verifyFixture(t, state)
	if state.Schema != schemaSnapshot(t, connection, backend) || state.Ciphertext != querySnapshot(t, connection, "SELECT encrypted_value FROM encrypted_credentials ORDER BY id") {
		t.Fatal("restart/schema/ciphertext compatibility failed")
	}
	t.Log("PASS legacy login/PAT/campaign/reviewer/reauth/reveal/audit/checkpoint, zero startup rewrites, normal write and restart")
}

func seedFixture(t *testing.T) fixtureState {
	t.Helper()
	owner, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	owner.Hash, err = auth.GeneratePasswordHash(fixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	owner.PasswordChangeRequired = false
	if err := models.PutUser(&owner); err != nil {
		t.Fatal(err)
	}
	group := models.Group{UserId: owner.Id, Name: "v04 fixture group", Targets: []models.Target{{BaseRecipient: models.BaseRecipient{Email: "fixture@example.test", FirstName: "Fixture", LastName: "Only", Position: "Synthetic"}}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatal(err)
	}
	template := models.Template{UserId: owner.Id, Name: "v04 fixture template", Subject: "synthetic", Text: "synthetic", HTML: "<p>synthetic</p>"}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatal(err)
	}
	page := models.Page{UserId: owner.Id, Name: "v04 fixture page", HTML: "<form><input type=\"password\"></form>", CaptureCredentials: true, CapturePasswords: true}
	if err := models.PostPage(&page); err != nil {
		t.Fatal(err)
	}
	smtp := models.SMTP{UserId: owner.Id, Name: "v04 fixture smtp", Host: "fixture.example.test:25", FromAddress: "fixture@example.test", Username: "synthetic", Password: "synthetic-integration-only"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}
	campaign := models.Campaign{UserId: owner.Id, Name: "v04 fixture campaign", Groups: []models.Group{group}, Template: template, Page: page, SMTP: smtp, LaunchDate: time.Now().UTC().Add(48 * time.Hour),
		CredentialCaptureMode: models.CredentialModeEncryptedReview, CredentialRetentionHours: 72, CredentialPolicy: models.CredentialPolicy{MinLength: 12, MaxLength: 128, MinDigits: 1}}
	if err := models.PostCampaign(&campaign, owner.Id); err != nil {
		t.Fatal(err)
	}
	if len(campaign.Results) != 1 {
		t.Fatal("unexpected fixture recipients")
	}
	if err := models.RecordCredentialSubmission(campaign, campaign.Results[0], fixtureCredential); err != nil {
		t.Fatal(err)
	}
	role, err := models.GetRoleBySlug(models.RoleSecurityReviewer)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := models.User{Username: "v04-fixture-reviewer", Hash: owner.Hash, ApiKey: "disabled-v04-fixture-reviewer", Role: role, RoleID: role.ID}
	if err := models.PutUser(&reviewer); err != nil {
		t.Fatal(err)
	}
	event := audit.Event{Result: "success", Metadata: "{}"}
	if _, err := models.AssignCampaignReviewer(campaign.Id, reviewer.Id, owner, nil, event); err != nil {
		t.Fatal(err)
	}
	_, token, err := models.CreatePersonalAccessTokenWithAudit(owner.Id, "v04 fixture active PAT", []string{"campaigns:read", "campaigns:write", "credentials:view"}, time.Now().UTC().Add(72*time.Hour), event)
	if err != nil || token == "" {
		t.Fatal("fixture PAT creation failed")
	}
	binding, err := models.NewSessionBinding()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.ReauthenticatePrivileged(context.Background(), reviewer, binding, models.ReauthenticationProof{Method: "password", Secret: fixturePassword}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.CreateAuditCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatal(err)
	}
	return fixtureState{OwnerID: owner.Id, ReviewerID: reviewer.Id, CampaignID: campaign.Id, ResultID: campaign.Results[0].RId, Token: token, Binding: binding}
}

func verifyFixture(t *testing.T, state fixtureState) {
	t.Helper()
	owner, err := models.GetUser(state.OwnerID)
	if err != nil || auth.ValidatePassword(fixturePassword, owner.Hash) != nil {
		t.Fatal("legacy password hash/login compatibility failed")
	}
	if _, err := models.AuthenticatePersonalAccessToken(state.Token); err != nil {
		t.Fatal("legacy PAT did not authenticate")
	}
	campaign, err := models.GetCampaign(state.CampaignID, state.OwnerID)
	if err != nil || len(campaign.Results) != 1 || campaign.Results[0].RId != state.ResultID || campaign.Template.Name != "v04 fixture template" || campaign.Page.Name != "v04 fixture page" || campaign.SMTP.Password != "synthetic-integration-only" {
		t.Fatal("legacy campaign/association/integration-secret read failed")
	}
	if _, err := models.GetCampaign(state.CampaignID, state.OwnerID+10000); err == nil {
		t.Fatal("legacy campaign escaped ownership boundary")
	}
	reviewer, err := models.GetUser(state.ReviewerID)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := models.CanReviewCredential(reviewer, state.CampaignID, time.Now().UTC()); err != nil || !allowed {
		t.Fatal("legacy reviewer assignment failed")
	}
	if _, err := models.ReauthenticatePrivileged(context.Background(), reviewer, state.Binding, models.ReauthenticationProof{Method: "password", Secret: fixturePassword}, time.Now().UTC()); err != nil {
		t.Fatal("legacy session reauthentication failed")
	}
	if fresh, err := models.IsPrivilegedSessionFresh(reviewer.Id, state.Binding, time.Now().UTC()); err != nil || !fresh {
		t.Fatal("fresh session not persisted")
	}
	revealed, err := models.RevealCredential(state.CampaignID, state.ResultID, reviewer.Id)
	if err != nil || revealed.Credential != fixtureCredential {
		t.Fatal("legacy encrypted credential failed authorized reveal")
	}
	if _, err := models.RevealCredential(state.CampaignID, state.ResultID, state.ReviewerID+10000); err == nil {
		t.Fatal("unassigned identity gained credential reveal")
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatal("stored audit chain/checkpoint verification failed")
	}
	content, manifest, err := models.BuildAuditExport("compatibility-fixture", "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.VerifyAuditExport(content, encoded); err != nil {
		t.Fatal("stored signed export verification failed")
	}
}

func verificationConnection(t *testing.T, backend, dsn string) *sql.DB {
	t.Helper()
	driver := backend
	if backend == "postgres" {
		for _, registered := range sql.Drivers() {
			if registered == "pgx" {
				driver = "pgx"
			}
		}
	}
	connection, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal("open isolated verification connection")
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func schemaSnapshot(t *testing.T, connection *sql.DB, backend string) string {
	t.Helper()
	if os.Getenv("DARKPHISH_COMPAT_COORDINATION") == "1" {
		switch backend {
		case "sqlite3":
			return querySnapshot(t, connection, "SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name")
		case "mysql":
			return querySnapshot(t, connection, "SELECT table_name,column_name,column_type,is_nullable,column_default,extra FROM information_schema.columns WHERE table_schema=DATABASE() ORDER BY table_name,ordinal_position")
		case "postgres":
			return querySnapshot(t, connection, "SELECT table_name,column_name,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema=current_schema() ORDER BY table_name,ordinal_position")
		}
	}
	switch backend {
	case "sqlite3":
		return querySnapshot(t, connection, "SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND tbl_name NOT IN ('audit_chain_heads','audit_delivery_receipts','audit_signing_identities') AND name <> 'idx_audit_checkpoints_chain_sequence' ORDER BY type,name")
	case "mysql":
		return querySnapshot(t, connection, "SELECT table_name,column_name,column_type,is_nullable,column_default,extra FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name NOT IN ('audit_chain_heads','audit_delivery_receipts','audit_signing_identities') ORDER BY table_name,ordinal_position")
	case "postgres":
		return querySnapshot(t, connection, "SELECT table_name,column_name,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name NOT IN ('audit_chain_heads','audit_delivery_receipts','audit_signing_identities') ORDER BY table_name,ordinal_position")
	default:
		t.Fatal("unsupported fixture backend")
		return ""
	}
}

func contentSnapshot(t *testing.T, connection *sql.DB, backend string) string {
	t.Helper()
	// Static schema allowlist only. No request or external identifier reaches SQL.
	tables := []string{"users", "roles", "permissions", "role_permissions", "campaigns", "templates", "attachments", "pages", "smtp", "headers", "groups", "targets", "group_targets", "results", "events", "mail_logs", "email_requests", "imap", "webhooks", "campaign_credential_policies", "credential_policy_results", "encrypted_credentials", "personal_access_tokens", "privileged_sessions", "campaign_reviewers", "audit_events", "audit_outbox", "audit_checkpoints", "goose_db_version"}
	includeCoordination := os.Getenv("DARKPHISH_COMPAT_COORDINATION") == "1"
	if includeCoordination {
		tables = append(tables, "audit_chain_heads", "audit_delivery_receipts", "audit_signing_identities")
	}
	var parts []string
	for _, table := range tables {
		quoted := "\"" + table + "\""
		if backend == "mysql" {
			quoted = "`" + table + "`"
		}
		query := "SELECT * FROM " + quoted
		if table == "goose_db_version" && !includeCoordination {
			query += " WHERE version_id <= 20260905010000"
		}
		parts = append(parts, table+":"+querySnapshot(t, connection, query))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func querySnapshot(t *testing.T, connection *sql.DB, query string) string {
	t.Helper()
	rows, err := connection.Query(query)
	if err != nil {
		t.Fatal("fixture snapshot query failed")
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var normalized []string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		destinations := make([]interface{}, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			t.Fatal(err)
		}
		for i, value := range values {
			switch v := value.(type) {
			case nil:
			case time.Time:
				values[i] = v.UTC().Format(time.RFC3339Nano)
			case []byte:
				values[i] = string(v)
			default:
				values[i] = fmt.Sprint(v)
			}
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		normalized = append(normalized, string(encoded))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(normalized)
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return hex.EncodeToString(sum[:])
}
