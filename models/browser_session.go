package models

import (
	"errors"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"gorm.io/gorm"
)

type BrowserSession struct {
	ID          int64     `json:"-"`
	SessionHash string    `json:"-"`
	UserID      int64     `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func browserSessionLifetime() time.Duration {
	hours := config.DefaultSessionLifetimeHours
	if conf != nil && conf.Session.LifetimeHours > 0 {
		hours = conf.Session.LifetimeHours
	}
	return time.Duration(hours) * time.Hour
}

func createBrowserSessionWithDB(tx *gorm.DB, userID int64, binding string, now time.Time) error {
	if binding == "" {
		return errors.New("browser session binding is required")
	}
	now = now.UTC()
	return tx.Create(&BrowserSession{
		SessionHash: sessionBindingHash(binding),
		UserID:      userID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(browserSessionLifetime()),
	}).Error
}

func CreateBrowserSession(userID int64, binding string, now time.Time) error {
	return createBrowserSessionWithDB(db, userID, binding, now)
}

func RotateBrowserSession(oldBinding string, userID int64, newBinding string, now time.Time) error {
	if oldBinding == "" {
		return errors.New("existing browser session binding is required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("session_hash=?", sessionBindingHash(oldBinding)).Delete(&BrowserSession{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("existing browser session was not active")
		}
		return createBrowserSessionWithDB(tx, userID, newBinding, now)
	})
}

func IsBrowserSessionActive(userID int64, binding string, now time.Time) (bool, error) {
	if binding == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&BrowserSession{}).
		Where("session_hash=? AND user_id=? AND expires_at>?", sessionBindingHash(binding), userID, now.UTC()).
		Count(&count).Error
	return count == 1, err
}

func RevokeBrowserSession(binding string) error {
	if binding == "" {
		return nil
	}
	return db.Where("session_hash=?", sessionBindingHash(binding)).Delete(&BrowserSession{}).Error
}

func RevokeUserBrowserSessions(userID int64) error {
	return db.Where("user_id=?", userID).Delete(&BrowserSession{}).Error
}

func PutUserAndRotateBrowserSession(user *User, binding string, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(user).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id=?", user.Id).Delete(&BrowserSession{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id=?", user.Id).Delete(&PrivilegedSession{}).Error; err != nil {
			return err
		}
		return createBrowserSessionWithDB(tx, user.Id, binding, now)
	})
}

func DeleteExpiredBrowserSessions(now time.Time) (int64, error) {
	query := db.Where("expires_at<=?", now.UTC()).Delete(&BrowserSession{})
	return query.RowsAffected, query.Error
}
