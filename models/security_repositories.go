package models

import (
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gorm.io/gorm"
)

// CampaignRepository isolates campaign authorization from query details.
type CampaignRepository interface {
	FindCampaign(id int64) (Campaign, error)
	IsOwner(campaignID, userID int64) (bool, error)
}

// CredentialRepository persists policy findings and protected credentials.
type CredentialRepository interface {
	FindEncrypted(resultID int64) (EncryptedCredential, error)
	ReplaceFinding(CredentialPolicyResult) error
	ReplaceEncrypted(EncryptedCredential) error
	PurgeEncrypted(resultID int64, now time.Time) error
	PurgeCredential(id int64, now time.Time) error
}

// TokenRepository persists and revokes personal access tokens.
type TokenRepository interface {
	CreateToken(*PersonalAccessToken) error
	RevokeToken(id, userID int64, at time.Time) (bool, error)
}

// AuditRepository durably enqueues security events within a transaction.
type AuditRepository interface {
	Enqueue(audit.Event) error
}

// ReviewerRepository persists campaign-scoped reviewer assignments.
type ReviewerRepository interface {
	Find(campaignID, userID int64) (CampaignReviewer, error)
	List(campaignID int64) ([]CampaignReviewer, error)
	Save(*CampaignReviewer) error
	Delete(campaignID, userID int64) (bool, error)
	HasActive(campaignID, userID int64, now time.Time) (bool, error)
}

type gormCampaignRepository struct{ db *gorm.DB }

func (r gormCampaignRepository) FindCampaign(id int64) (Campaign, error) {
	var campaign Campaign
	err := r.db.Where("id=?", id).First(&campaign).Error
	return campaign, err
}

func (r gormCampaignRepository) IsOwner(campaignID, userID int64) (bool, error) {
	var count int64
	err := r.db.Model(&Campaign{}).Where("id=? AND user_id=?", campaignID, userID).Count(&count).Error
	return count == 1, err
}

type gormCredentialRepository struct{ db *gorm.DB }

func (r gormCredentialRepository) FindEncrypted(resultID int64) (EncryptedCredential, error) {
	var value EncryptedCredential
	err := r.db.Where("result_id=?", resultID).First(&value).Error
	return value, err
}

func (r gormCredentialRepository) ReplaceFinding(value CredentialPolicyResult) error {
	if err := r.db.Where("result_id=?", value.ResultID).Delete(&CredentialPolicyResult{}).Error; err != nil {
		return err
	}
	return r.db.Save(&value).Error
}

func (r gormCredentialRepository) ReplaceEncrypted(value EncryptedCredential) error {
	if err := r.db.Where("result_id=?", value.ResultID).Delete(&EncryptedCredential{}).Error; err != nil {
		return err
	}
	return r.db.Save(&value).Error
}

func (r gormCredentialRepository) PurgeEncrypted(resultID int64, now time.Time) error {
	return r.db.Model(&EncryptedCredential{}).Where("result_id=?", resultID).
		Updates(map[string]interface{}{"encrypted_value": "", "purged_at": now.UTC()}).Error
}

func (r gormCredentialRepository) PurgeCredential(id int64, now time.Time) error {
	return r.db.Model(&EncryptedCredential{}).Where("id=?", id).
		Updates(map[string]interface{}{"encrypted_value": "", "purged_at": now.UTC()}).Error
}

type gormTokenRepository struct{ db *gorm.DB }

func (r gormTokenRepository) CreateToken(value *PersonalAccessToken) error {
	return r.db.Save(value).Error
}

func (r gormTokenRepository) RevokeToken(id, userID int64, at time.Time) (bool, error) {
	query := r.db.Model(&PersonalAccessToken{}).Where("id=? AND user_id=? AND revoked_at IS NULL", id, userID).Update("revoked_at", at.UTC())
	return query.RowsAffected == 1, query.Error
}

type gormAuditRepository struct{ db *gorm.DB }

func (r gormAuditRepository) Enqueue(event audit.Event) error {
	return enqueueAuditEvent(r.db, event)
}

type gormReviewerRepository struct{ db *gorm.DB }

func (r gormReviewerRepository) Find(campaignID, userID int64) (CampaignReviewer, error) {
	var assignment CampaignReviewer
	err := r.db.Where("campaign_id=? AND user_id=?", campaignID, userID).First(&assignment).Error
	return assignment, err
}

func (r gormReviewerRepository) List(campaignID int64) ([]CampaignReviewer, error) {
	values := []CampaignReviewer{}
	err := r.db.Table("campaign_reviewers cr").
		Select("cr.*, users.username AS reviewer_username, assigners.username AS assigned_by_username").
		Joins("JOIN users ON users.id=cr.user_id").
		Joins("JOIN users assigners ON assigners.id=cr.assigned_by").
		Where("cr.campaign_id=?", campaignID).Order("cr.assigned_at ASC, cr.id ASC").Scan(&values).Error
	return values, err
}

func (r gormReviewerRepository) Save(value *CampaignReviewer) error { return r.db.Save(value).Error }

func (r gormReviewerRepository) Delete(campaignID, userID int64) (bool, error) {
	query := r.db.Where("campaign_id=? AND user_id=?", campaignID, userID).Delete(&CampaignReviewer{})
	return query.RowsAffected == 1, query.Error
}

func (r gormReviewerRepository) HasActive(campaignID, userID int64, now time.Time) (bool, error) {
	var count int64
	err := r.db.Model(&CampaignReviewer{}).
		Where("campaign_id=? AND user_id=? AND (expires_at IS NULL OR expires_at>?)", campaignID, userID, now.UTC()).Count(&count).Error
	return count > 0, err
}

func withSecurityTransaction(fn func(*gorm.DB) error) error {
	// GORM v2 rolls back on returned errors, panic and failed commit; a fresh
	// transaction statement prevents predicates from leaking between writes.
	return db.Transaction(fn)
}
