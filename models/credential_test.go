package models

import (
	"encoding/json"
	"strings"
	"time"

	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	"gopkg.in/check.v1"
)

func (s *ModelsSuite) TestCredentialPolicyOnlyNeverStoresValue(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.CredentialCaptureMode = CredentialModePolicyOnly
	campaign.CredentialPolicy = CredentialPolicy{
		MinLength: 14, MaxLength: 128, MinUppercase: 1,
		MinDigits: 2, DisallowedPatterns: []string{"darkphish"},
	}
	c.Assert(PostCampaign(&campaign, 1), check.IsNil)
	result := campaign.Results[0]
	credential := "Darkphish-example-9"
	c.Assert(RecordCredentialSubmission(campaign, result, credential), check.IsNil)

	var finding CredentialPolicyResult
	c.Assert(db.Where("result_id=?", result.Id).First(&finding).Error, check.IsNil)
	c.Assert(finding.PolicyPassed, check.Equals, false)
	c.Assert(strings.Contains(finding.FailuresRaw, "disallowed_pattern"), check.Equals, true)
	var count int64
	c.Assert(db.Model(&EncryptedCredential{}).Where("result_id=?", result.Id).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	encoded, err := json.Marshal(finding)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(encoded), credential), check.Equals, false)
}

func (s *ModelsSuite) TestEncryptedCredentialRevealAndExpiry(c *check.C) {
	store, err := secretpkg.NewKeyring("V1", map[string][]byte{"V1": []byte("0123456789abcdef0123456789abcdef")}, nil)
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
	credential := "Never log this credential!9"
	c.Assert(RecordCredentialSubmission(campaign, result, credential), check.IsNil)

	var stored EncryptedCredential
	c.Assert(db.Where("result_id=?", result.Id).First(&stored).Error, check.IsNil)
	c.Assert(stored.EncryptedValue == credential, check.Equals, false)
	c.Assert(strings.HasPrefix(stored.EncryptedValue, "darkphish:secret:v2:"), check.Equals, true)
	reveal, err := RevealCredential(campaign.Id, result.RId, 1)
	c.Assert(err, check.IsNil)
	c.Assert(reveal.Credential, check.Equals, credential)

	_, err = DeleteExpiredCredentialValues(time.Now().UTC().Add(25 * time.Hour))
	c.Assert(err, check.IsNil)
	_, err = RevealCredential(campaign.Id, result.RId, 1)
	c.Assert(err, check.Equals, ErrCredentialExpired)
	c.Assert(db.Where("id=?", stored.ID).First(&stored).Error, check.IsNil)
	c.Assert(stored.EncryptedValue, check.Equals, "")
	var finding CredentialPolicyResult
	c.Assert(db.Where("result_id=?", result.Id).First(&finding).Error, check.IsNil)
	c.Assert(finding.PolicyPassed, check.Equals, true)
}

func (s *ModelsSuite) TestZeroRetentionKeepsOnlyPolicyFindings(c *check.C) {
	store, err := secretpkg.NewKeyring("V1", map[string][]byte{"V1": []byte("0123456789abcdef0123456789abcdef")}, nil)
	c.Assert(err, check.IsNil)
	previousStore := secretStore
	secretStore = store
	defer func() { secretStore = previousStore }()

	campaign := s.createCampaignDependencies(c)
	campaign.CredentialCaptureMode = CredentialModeEncryptedReview
	campaign.CredentialRetentionHours = 0
	campaign.credentialRetentionSpecified = true
	campaign.CredentialPolicy = defaultCredentialPolicy()
	c.Assert(PostCampaign(&campaign, 1), check.IsNil)
	result := campaign.Results[0]
	c.Assert(RecordCredentialSubmission(campaign, result, "Synthetic-only-credential-9"), check.IsNil)
	var encryptedCount, findingCount int64
	c.Assert(db.Model(&EncryptedCredential{}).Where("result_id=?", result.Id).Count(&encryptedCount).Error, check.IsNil)
	c.Assert(db.Model(&CredentialPolicyResult{}).Where("result_id=?", result.Id).Count(&findingCount).Error, check.IsNil)
	c.Assert(encryptedCount, check.Equals, int64(0))
	c.Assert(findingCount, check.Equals, int64(1))
}

func (s *ModelsSuite) TestRevealRejectsPlaintextCredentialRows(c *check.C) {
	store, err := secretpkg.NewKeyring("V1", map[string][]byte{"V1": []byte("0123456789abcdef0123456789abcdef")}, nil)
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
	now := time.Now().UTC()
	record := EncryptedCredential{
		CampaignID: campaign.Id, ResultID: result.Id, EncryptedValue: "not-encrypted",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	c.Assert(db.Save(&record).Error, check.IsNil)
	_, err = RevealCredential(campaign.Id, result.RId, 1)
	c.Assert(err, check.Equals, ErrCredentialEncryption)
}

func (s *ModelsSuite) TestEncryptedReviewDoesNotRetainOversizedValues(c *check.C) {
	store, err := secretpkg.NewKeyring("V1", map[string][]byte{"V1": []byte("0123456789abcdef0123456789abcdef")}, nil)
	c.Assert(err, check.IsNil)
	previousStore := secretStore
	secretStore = store
	defer func() { secretStore = previousStore }()

	campaign := s.createCampaignDependencies(c)
	campaign.CredentialCaptureMode = CredentialModeEncryptedReview
	campaign.CredentialRetentionHours = 24
	campaign.credentialRetentionSpecified = true
	campaign.CredentialPolicy = CredentialPolicy{MinLength: 12, MaxLength: 16}
	c.Assert(PostCampaign(&campaign, 1), check.IsNil)
	result := campaign.Results[0]
	c.Assert(RecordCredentialSubmission(campaign, result, strings.Repeat("x", 17)), check.IsNil)
	var finding CredentialPolicyResult
	c.Assert(db.Where("result_id=?", result.Id).First(&finding).Error, check.IsNil)
	c.Assert(finding.PolicyPassed, check.Equals, false)
	var encryptedCount int64
	c.Assert(db.Model(&EncryptedCredential{}).Where("result_id=?", result.Id).Count(&encryptedCount).Error, check.IsNil)
	c.Assert(encryptedCount, check.Equals, int64(0))
}
