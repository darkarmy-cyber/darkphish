// Package secrets provides storage-independent protection for integration
// credentials. Ciphertexts are versioned so another backend (for example a
// KMS or Vault implementation) can be introduced without changing models.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const encryptedPrefix = "darkphish:secret:v1:"

var (
	// ErrInvalidKey is returned when AES-256-GCM key material is not 32 bytes.
	ErrInvalidKey = errors.New("secret encryption key must contain exactly 32 bytes")
	// ErrInvalidCiphertext is returned for malformed or unauthentic ciphertext.
	ErrInvalidCiphertext = errors.New("invalid encrypted secret")
)

// Store protects and retrieves a secret value. Implementations must never log
// plaintext or key material.
type Store interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}

// PlaintextStore is an explicit development-only compatibility store. The
// application never selects it in production mode.
type PlaintextStore struct{}

func (PlaintextStore) Seal(value string) (string, error) { return value, nil }
func (PlaintextStore) Open(value string) (string, error) { return value, nil }

type aesGCMStore struct {
	aead cipher.AEAD
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

// NewAESGCM creates an AES-256-GCM-backed secret store.
func NewAESGCM(key []byte) (Store, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &aesGCMStore{aead: aead}, nil
}

func (s *aesGCMStore) Seal(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(value), []byte(encryptedPrefix))
	return encryptedPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *aesGCMStore) Open(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	// Existing installations may contain plaintext. Reading it preserves
	// compatibility; the next write transparently migrates it to ciphertext.
	if !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	encoded := strings.TrimPrefix(value, encryptedPrefix)
	sealed, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < s.aead.NonceSize() {
		return "", ErrInvalidCiphertext
	}
	nonce, ciphertext := sealed[:s.aead.NonceSize()], sealed[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(encryptedPrefix))
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plaintext), nil
}
