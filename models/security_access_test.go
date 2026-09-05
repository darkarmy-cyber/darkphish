package models

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	"gopkg.in/check.v1"
)

func createSecurityTestUser(c *check.C, username, roleSlug string) User {
	role, err := GetRoleBySlug(roleSlug)
	c.Assert(err, check.IsNil)
	hash, err := auth.GeneratePasswordHash("correct horse battery staple")
	c.Assert(err, check.IsNil)
	user := User{
		Username: username, Hash: hash, ApiKey: "disabled-" + username,
		Role: role, RoleID: role.ID,
	}
	c.Assert(db.Save(&user).Error, check.IsNil)
	return user
}

func (s *ModelsSuite) TestCredentialReviewAuthorizationMatrix(c *check.C) {
	now := time.Now().UTC()
	owner := createSecurityTestUser(c, "matrix-owner", RoleUser)
	reviewer := createSecurityTestUser(c, "matrix-reviewer", RoleSecurityReviewer)
	campaign := Campaign{Name: "authorization matrix", UserId: owner.Id}
	c.Assert(db.Save(&campaign).Error, check.IsNil)

	allowed, err := CanReviewCredential(owner, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, false)

	allowed, err = CanReviewCredential(reviewer, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, false)

	admin, err := GetUser(1)
	c.Assert(err, check.IsNil)
	assignment, err := AssignCampaignReviewer(campaign.Id, reviewer.Id, admin, nil, audit.Event{Result: "success", Metadata: "{}"})
	c.Assert(err, check.IsNil)
	c.Assert(assignment.UserID, check.Equals, reviewer.Id)
	allowed, err = CanReviewCredential(reviewer, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, true)

	past := now.Add(-time.Minute)
	c.Assert(db.Model(&CampaignReviewer{}).Where("id=?", assignment.ID).Update("expires_at", past).Error, check.IsNil)
	allowed, err = CanReviewCredential(reviewer, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, false)

	allowed, err = CanReviewCredential(admin, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, true)
	admin.AccountLocked = true
	allowed, err = CanReviewCredential(admin, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, false)
	admin.AccountLocked = false
	admin.PasswordChangeRequired = true
	allowed, err = CanReviewCredential(admin, campaign.Id, now)
	c.Assert(err, check.IsNil)
	c.Assert(allowed, check.Equals, false)
}

func (s *ModelsSuite) TestPrivilegedReauthenticationIsSessionBoundAndExpires(c *check.C) {
	user := createSecurityTestUser(c, "reauth-user", RoleSecurityReviewer)
	now := time.Now().UTC()
	binding, err := NewSessionBinding()
	c.Assert(err, check.IsNil)

	fresh, err := IsPrivilegedSessionFresh(user.Id, binding, now)
	c.Assert(err, check.IsNil)
	c.Assert(fresh, check.Equals, false)
	_, err = ReauthenticatePrivileged(context.Background(), user, binding, ReauthenticationProof{Method: "password", Secret: "wrong password"}, now)
	c.Assert(errors.Is(err, auth.ErrInvalidPassword), check.Equals, true)

	privileged, err := ReauthenticatePrivileged(context.Background(), user, binding, ReauthenticationProof{Method: "password", Secret: "correct horse battery staple"}, now)
	c.Assert(err, check.IsNil)
	c.Assert(privileged.ExpiresAt.Sub(privileged.ReauthenticatedAt), check.Equals, 5*time.Minute)
	fresh, err = IsPrivilegedSessionFresh(user.Id, binding, now.Add(time.Minute))
	c.Assert(err, check.IsNil)
	c.Assert(fresh, check.Equals, true)
	fresh, err = IsPrivilegedSessionFresh(user.Id, binding+"other", now.Add(time.Minute))
	c.Assert(err, check.IsNil)
	c.Assert(fresh, check.Equals, false)
	fresh, err = IsPrivilegedSessionFresh(user.Id, binding, now.Add(6*time.Minute))
	c.Assert(err, check.IsNil)
	c.Assert(fresh, check.Equals, false)
}

