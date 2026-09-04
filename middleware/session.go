package middleware

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/secrets"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

const CookieName = "darkphish"

var ErrWeakSessionKey = errors.New("session key material is too short")

// init registers the necessary models to be saved in the session later
func init() {
	gob.Register(&models.User{})
	gob.Register(&models.Flash{})
	setSessionOptions(Store, false, 24)
}

// Store contains the session information for the request
var Store = sessions.NewCookieStore(
	[]byte(securecookie.GenerateRandomKey(64)), //Signing key
	[]byte(securecookie.GenerateRandomKey(32)))

func setSessionOptions(store *sessions.CookieStore, secure bool, lifetimeHours int) {
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int((time.Duration(lifetimeHours) * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ConfigureSession replaces the process-local development store with one
// backed by administrator-supplied persistent keys. Production mode never
// falls back to randomly generated keys.
func ConfigureSession(authValue, encryptionValue string, secure bool, lifetimeHours int, production bool) error {
	if authValue == "" || encryptionValue == "" {
		if production {
			return errors.New("persistent session keys are required in production mode")
		}
		Store = sessions.NewCookieStore(securecookie.GenerateRandomKey(64), securecookie.GenerateRandomKey(32))
		setSessionOptions(Store, secure, lifetimeHours)
		return nil
	}
	authKey, err := secrets.DecodeKey(authValue)
	if err != nil {
		return fmt.Errorf("decode session authentication key: %w", err)
	}
	encryptionKey, err := secrets.DecodeKey(encryptionValue)
	if err != nil {
		return fmt.Errorf("decode session encryption key: %w", err)
	}
	if len(authKey) < 32 || len(encryptionKey) != 32 {
		return ErrWeakSessionKey
	}
	Store = sessions.NewCookieStore(authKey, encryptionKey)
	setSessionOptions(Store, secure, lifetimeHours)
	return nil
}
