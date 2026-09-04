// Package secrets provides storage-independent protection for credentials.
// Ciphertexts carry an envelope version and key identifier so keys can rotate
// without downtime or ambiguous decryption attempts.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	legacyEncryptedPrefix = "darkphish:secret:v1:"
	encryptedPrefix       = "darkphish:secret:v2:"
	algorithmAES256GCM    = "AES-256-GCM"
)

var (
	ErrInvalidKey        = errors.New("secret encryption key must contain exactly 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid encrypted secret")
	ErrUnknownKey        = errors.New("encrypted secret references an unavailable key")
	keyIDPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Store interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}

type VersionedStore interface {
	Store
	ActiveKeyID() string
	NeedsRewrap(string) bool
}

// PlaintextStore is development compatibility only. It is never accepted for
// encrypted credential review.
type PlaintextStore struct{}

func (PlaintextStore) Seal(value string) (string, error) { return value, nil }
func (PlaintextStore) Open(value string) (string, error) { return value, nil }

type keyringStore struct {
	activeKeyID string
	keys        map[string]cipher.AEAD
	legacy      cipher.AEAD
}

type envelope struct {
	Version    int    `json:"version"`
	KeyID      string `json:"key_id"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// IsCiphertext reports whether a value uses a recognized Darkphish encrypted
// envelope. It does not validate or decrypt the envelope.
func IsCiphertext(value string) bool {
	return strings.HasPrefix(value, encryptedPrefix) || strings.HasPrefix(value, legacyEncryptedPrefix)
}

// DecodeKey accepts raw key material or values prefixed with base64: or hex:.
func DecodeKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "base64:") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("decode base64 key: %w", err)
		}
		return decoded, nil
	}
	if strings.HasPrefix(value, "hex:") {
		decoded, err := hex.DecodeString(strings.TrimPrefix(value, "hex:"))
		if err != nil {
			return nil, fmt.Errorf("decode hexadecimal key: %w", err)
		}
		return decoded, nil
	}
	return []byte(value), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// NewAESGCM creates a single-key store that writes v2 envelopes while reading
// historical v1 ciphertexts generated with the same key.
func NewAESGCM(key []byte) (Store, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return NewKeyring("default", map[string][]byte{"default": key}, key)
}

// NewKeyring creates a versioned AES-256-GCM store. legacyKey may be nil; when
// present it is used only to read v1 ciphertexts that predate key identifiers.
func NewKeyring(activeKeyID string, keys map[string][]byte, legacyKey []byte) (VersionedStore, error) {
	if !keyIDPattern.MatchString(activeKeyID) {
		return nil, fmt.Errorf("invalid active key id %q", activeKeyID)
	}
	store := &keyringStore{activeKeyID: activeKeyID, keys: make(map[string]cipher.AEAD, len(keys))}
	for id, key := range keys {
		if !keyIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid key id %q", id)
		}
		aead, err := newAEAD(key)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", id, err)
		}
		store.keys[id] = aead
	}
	if _, ok := store.keys[activeKeyID]; !ok {
		return nil, fmt.Errorf("active key %q is not configured", activeKeyID)
	}
	if len(legacyKey) > 0 {
		var err error
		store.legacy, err = newAEAD(legacyKey)
		if err != nil {
			return nil, fmt.Errorf("legacy key: %w", err)
		}
	}
	return store, nil
}

func (s *keyringStore) ActiveKeyID() string { return s.activeKeyID }

func (s *keyringStore) NeedsRewrap(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, legacyEncryptedPrefix) || !strings.HasPrefix(value, encryptedPrefix) {
		return true
	}
	env, err := decodeEnvelope(value)
	return err != nil || env.KeyID != s.activeKeyID
}

func (s *keyringStore) Seal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	aead := s.keys[s.activeKeyID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(value), []byte(encryptedPrefix+s.activeKeyID))
	env := envelope{
		Version:    2,
		KeyID:      s.activeKeyID,
		Algorithm:  algorithmAES256GCM,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	encoded, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return encryptedPrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeEnvelope(value string) (envelope, error) {
	var env envelope
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrefix))
	if err != nil || json.Unmarshal(raw, &env) != nil || env.Version != 2 || env.Algorithm != algorithmAES256GCM || !keyIDPattern.MatchString(env.KeyID) {
		return env, ErrInvalidCiphertext
	}
	return env, nil
}

func (s *keyringStore) Open(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	// Existing integration rows may contain plaintext. RotateSecrets rewrites
	// them into a v2 envelope; credential-review rows never use this exception.
	if !IsCiphertext(value) {
		return value, nil
	}
	if strings.HasPrefix(value, legacyEncryptedPrefix) {
		if s.legacy == nil {
			return "", ErrUnknownKey
		}
		sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, legacyEncryptedPrefix))
		if err != nil || len(sealed) < s.legacy.NonceSize() {
			return "", ErrInvalidCiphertext
		}
		nonce, ciphertext := sealed[:s.legacy.NonceSize()], sealed[s.legacy.NonceSize():]
		plaintext, err := s.legacy.Open(nil, nonce, ciphertext, []byte(legacyEncryptedPrefix))
		if err != nil {
			return "", ErrInvalidCiphertext
		}
		return string(plaintext), nil
	}
	env, err := decodeEnvelope(value)
	if err != nil {
		return "", err
	}
	aead, ok := s.keys[env.KeyID]
	if !ok {
		return "", ErrUnknownKey
	}
	nonce, nonceErr := base64.RawStdEncoding.DecodeString(env.Nonce)
	ciphertext, ciphertextErr := base64.RawStdEncoding.DecodeString(env.Ciphertext)
	if nonceErr != nil || ciphertextErr != nil || len(nonce) != aead.NonceSize() {
		return "", ErrInvalidCiphertext
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(encryptedPrefix+env.KeyID))
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plaintext), nil
}
