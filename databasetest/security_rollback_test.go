package databasetest

import (
	"database/sql"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
)

func exerciseSecurityRollback(t *testing.T, conf *config.Config, connection *sql.DB, admin, reviewer models.User, campaign models.Campaign, binding, credential string) {
	t.Helper()
	event := audit.Event{Result: "success", Metadata: "{}", Action: "test.security.mutation"}
	token, raw, err := models.CreatePersonalAccessTokenWithAudit(admin.Id, "rollback existing token", []string{"campaigns:read"}, time.Now().UTC().Add(time.Hour), event)
	if err != nil {
		t.Fatal(err)
	}
	other := models.User{Username: "rollback-unassigned-reviewer", Hash: reviewer.Hash, ApiKey: "disabled-rollback-reviewer", RoleID: reviewer.RoleID}
	if err := models.PutUser(&other); err != nil {
		t.Fatal(err)
	}
	other, err = models.GetUser(other.Id)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := func() string {
		t.Helper()
		var value string
		// This fixture has exactly one retained synthetic credential.
		if err := connection.QueryRow("SELECT encrypted_value FROM encrypted_credentials").Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := ciphertext()
	stopFailure := rejectOutbox(t, connection, conf.DBName)
	if err := models.RevokePersonalAccessTokenWithAudit(token.ID, admin.Id, event); err == nil {
		t.Fatal("PAT revocation bypassed outbox failure")
	}
	if _, err := models.AssignCampaignReviewer(campaign.Id, other.Id, admin, nil, event); err == nil {
		t.Fatal("reviewer assignment bypassed outbox failure")
	}
	changed := reviewer
	changed.AccountLocked = true
	changed.RoleID = admin.RoleID
	if err := models.PutUserWithAudit(&changed, true, event); err == nil {
		t.Fatal("account/role update bypassed outbox failure")
	}
	if _, err := models.AuthenticatePersonalAccessToken(raw); err != nil {
		t.Fatal("failed PAT revocation did not roll back")
	}
	if allowed, err := models.CanReviewCredential(other, campaign.Id, time.Now().UTC()); err != nil || allowed {
		t.Fatal("failed assignment left reviewer access")
	}
	reloaded, err := models.GetUser(reviewer.Id)
	if err != nil || reloaded.AccountLocked || reloaded.RoleID != reviewer.RoleID {
		t.Fatal("failed account/role mutation did not roll back")
	}
	if fresh, err := models.IsPrivilegedSessionFresh(reviewer.Id, binding, time.Now().UTC()); err != nil || !fresh {
		t.Fatal("failed account mutation revoked privileged grant")
	}
	stopFailure()
	// Capture persistence has a finding+ciphertext transaction, while audited
	// reveal/rotation use the audit boundary. Inject at the actual write boundary.
	stopCiphertextFailure := rejectCiphertext(t, connection, conf.DBName)
	if err := models.RecordCredentialSubmission(campaign, campaign.Results[0], "Replacement-synthetic-value-9!"); err == nil {
		t.Fatal("credential replacement bypassed ciphertext write failure")
	}
	stopCiphertextFailure()
	if ciphertext() != before {
		t.Fatal("failed credential transaction changed persisted ciphertext")
	}
	if reveal, err := models.RevealCredential(campaign.Id, campaign.Results[0].RId, reviewer.Id); err != nil || reveal.Credential != credential {
		t.Fatal("original encrypted credential did not survive rollback")
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}

	// Reopen with a new active wrapping key but retain the old key. Startup must
	// not rotate data; an explicit rotation and its outbox write are atomic.
	rotatedConf := *conf
	rotatedConf.Secrets = conf.Secrets
	rotatedConf.Secrets.Keys = map[string]string{}
	for id, key := range conf.Secrets.Keys {
		rotatedConf.Secrets.Keys[id] = key
	}
	rotatedConf.Secrets.ActiveKeyID = "NEXT"
	rotatedConf.Secrets.Keys["NEXT"] = "fedcba9876543210fedcba9876543210"
	if err := models.Close(); err != nil {
		t.Fatal(err)
	}
	if err := models.Setup(&rotatedConf); err != nil {
		t.Fatal(err)
	}
	if ciphertext() != before {
		t.Fatal("startup unexpectedly rewrote ciphertext")
	}
	stopFailure = rejectOutbox(t, connection, conf.DBName)
	if count, err := models.RotateSecrets(); err == nil || count != 0 {
		t.Fatal("failed secret rotation committed partial progress")
	}
	if ciphertext() != before {
		t.Fatal("failed rotation changed retained ciphertext")
	}
	stopFailure()
	if count, err := models.RotateSecrets(); err != nil || count == 0 {
		t.Fatalf("explicit secret rotation failed: %v", err)
	}
	if ciphertext() == before {
		t.Fatal("explicit rotation did not rewrap retained ciphertext")
	}
	if status, err := models.SecretsStatus(); err != nil || status.RequiringMigration != 0 {
		t.Fatal("rotation left stale migration state")
	}
	if reveal, err := models.RevealCredential(campaign.Id, campaign.Results[0].RId, reviewer.Id); err != nil || reveal.Credential != credential {
		t.Fatal("rotated credential failed authorized decryption")
	}
	if err := models.FlushAuditOutbox(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.CreateAuditCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if _, err := models.VerifyAuditChain(); err != nil {
		t.Fatal("rotation invalidated audit integrity")
	}
}
