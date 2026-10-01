package models

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"gorm.io/gorm"
)

var patAdmission struct {
	sync.Mutex
	database  *gorm.DB
	refreshAt time.Time
	tokens    map[string]PersonalAccessToken
}

func invalidatePATAdmission() {
	patAdmission.Lock()
	defer patAdmission.Unlock()
	patAdmission.refreshAt = time.Time{}
}

// RecognizedPATForAdmission only reserves rate-limit capacity. It never
// authenticates or authorizes a request: callers must still perform full live
// token, account, revocation and scope checks. This bounded-refresh snapshot
// stores persisted digests, never raw tokens or attacker-created entries.
func RecognizedPATForAdmission(raw string) bool {
	prefix, secret, ok := parsePAT(raw)
	if !ok {
		return false
	}
	patAdmission.Lock()
	defer patAdmission.Unlock()
	now := time.Now().UTC()
	if patAdmission.database != db || !now.Before(patAdmission.refreshAt) {
		patAdmission.database = db
		patAdmission.refreshAt = now.Add(time.Minute)
		patAdmission.tokens = make(map[string]PersonalAccessToken)
		var tokens []PersonalAccessToken
		if db == nil || db.Select("prefix", "token_hash", "expires_at").Where("revoked_at IS NULL AND expires_at > ?", now).Find(&tokens).Error != nil {
			return false
		}
		for _, token := range tokens {
			patAdmission.tokens[token.Prefix] = token
		}
	}
	token, ok := patAdmission.tokens[prefix]
	if !ok || !token.ExpiresAt.After(now) {
		return false
	}
	expected, err := hex.DecodeString(token.TokenHash)
	if err != nil {
		return false
	}
	actual := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(expected, actual[:]) == 1
}
