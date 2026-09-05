package models

import (
	"errors"
	"strconv"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/jinzhu/gorm"
)

var (
	ErrInvalidReviewerAssignment = errors.New("invalid reviewer assignment")
	ErrCampaignReviewDenied      = errors.New("credential review is not authorized for this campaign")
)

type CampaignReviewer struct {
	ID                 int64      `json:"id"`
	CampaignID         int64      `json:"campaign_id"`
	UserID             int64      `json:"user_id"`
	AssignedBy         int64      `json:"assigned_by"`
	AssignedAt         time.Time  `json:"assigned_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	ExpiredAuditedAt   *time.Time `json:"-"`
	ReviewerUsername   string     `json:"reviewer_username" gorm:"-"`
	AssignedByUsername string     `json:"assigned_by_username" gorm:"-"`
}

type ReviewerService struct {
	Campaigns CampaignRepository
	Reviewers ReviewerRepository
	Audit     AuditRepository
}

func reviewerServiceFor(database *gorm.DB) ReviewerService {
	return ReviewerService{
		Campaigns: gormCampaignRepository{db: database},
		Reviewers: gormReviewerRepository{db: database},
		Audit:     gormAuditRepository{db: database},
	}
}

func (s ReviewerService) canManage(campaignID int64, actor User) (bool, error) {
	hasSystem, err := actor.HasPermission(PermissionModifySystem)
	if err != nil {
		return false, err
	}
	if hasSystem {
		return true, nil
	}
	return s.Campaigns.IsOwner(campaignID, actor.Id)
}

func (s ReviewerService) Assign(campaignID, reviewerID int64, actor User, expiresAt *time.Time, event audit.Event) (CampaignReviewer, error) {
	var assignment CampaignReviewer
	allowed, err := s.canManage(campaignID, actor)
	if err != nil || !allowed {
		return assignment, ErrInvalidReviewerAssignment
	}
	reviewer, err := GetUser(reviewerID)
	if err != nil || reviewer.Role.Slug != RoleSecurityReviewer || reviewer.AccountLocked || reviewer.PasswordChangeRequired {
		return assignment, ErrInvalidReviewerAssignment
	}
	now := time.Now().UTC()
	if expiresAt != nil {
		value := expiresAt.UTC()
		if !value.After(now) || value.After(now.Add(366*24*time.Hour)) {
			return assignment, ErrInvalidReviewerAssignment
		}
		expiresAt = &value
	}
	err = withSecurityTransaction(func(tx *gorm.DB) error {
		txService := reviewerServiceFor(tx)
		existing, findErr := txService.Reviewers.Find(campaignID, reviewerID)
		if findErr == nil {
			assignment = existing
		} else if findErr != gorm.ErrRecordNotFound {
			return findErr
		}
		assignment.CampaignID = campaignID
		assignment.UserID = reviewerID
		assignment.AssignedBy = actor.Id
		assignment.AssignedAt = now
		assignment.ExpiresAt = expiresAt
		assignment.ExpiredAuditedAt = nil
		if err := txService.Reviewers.Save(&assignment); err != nil {
			return err
		}
		event.Action = "campaign.reviewer.assign"
		event.TargetType = "campaign"
		event.TargetID = strconv.FormatInt(campaignID, 10) + "/reviewers/" + strconv.FormatInt(reviewerID, 10)
		return txService.Audit.Enqueue(event)
	})
	if err == nil {
		flushAuditOutboxAfterCommit()
	}
	return assignment, err
}

func (s ReviewerService) Remove(campaignID, reviewerID int64, actor User, event audit.Event) error {
	allowed, err := s.canManage(campaignID, actor)
	if err != nil || !allowed {
		return ErrInvalidReviewerAssignment
	}
	err = withSecurityTransaction(func(tx *gorm.DB) error {
		removed, removeErr := reviewerServiceFor(tx).Reviewers.Delete(campaignID, reviewerID)
		if removeErr != nil {
			return removeErr
		}
		if !removed {
			return gorm.ErrRecordNotFound
		}
		event.Action = "campaign.reviewer.remove"
		event.TargetType = "campaign"
		event.TargetID = strconv.FormatInt(campaignID, 10) + "/reviewers/" + strconv.FormatInt(reviewerID, 10)
		return reviewerServiceFor(tx).Audit.Enqueue(event)
	})
	if err == nil {
		flushAuditOutboxAfterCommit()
	}
	return err
}

func (s ReviewerService) List(campaignID int64, actor User) ([]CampaignReviewer, error) {
	allowed, err := s.canManage(campaignID, actor)
	if err != nil || !allowed {
		return nil, ErrInvalidReviewerAssignment
	}
	values, err := s.Reviewers.List(campaignID)
	if err != nil {
		return nil, err
	}
	_, _ = AuditExpiredCampaignReviewers(time.Now().UTC())
	return values, nil
}

// AuditExpiredCampaignReviewers records each natural assignment expiry once.
// The timestamp marker and durable outbox record are committed atomically.
func AuditExpiredCampaignReviewers(now time.Time) (int64, error) {
	values := []CampaignReviewer{}
	if err := db.Where("expires_at IS NOT NULL AND expires_at<=? AND expired_audited_at IS NULL", now.UTC()).Find(&values).Error; err != nil {
		return 0, err
	}
	var audited int64
	for _, value := range values {
		err := withSecurityTransaction(func(tx *gorm.DB) error {
			updated := tx.Model(&CampaignReviewer{}).Where("id=? AND expired_audited_at IS NULL", value.ID).Update("expired_audited_at", now.UTC())
			if updated.Error != nil || updated.RowsAffected == 0 {
				return updated.Error
			}
			if err := (gormAuditRepository{db: tx}).Enqueue(audit.Event{
				Timestamp: now.UTC(), Actor: "darkphish", ActorType: "system", Action: "campaign.reviewer.expire",
				TargetType: "campaign", TargetID: strconv.FormatInt(value.CampaignID, 10) + "/reviewers/" + strconv.FormatInt(value.UserID, 10),
				Result: "success", RequestID: audit.NewRequestID(), AuthMethod: "system", Metadata: "{}",
			}); err != nil {
				return err
			}
			audited++
			return nil
		})
		if err != nil {
			return audited, err
		}
	}
	if audited > 0 {
		flushAuditOutboxAfterCommit()
	}
	return audited, nil
}

func AssignCampaignReviewer(campaignID, reviewerID int64, actor User, expiresAt *time.Time, event audit.Event) (CampaignReviewer, error) {
	return reviewerServiceFor(db).Assign(campaignID, reviewerID, actor, expiresAt, event)
}

func RemoveCampaignReviewer(campaignID, reviewerID int64, actor User, event audit.Event) error {
	return reviewerServiceFor(db).Remove(campaignID, reviewerID, actor, event)
}

func GetCampaignReviewers(campaignID int64, actor User) ([]CampaignReviewer, error) {
	return reviewerServiceFor(db).List(campaignID, actor)
}

func CanManageCampaignReviewers(campaignID int64, actor User) (bool, error) {
	return reviewerServiceFor(db).canManage(campaignID, actor)
}

func GetEligibleSecurityReviewers() ([]User, error) {
	role, err := GetRoleBySlug(RoleSecurityReviewer)
	if err != nil {
		return nil, err
	}
	values := []User{}
	err = db.Preload("Role").Where("role_id=? AND account_locked=? AND password_change_required=?", role.ID, false, false).
		Order("username ASC, id ASC").Find(&values).Error
	return values, err
}

func CanReviewCredential(user User, campaignID int64, now time.Time) (bool, error) {
	if err := auth.CheckAccountState(user.AccountLocked, user.PasswordChangeRequired, false); err != nil {
		return false, nil
	}
	permission, err := user.HasPermission(PermissionViewCredentials)
	if err != nil || !permission {
		return false, err
	}
	if user.Role.Slug == RoleAdmin {
		return true, nil
	}
	return (gormReviewerRepository{db: db}).HasActive(campaignID, user.Id, now.UTC())
}

func CanReadCampaign(user User, campaignID int64, now time.Time) (bool, error) {
	if err := auth.CheckAccountState(user.AccountLocked, user.PasswordChangeRequired, false); err != nil {
		return false, nil
	}
	if user.Role.Slug == RoleAdmin {
		return true, nil
	}
	owner, err := (gormCampaignRepository{db: db}).IsOwner(campaignID, user.Id)
	if err != nil || owner {
		return owner, err
	}
	read, err := user.HasPermission(PermissionCampaignsRead)
	if err != nil || !read {
		return false, err
	}
	return (gormReviewerRepository{db: db}).HasActive(campaignID, user.Id, now.UTC())
}

// GetAccessibleCampaigns returns owned campaigns for operators, all campaigns
// for administrators, and only actively assigned campaigns for reviewers.
func GetAccessibleCampaigns(user User, now time.Time) ([]Campaign, error) {
	values := []Campaign{}
	query := db.Model(&Campaign{})
	switch user.Role.Slug {
	case RoleAdmin:
	case RoleSecurityReviewer:
		query = query.Joins("JOIN campaign_reviewers cr ON cr.campaign_id=campaigns.id").
			Where("cr.user_id=? AND (cr.expires_at IS NULL OR cr.expires_at>?)", user.Id, now.UTC())
	default:
		query = query.Where("campaigns.user_id=?", user.Id)
	}
	if err := query.Order("campaigns.created_date DESC, campaigns.id DESC").Find(&values).Error; err != nil {
		return nil, err
	}
	for i := range values {
		if err := values[i].getDetails(); err != nil {
			return nil, err
		}
	}
	return values, nil
}
