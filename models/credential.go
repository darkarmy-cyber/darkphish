package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	"github.com/jinzhu/gorm"
)

const (
	CredentialModeDisabled        = "disabled"
	CredentialModePolicyOnly      = "policy_only"
	CredentialModeEncryptedReview = "encrypted_review"
	DefaultCredentialRetention    = 24
	MaxCredentialRetention        = 24 * 30
)

var (
	ErrCredentialReviewDisabled = errors.New("encrypted credential review is not enabled for this campaign")
	ErrCredentialExpired        = errors.New("retained credential has expired")
	ErrCredentialEncryption     = errors.New("encrypted credential review requires a configured versioned secret keyring")
)

// CredentialPolicy is the per-campaign password-policy measurement profile.
// DisallowedPatterns are evaluated case-insensitively and are never copied to
// result rows alongside submitted values.
type CredentialPolicy struct {
	CampaignID            int64    `json:"-" gorm:"column:campaign_id;primary_key"`
	MinLength             int      `json:"min_length"`
	MaxLength             int      `json:"max_length"`
	MinUppercase          int      `json:"minimum_uppercase"`
	MinLowercase          int      `json:"minimum_lowercase"`
	MinDigits             int      `json:"minimum_digits"`
	MinSymbols            int      `json:"minimum_symbols"`
	DisallowedPatternsRaw string   `json:"-" gorm:"column:disallowed_patterns"`
	DisallowedPatterns    []string `json:"disallowed_patterns" gorm:"-"`
}

func (CredentialPolicy) TableName() string { return "campaign_credential_policies" }

// CredentialPolicyResult contains irreversible derived measurements only.
type CredentialPolicyResult struct {
	ID             int64     `json:"-"`
	CampaignID     int64     `json:"-"`
	ResultID       int64     `json:"-"`
	EvaluatedAt    time.Time `json:"evaluated_at"`
	Length         int       `json:"length"`
	UppercaseCount int       `json:"uppercase_count"`
	LowercaseCount int       `json:"lowercase_count"`
	DigitCount     int       `json:"digit_count"`
	SymbolCount    int       `json:"symbol_count"`
	StrengthScore  int       `json:"strength_score"`
	PolicyPassed   bool      `json:"policy_passed"`
	FailuresRaw    string    `json:"-" gorm:"column:failures"`
	Failures       []string  `json:"failures" gorm:"-"`
}

type EncryptedCredential struct {
	ID             int64      `json:"-"`
	CampaignID     int64      `json:"-"`
	ResultID       int64      `json:"-"`
	EncryptedValue string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	PurgedAt       *time.Time `json:"purged_at,omitempty"`
}