func (s *ModelsSuite) TestCredentialPersistenceRollsBackWhenCiphertextWriteFails(c *check.C) {
	store, err := secretpkg.NewKeyring("TEST", map[string][]byte{"TEST": []byte("0123456789abcdef0123456789abcdef")}, nil)
	c.Assert(err, check.IsNil)
	previousStore := secretStore
	secretStore = store
	defer func() { secretStore = previousStore }()
	campaign := s.createCampaignDependencies(c)
	campaign.CredentialCaptureMode = CredentialModeEncryptedReview
	campaign.CredentialRetentionHours = 24
	campaign.credentialRetentionSpecified = true
	campaign.CredentialPolicy = defaultCredentialPolicy()
	c.Assert(PostCampaign(&campaign, 1), check.IsNil)
	result := campaign.Results[0]
	c.Assert(db.Exec(`CREATE TRIGGER reject_encrypted_credential BEFORE INSERT ON encrypted_credentials BEGIN SELECT RAISE(ABORT, 'injected ciphertext failure'); END`).Error, check.IsNil)
	defer db.Exec("DROP TRIGGER IF EXISTS reject_encrypted_credential")

	c.Assert(RecordCredentialSubmission(campaign, result, "Synthetic-credential-9!"), check.NotNil)
	var findingCount, ciphertextCount int
	c.Assert(db.Model(&CredentialPolicyResult{}).Where("result_id=?", result.Id).Count(&findingCount).Error, check.IsNil)
	c.Assert(db.Model(&EncryptedCredential{}).Where("result_id=?", result.Id).Count(&ciphertextCount).Error, check.IsNil)
	c.Assert(findingCount, check.Equals, 0)
	c.Assert(ciphertextCount, check.Equals, 0)
}

func (s *ModelsSuite) TestNonRetainableResubmissionRemovesStaleCiphertext(c *check.C) {
	store, err := secretpkg.NewKeyring("TEST", map[string][]byte{"TEST": []byte("0123456789abcdef0123456789abcdef")}, nil)
	c.Assert(err, check.IsNil)
	previousStore := secretStore
	secretStore = store
	defer func() { secretStore = previousStore }()
	campaign := s.createCampaignDependencies(c)
	campaign.CredentialCaptureMode = CredentialModeEncryptedReview
	campaign.CredentialRetentionHours = 24
	campaign.credentialRetentionSpecified = true
	campaign.CredentialPolicy = defaultCredentialPolicy()
	c.Assert(PostCampaign(&campaign, 1), check.IsNil)
	result := campaign.Results[0]
	c.Assert(RecordCredentialSubmission(campaign, result, "Short-9!"), check.IsNil)

	c.Assert(RecordCredentialSubmission(campaign, result, strings.Repeat("x", campaign.CredentialPolicy.MaxLength+1)), check.IsNil)
	var ciphertextCount int
	c.Assert(db.Model(&EncryptedCredential{}).Where("result_id=? AND encrypted_value <> ''", result.Id).Count(&ciphertextCount).Error, check.IsNil)
	c.Assert(ciphertextCount, check.Equals, 0)
}

