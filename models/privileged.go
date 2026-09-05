package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/jinzhu/gorm"
)

var (
	ErrReauthenticationRequired    = errors.New("fresh privileged reauthentication is required")
	ErrUnsupportedReauthentication = errors.New("unsupported reauthentication method")
)

type ReauthenticationProof struct {
	Method string
	Secret string
}

type ReauthenticationResult struct {
	UpgradedPasswordHash string
}

// Reauthenticator is independent of the privileged-session representation so
// WebAuthn, OIDC MFA, or TOTP proofs can be added without changing reveal code.
type Reauthenticator interface {
	Reauthenticate(context.Context, User, ReauthenticationProof) (ReauthenticationResult, error)
}

type PasswordReauthenticator struct{}

func (PasswordReauthenticator) Reauthenticate(_ context.Context, user User, proof ReauthenticationProof) (ReauthenticationResult, error) {
	if proof.Method != "password" {
		return ReauthenticationResult{}, ErrUnsupportedReauthentication
	}
	upgraded, err := auth.ValidatePasswordWithUpgrade(proof.Secret, user.Hash)
	if err != nil {
		return ReauthenticationResult{}, err
	}
	return ReauthenticationResult{UpgradedPasswordHash: upgraded}, nil
}

type PrivilegedSession struct {
	ID                int64     `json:"-"`
	SessionHash       string    `json:"-"`
	UserID            int64     `json:"-"`
	ReauthenticatedAt time.Time `json:"reauthenticated_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	CreatedAt         time.Time `json:"created_at"`
}

func NewSessionBinding() (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func sessionBindingHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func privilegedWindow() time.Duration {
	minutes := config.DefaultPrivilegedWindowMinutes
	if conf != nil && conf.PrivilegedAccess.WindowMinutes != 0 {
		minutes = conf.PrivilegedAccess.WindowMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func ReauthenticatePrivileged(ctx context.Context, user User, sessionBinding string, proof ReauthenticationProof, now time.Time) (PrivilegedSession, error) {
	var privileged PrivilegedSession
	if err := auth.CheckAccountState(user.AccountLocked, user.PasswordChangeRequired, false); err != nil {
		return privileged, err
	}
	if sessionBinding == "" {
		return privileged, ErrReauthenticationRequired
	}
	result, err := (PasswordReauthenticator{}).Reauthenticate(ctx, user, proof)
	if err != nil {
		return privileged, err
	}
	now = now.UTC()
	privileged = PrivilegedSession{
		SessionHash: sessionBindingHash(sessionBinding), UserID: user.Id,
		ReauthenticatedAt: now, ExpiresAt: now.Add(privilegedWindow()), CreatedAt: now,
	}
	err = withSecurityTransaction(func(tx *gorm.DB) error {
		if result.UpgradedPasswordHash != "" {
			if err := tx.Model(&User{}).Where("id=?", user.Id).Update("hash", result.UpgradedPasswordHash).Error; err != nil {
				return err
			}
		}
		var existing PrivilegedSession
		lookup := tx.Where("session_hash=?", privileged.SessionHash).First(&existing)
		if lookup.Error == nil {
			privileged.ID = existing.ID
			privileged.CreatedAt = existing.CreatedAt
		} else if lookup.Error != gorm.ErrRecordNotFound {
			return lookup.Error
		}
		return tx.Save(&privileged).Error
	})
	return privileged, err
}

func IsPrivilegedSessionFresh(userID int64, sessionBinding string, now time.Time) (bool, error) {
	if sessionBinding == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&PrivilegedSession{}).
		Where("session_hash=? AND user_id=? AND expires_at>?", sessionBindingHash(sessionBinding), userID, now.UTC()).Count(&count).Error
	return count == 1, err
}

func RevokePrivilegedSession(sessionBinding string) error {
	if sessionBinding == "" {
		return nil
	}
	return db.Where("session_hash=?", sessionBindingHash(sessionBinding)).Delete(&PrivilegedSession{}).Error
}

func RevokeUserPrivilegedSessions(userID int64) error {
	return db.Where("user_id=?", userID).Delete(&PrivilegedSession{}).Error
}

func DeleteExpiredPrivilegedSessions(now time.Time) (int64, error) {
	query := db.Where("expires_at<=?", now.UTC()).Delete(&PrivilegedSession{})
	return query.RowsAffected, query.Error
}