type CredentialReveal struct {
	Credential string    `json:"credential"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func defaultCredentialPolicy() CredentialPolicy {
	return CredentialPolicy{MinLength: 12, MaxLength: 128, DisallowedPatterns: []string{}}
}

func normalizeCredentialPolicy(policy *CredentialPolicy) error {
	if policy.MinLength == 0 {
		policy.MinLength = 12
	}
	if policy.MaxLength == 0 {
		policy.MaxLength = 128
	}
	if policy.MinLength < 1 || policy.MaxLength < policy.MinLength || policy.MaxLength > 4096 {
		return errors.New("credential policy length range must be between 1 and 4096")
	}
	for name, count := range map[string]int{
		"minimum_uppercase": policy.MinUppercase,
		"minimum_lowercase": policy.MinLowercase,
		"minimum_digits":    policy.MinDigits,
		"minimum_symbols":   policy.MinSymbols,
	} {
		if count < 0 || count > policy.MaxLength {
			return fmt.Errorf("%s must be between 0 and maximum_length", name)
		}
	}
	if len(policy.DisallowedPatterns) > 20 {
		return errors.New("credential policy supports at most 20 disallowed patterns")
	}
	clean := make([]string, 0, len(policy.DisallowedPatterns))
	for _, pattern := range policy.DisallowedPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if utf8.RuneCountInString(pattern) > 64 {
			return errors.New("credential policy patterns must not exceed 64 characters")
		}
		clean = append(clean, pattern)
	}
	policy.DisallowedPatterns = clean
	encoded, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	policy.DisallowedPatternsRaw = string(encoded)
	return nil
}

func loadCredentialPolicy(campaignID int64) (CredentialPolicy, error) {
	policy := defaultCredentialPolicy()
	err := db.Where("campaign_id=?", campaignID).First(&policy).Error
	if err != nil {
		return policy, err
	}
	if policy.DisallowedPatternsRaw != "" {
		if err := json.Unmarshal([]byte(policy.DisallowedPatternsRaw), &policy.DisallowedPatterns); err != nil {
			return policy, fmt.Errorf("decode credential policy patterns: %w", err)
		}
	}
	return policy, nil
}

func saveCredentialPolicy(campaignID int64, policy CredentialPolicy) error {
	if err := normalizeCredentialPolicy(&policy); err != nil {
		return err
	}
	policy.CampaignID = campaignID
	return db.Save(&policy).Error
}

func secretEncryptionAvailable() bool {
	_, ok := secretStore.(secretpkg.VersionedStore)
	return ok
}

func validateCampaignCredentialControls(c *Campaign) error {
	switch c.CredentialCaptureMode {
	case "":
		c.CredentialCaptureMode = CredentialModeDisabled
	case CredentialModeDisabled, CredentialModePolicyOnly:
	case CredentialModeEncryptedReview:
		if !secretEncryptionAvailable() {
			return ErrCredentialEncryption
		}
	default:
		return fmt.Errorf("unsupported credential capture mode %q", c.CredentialCaptureMode)
	}
	if !c.credentialRetentionSpecified {
		c.CredentialRetentionHours = DefaultCredentialRetention
	}
	if c.CredentialRetentionHours < 0 || c.CredentialRetentionHours > MaxCredentialRetention {
		return fmt.Errorf("credential retention must be between 0 and %d hours", MaxCredentialRetention)
	}
	return normalizeCredentialPolicy(&c.CredentialPolicy)
}

func evaluateCredential(policy CredentialPolicy, credential string) CredentialPolicyResult {
	result := CredentialPolicyResult{EvaluatedAt: time.Now().UTC(), Length: utf8.RuneCountInString(credential)}
	for _, r := range credential {
		switch {
		case unicode.IsUpper(r):
			result.UppercaseCount++
		case unicode.IsLower(r):
			result.LowercaseCount++
		case unicode.IsDigit(r):
			result.DigitCount++
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			result.SymbolCount++
		}
	}
	if result.Length < policy.MinLength {
		result.Failures = append(result.Failures, "minimum_length")
	}
	if result.Length > policy.MaxLength {
		result.Failures = append(result.Failures, "maximum_length")
	}
	if result.UppercaseCount < policy.MinUppercase {
		result.Failures = append(result.Failures, "uppercase_required")
	}
	if result.LowercaseCount < policy.MinLowercase {
		result.Failures = append(result.Failures, "lowercase_required")
	}
	if result.DigitCount < policy.MinDigits {
		result.Failures = append(result.Failures, "digit_required")
	}
	if result.SymbolCount < policy.MinSymbols {
		result.Failures = append(result.Failures, "symbol_required")
	}
	lower := strings.ToLower(credential)
	for _, pattern := range policy.DisallowedPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			result.Failures = append(result.Failures, "disallowed_pattern")
			break
		}
	}
	result.PolicyPassed = len(result.Failures) == 0
	classes := 0
	for _, count := range []int{result.UppercaseCount, result.LowercaseCount, result.DigitCount, result.SymbolCount} {
		if count > 0 {
			classes++
		}
	}
	result.StrengthScore = classes
	if result.Length >= 16 && result.StrengthScore < 4 {
		result.StrengthScore++
	}
	if result.Length < 8 && result.StrengthScore > 1 {
		result.StrengthScore = 1
	}
	encoded, _ := json.Marshal(result.Failures)
	result.FailuresRaw = string(encoded)
	return result
}

// RecordCredentialSubmission persists only policy measurements unless the
// campaign explicitly enables encrypted review with non-zero retention.
func RecordCredentialSubmission(c Campaign, result Result, credential string) error {
	if c.CredentialCaptureMode == CredentialModeDisabled {
		return nil
	}
	policy := c.CredentialPolicy
	if policy.MaxLength == 0 {
		var err error
		policy, err = loadCredentialPolicy(c.Id)
		if err != nil {
			return err
		}
	}
	finding := evaluateCredential(policy, credential)
	finding.CampaignID = c.Id
	finding.ResultID = result.Id
	if err := db.Where("result_id=?", result.Id).Delete(&CredentialPolicyResult{}).Error; err != nil {
		return err
	}
	if err := db.Save(&finding).Error; err != nil {
		return err
	}
	// Values beyond the campaign's configured maximum remain represented by
	// the policy finding but are never retained as reviewable ciphertext.
	if c.CredentialCaptureMode != CredentialModeEncryptedReview || c.CredentialRetentionHours == 0 || !utf8.ValidString(credential) || finding.Length > policy.MaxLength {
		return nil
	}
	if !secretEncryptionAvailable() {
		return ErrCredentialEncryption
	}
	protected, err := secretStore.Seal(credential)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	record := EncryptedCredential{
		CampaignID: c.Id, ResultID: result.Id, EncryptedValue: protected,
		CreatedAt: now, ExpiresAt: now.Add(time.Duration(c.CredentialRetentionHours) * time.Hour),
	}
	if err := db.Where("result_id=?", result.Id).Delete(&EncryptedCredential{}).Error; err != nil {
		return err
	}
	return db.Save(&record).Error
}

func attachCredentialFindings(results []Result) error {
	if len(results) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(results))
	byID := make(map[int64]*Result, len(results))
	for i := range results {
		ids = append(ids, results[i].Id)
		byID[results[i].Id] = &results[i]
	}
	findings := []CredentialPolicyResult{}
	if err := db.Where("result_id IN (?)", ids).Find(&findings).Error; err != nil {
		return err
	}
	for i := range findings {
		_ = json.Unmarshal([]byte(findings[i].FailuresRaw), &findings[i].Failures)
		finding := findings[i]
		byID[finding.ResultID].CredentialPolicy = &finding
	}
	now := time.Now().UTC()
	records := []EncryptedCredential{}
	if err := db.Where("result_id IN (?) AND encrypted_value <> '' AND purged_at IS NULL AND expires_at > ?", ids, now).Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		byID[record.ResultID].CredentialReviewAvailable = true
	}
	return nil
}

// RevealCredential decrypts one retained value after the controller has
// applied both user authorization and the credentials:view permission.
func RevealCredential(campaignID int64, rid string, userID int64) (CredentialReveal, error) {
	var reveal CredentialReveal
	var result Result
	query := db.Table("results").
		Joins("JOIN campaigns ON campaigns.id=results.campaign_id").
		Where("results.campaign_id=? AND results.r_id=? AND campaigns.user_id=?", campaignID, rid, userID).
		Select("results.*").First(&result)
	if query.Error != nil {
		return reveal, query.Error
	}
	var campaign Campaign
	if err := db.Where("id=? AND user_id=?", campaignID, userID).First(&campaign).Error; err != nil {
		return reveal, err
	}
	if campaign.CredentialCaptureMode != CredentialModeEncryptedReview {
		return reveal, ErrCredentialReviewDisabled
	}
	var record EncryptedCredential
	if err := db.Where("result_id=?", result.Id).First(&record).Error; err != nil {
		return reveal, err
	}
	if record.PurgedAt != nil || record.EncryptedValue == "" || !record.ExpiresAt.After(time.Now().UTC()) {
		_ = purgeCredentialRecord(&record, time.Now().UTC())
		return reveal, ErrCredentialExpired
	}
	if !secretpkg.IsCiphertext(record.EncryptedValue) {
		return reveal, ErrCredentialEncryption
	}
	plaintext, err := secretStore.Open(record.EncryptedValue)
	if err != nil {
		return reveal, err
	}
	return CredentialReveal{Credential: plaintext, ExpiresAt: record.ExpiresAt}, nil
}

func purgeCredentialRecord(record *EncryptedCredential, now time.Time) error {
	return db.Model(&EncryptedCredential{}).Where("id=?", record.ID).
		Updates(map[string]interface{}{"encrypted_value": "", "purged_at": now}).Error
}

// DeleteExpiredCredentialValues destroys ciphertext while retaining policy
// findings and the non-secret purge timestamp.
func DeleteExpiredCredentialValues(now time.Time) (int64, error) {
	query := db.Model(&EncryptedCredential{}).
		Where("encrypted_value <> '' AND purged_at IS NULL AND expires_at <= ?", now.UTC()).
		Updates(map[string]interface{}{"encrypted_value": "", "purged_at": now.UTC()})
	return query.RowsAffected, query.Error
}

func deleteCampaignCredentialData(campaignID int64) error {
	if err := db.Where("campaign_id=?", campaignID).Delete(&EncryptedCredential{}).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err := db.Where("campaign_id=?", campaignID).Delete(&CredentialPolicyResult{}).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	return db.Where("campaign_id=?", campaignID).Delete(&CredentialPolicy{}).Error
}