func (s *ModelsSuite) TestReviewerAssignmentRollsBackWhenOutboxWriteFails(c *check.C) {
	reviewer := createSecurityTestUser(c, "outbox-reviewer", RoleSecurityReviewer)
	campaign := Campaign{Name: "outbox transaction", UserId: 1}
	c.Assert(db.Save(&campaign).Error, check.IsNil)
	admin, err := GetUser(1)
	c.Assert(err, check.IsNil)
	c.Assert(db.Exec(`CREATE TRIGGER reject_audit_outbox BEFORE INSERT ON audit_outbox BEGIN SELECT RAISE(ABORT, 'injected outbox failure'); END`).Error, check.IsNil)
	defer db.Exec("DROP TRIGGER IF EXISTS reject_audit_outbox")

	_, err = AssignCampaignReviewer(campaign.Id, reviewer.Id, admin, nil, audit.Event{Result: "success", Metadata: "{}"})
	c.Assert(err, check.NotNil)
	var count int
	c.Assert(db.Model(&CampaignReviewer{}).Where("campaign_id=? AND user_id=?", campaign.Id, reviewer.Id).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *ModelsSuite) TestReviewerExpiryIsAuditedExactlyOnce(c *check.C) {
	reviewer := createSecurityTestUser(c, "expiry-reviewer", RoleSecurityReviewer)
	campaign := Campaign{Name: "expiry audit", UserId: 1}
	c.Assert(db.Save(&campaign).Error, check.IsNil)
	now := time.Now().UTC()
	assignment := CampaignReviewer{CampaignID: campaign.Id, UserID: reviewer.Id, AssignedBy: 1, AssignedAt: now.Add(-2 * time.Hour), ExpiresAt: timePointer(now.Add(-time.Hour))}
	c.Assert(db.Save(&assignment).Error, check.IsNil)
	audited, err := AuditExpiredCampaignReviewers(now)
	c.Assert(err, check.IsNil)
	c.Assert(audited, check.Equals, int64(1))
	audited, err = AuditExpiredCampaignReviewers(now.Add(time.Minute))
	c.Assert(err, check.IsNil)
	c.Assert(audited, check.Equals, int64(0))
	_, total, err := audit.Query(audit.Filter{Action: "campaign.reviewer.expire", Page: 1, PerPage: 10})
	c.Assert(err, check.IsNil)
	c.Assert(total, check.Equals, int64(1))
}

func timePointer(value time.Time) *time.Time { return &value }

func (s *ModelsSuite) TestPATCreationNeverReturnsRawValueAfterPersistenceFailure(c *check.C) {
	c.Assert(db.Exec(`CREATE TRIGGER reject_pat BEFORE INSERT ON personal_access_tokens BEGIN SELECT RAISE(ABORT, 'injected token failure'); END`).Error, check.IsNil)
	defer db.Exec("DROP TRIGGER IF EXISTS reject_pat")
	_, raw, err := CreatePersonalAccessToken(1, "failure injection", []string{"campaigns:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.NotNil)
	c.Assert(raw, check.Equals, "")
}

func (s *ModelsSuite) TestAuditedPATCreationRollsBackWhenOutboxWriteFails(c *check.C) {
	c.Assert(db.Exec(`CREATE TRIGGER reject_pat_outbox BEFORE INSERT ON audit_outbox BEGIN SELECT RAISE(ABORT, 'injected outbox failure'); END`).Error, check.IsNil)
	defer db.Exec("DROP TRIGGER IF EXISTS reject_pat_outbox")
	_, raw, err := CreatePersonalAccessTokenWithAudit(1, "outbox failure", []string{"campaigns:read"}, time.Now().UTC().Add(time.Hour), audit.Event{Result: "success", Metadata: "{}"})
	c.Assert(err, check.NotNil)
	c.Assert(raw, check.Equals, "")
	var count int
	c.Assert(db.Model(&PersonalAccessToken{}).Where("name=?", "outbox failure").Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *ModelsSuite) TestSecretMigrationRollsBackWhenOutboxWriteFails(c *check.C) {
	keyOne := []byte("0123456789abcdef0123456789abcdef")
	keyTwo := []byte("abcdef0123456789abcdef0123456789")
	oldStore, err := secretpkg.NewKeyring("V1", map[string][]byte{"V1": keyOne}, nil)
	c.Assert(err, check.IsNil)
	oldCiphertext, err := oldStore.Seal("migration rollback value")
	c.Assert(err, check.IsNil)
	activeStore, err := secretpkg.NewKeyring("V2", map[string][]byte{"V1": keyOne, "V2": keyTwo}, nil)
	c.Assert(err, check.IsNil)
	previousStore := secretStore
	secretStore = activeStore
	defer func() { secretStore = previousStore }()
	record := EncryptedCredential{
		CampaignID: 9999, ResultID: 9999, EncryptedValue: oldCiphertext,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	c.Assert(db.Save(&record).Error, check.IsNil)
	c.Assert(db.Exec(`CREATE TRIGGER reject_secret_migration_outbox BEFORE INSERT ON audit_outbox BEGIN SELECT RAISE(ABORT, 'injected outbox failure'); END`).Error, check.IsNil)
	defer db.Exec("DROP TRIGGER IF EXISTS reject_secret_migration_outbox")

	rotated, err := RotateSecrets()
	c.Assert(err, check.NotNil)
	c.Assert(rotated, check.Equals, int64(0))
	var stored EncryptedCredential
	c.Assert(db.Where("id=?", record.ID).First(&stored).Error, check.IsNil)
	c.Assert(stored.EncryptedValue, check.Equals, oldCiphertext)
}
