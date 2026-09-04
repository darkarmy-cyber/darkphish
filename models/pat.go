package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
)

const patTokenPrefix = "darkphish_pat_"

var (
	ErrInvalidPAT        = errors.New("invalid personal access token")
	ErrExpiredPAT        = errors.New("personal access token has expired")
	ErrRevokedPAT        = errors.New("personal access token has been revoked")
	ErrInvalidPATScope   = errors.New("invalid personal access token scope")
	ErrPATExpiryRequired = errors.New("personal access token expiry is required")
)

var allowedPATScopes = map[string]struct{}{
	"audit:read": {}, "campaigns:read": {}, "campaigns:write": {},
	"credentials:view": {}, "groups:read": {}, "groups:write": {},
	"integrations:read": {}, "integrations:write": {},
	"landing-pages:read": {}, "landing-pages:write": {},
	"reports:read": {}, "sending-profiles:read": {}, "sending-profiles:write": {},
	"templates:read": {}, "templates:write": {}, "tokens:manage": {},
	"users:read": {}, "users:write": {},
}

type PersonalAccessToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	TokenHash  string     `json:"-" gorm:"column:token_hash"`
	ScopesRaw  string     `json:"-" gorm:"column:scopes"`
	Scopes     []string   `json:"scopes" gorm:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type PATAuthentication struct {
	User   User
	Token  PersonalAccessToken
	Scopes map[string]struct{}
}

func AllowedPATScopes() []string {
	values := make([]string, 0, len(allowedPATScopes))
	for scope := range allowedPATScopes {
		values = append(values, scope)
	}
	sort.Strings(values)
	return values
}

func normalizePATScopes(scopes []string) ([]string, error) {
	unique := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if _, ok := allowedPATScopes[scope]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidPATScope, scope)
		}
		unique[scope] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalidPATScope)
	}
	normalized := make([]string, 0, len(unique))
	for scope := range unique {
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func randomTokenPart(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashPATSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func parsePAT(raw string) (string, string, bool) {
	if !strings.HasPrefix(raw, patTokenPrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(raw, patTokenPrefix)
	prefix, secret, ok := strings.Cut(remainder, "_")
	if !ok || len(prefix) != 16 || len(secret) < 32 {
		return "", "", false
	}
	if _, err := hex.DecodeString(prefix); err != nil {
		return "", "", false
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(secret); err != nil || len(decoded) != 32 {
		return "", "", false
	}
	return prefix, secret, true
}

// CreatePersonalAccessToken returns the raw token exactly once. Only its
// SHA-256 digest is persisted; the secret has 256 bits of random entropy.
func CreatePersonalAccessToken(userID int64, name string, scopes []string, expiresAt time.Time) (PersonalAccessToken, string, error) {
	var pat PersonalAccessToken
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return pat, "", errors.New("personal access token name must be between 1 and 128 characters")
	}
	scopes, err := normalizePATScopes(scopes)
	if err != nil {
		return pat, "", err
	}
	now := time.Now().UTC()
	if expiresAt.IsZero() {
		return pat, "", ErrPATExpiryRequired
	}
	expiresAt = expiresAt.UTC()
	maxDays := configPATMaxLifetimeDays()
	if !expiresAt.After(now) || expiresAt.After(now.Add(time.Duration(maxDays)*24*time.Hour)) {
		return pat, "", fmt.Errorf("personal access token expiry must be within %d days", maxDays)
	}
	prefixBytes := make([]byte, 8)
	if _, err := rand.Read(prefixBytes); err != nil {
		return pat, "", err
	}
	prefix := hex.EncodeToString(prefixBytes)
	secret, err := randomTokenPart(32)
	if err != nil {
		return pat, "", err
	}
	raw := patTokenPrefix + prefix + "_" + secret
	encodedScopes, _ := json.Marshal(scopes)
	pat = PersonalAccessToken{
		UserID: userID, Name: name, Prefix: prefix, TokenHash: hashPATSecret(secret),
		ScopesRaw: string(encodedScopes), Scopes: scopes, CreatedAt: now, ExpiresAt: expiresAt,
	}
	if err := db.Save(&pat).Error; err != nil {
		return PersonalAccessToken{}, "", err
	}
	return pat, raw, nil
}

func configPATMaxLifetimeDays() int {
	if conf == nil || conf.PAT.MaxLifetimeDays == 0 {
		return 90
	}
	return conf.PAT.MaxLifetimeDays
}

func hydratePAT(pat *PersonalAccessToken) error {
	if err := json.Unmarshal([]byte(pat.ScopesRaw), &pat.Scopes); err != nil {
		return ErrInvalidPAT
	}
	return nil
}

func GetPersonalAccessTokens(userID int64) ([]PersonalAccessToken, error) {
	values := []PersonalAccessToken{}
	if err := db.Where("user_id=?", userID).Order("created_at DESC").Find(&values).Error; err != nil {
		return nil, err
	}
	for i := range values {
		if err := hydratePAT(&values[i]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func RevokePersonalAccessToken(id, userID int64) error {
	now := time.Now().UTC()
	query := db.Model(&PersonalAccessToken{}).Where("id=? AND user_id=? AND revoked_at IS NULL", id, userID).Update("revoked_at", now)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func AuthenticatePersonalAccessToken(raw string) (PATAuthentication, error) {
	var authentication PATAuthentication
	prefix, secret, ok := parsePAT(raw)
	if !ok {
		return authentication, ErrInvalidPAT
	}
	var pat PersonalAccessToken
	if err := db.Where("prefix=?", prefix).First(&pat).Error; err != nil {
		return authentication, ErrInvalidPAT
	}
	expected, err := hex.DecodeString(pat.TokenHash)
	if err != nil {
		return authentication, ErrInvalidPAT
	}
	actualDigest := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(expected, actualDigest[:]) != 1 {
		return authentication, ErrInvalidPAT
	}
	now := time.Now().UTC()
	if pat.RevokedAt != nil {
		return authentication, ErrRevokedPAT
	}
	if !pat.ExpiresAt.After(now) {
		return authentication, ErrExpiredPAT
	}
	if err := hydratePAT(&pat); err != nil {
		return authentication, err
	}
	user, err := GetUser(pat.UserID)
	if err != nil {
		return authentication, ErrInvalidPAT
	}
	if pat.LastUsedAt == nil || pat.LastUsedAt.Before(now.Add(-15*time.Minute)) {
		_ = db.Model(&PersonalAccessToken{}).Where("id=?", pat.ID).Update("last_used_at", now).Error
		pat.LastUsedAt = &now
	}
	scopeSet := make(map[string]struct{}, len(pat.Scopes))
	for _, scope := range pat.Scopes {
		scopeSet[scope] = struct{}{}
	}
	return PATAuthentication{User: user, Token: pat, Scopes: scopeSet}, nil
}
