package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	ErrProviderUnavailable = errors.New("external key provider is unavailable")
	ErrWrongProvider       = errors.New("encrypted secret references an unavailable provider")
)

type KeyReference struct {
	Provider string
	KeyID    string
}

type KeyProvider interface {
	Name() string
	ActiveKey(context.Context) (KeyReference, error)
	EncryptDataKey(context.Context, KeyReference, []byte) ([]byte, error)
	DecryptDataKey(context.Context, KeyReference, []byte) ([]byte, error)
}

type providerEnvelope struct {
	EnvelopeVersion int       `json:"envelope_version"`
	Provider        string    `json:"provider"`
	KeyReference    string    `json:"key_reference"`
	WrappedDataKey  string    `json:"wrapped_data_key"`
	Nonce           string    `json:"nonce"`
	Ciphertext      string    `json:"ciphertext"`
	Algorithm       string    `json:"algorithm"`
	CreatedAt       time.Time `json:"created_at"`
}

type RoutingStore struct {
	activeProvider string
	local          VersionedStore
	providers      map[string]KeyProvider
}

func NewRoutingStore(activeProvider string, local VersionedStore, providers ...KeyProvider) (VersionedStore, error) {
	routing := &RoutingStore{activeProvider: activeProvider, local: local, providers: make(map[string]KeyProvider)}
	for _, provider := range providers {
		if provider == nil || !keyIDPattern.MatchString(provider.Name()) {
			return nil, errors.New("invalid external key provider")
		}
		routing.providers[provider.Name()] = provider
	}
	if activeProvider == "local" {
		if local == nil {
			return nil, errors.New("local key provider is unavailable")
		}
	} else if _, ok := routing.providers[activeProvider]; !ok {
		return nil, fmt.Errorf("active key provider %q is unavailable", activeProvider)
	}
	return routing, nil
}

func (s *RoutingStore) ActiveKeyID() string {
	if s.activeProvider == "local" {
		return "local:" + s.local.ActiveKeyID()
	}
	provider := s.providers[s.activeProvider]
	reference, err := provider.ActiveKey(context.Background())
	if err != nil {
		return s.activeProvider + ":unavailable"
	}
	return reference.Provider + ":" + reference.KeyID
}

func (s *RoutingStore) NeedsRewrap(value string) bool {
	if value == "" {
		return false
	}
	if s.activeProvider == "local" {
		return strings.HasPrefix(value, providerPrefix) || s.local.NeedsRewrap(value)
	}
	if !strings.HasPrefix(value, providerPrefix) {
		return true
	}
	envelope, err := decodeProviderEnvelope(value)
	if err != nil {
		return true
	}
	reference, err := s.providers[s.activeProvider].ActiveKey(context.Background())
	return err != nil || envelope.Provider != reference.Provider || envelope.KeyReference != reference.KeyID
}

func (s *RoutingStore) Seal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if s.activeProvider == "local" {
		return s.local.Seal(value)
	}
	provider := s.providers[s.activeProvider]
	reference, err := provider.ActiveKey(context.Background())
	if err != nil {
		return "", ErrProviderUnavailable
	}
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return "", err
	}
	wrapped, err := provider.EncryptDataKey(context.Background(), reference, dataKey)
	if err != nil {
		clear(dataKey)
		return "", err
	}
	aead, err := newAEAD(dataKey)
	clear(dataKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	aad := []byte(providerPrefix + reference.Provider + ":" + reference.KeyID + ":" + base64.RawStdEncoding.EncodeToString(wrapped))
	ciphertext := aead.Seal(nil, nonce, []byte(value), aad)
	envelope := providerEnvelope{
		EnvelopeVersion: 3, Provider: reference.Provider, KeyReference: reference.KeyID,
		WrappedDataKey: base64.RawStdEncoding.EncodeToString(wrapped), Nonce: base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext), Algorithm: algorithmAES256GCM, CreatedAt: time.Now().UTC(),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return providerPrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeProviderEnvelope(value string) (providerEnvelope, error) {
	var envelope providerEnvelope
	if !strings.HasPrefix(value, providerPrefix) || len(value) > 1<<20 {
		return envelope, ErrInvalidCiphertext
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, providerPrefix))
	if err != nil || json.Unmarshal(raw, &envelope) != nil || envelope.EnvelopeVersion != 3 || envelope.Algorithm != algorithmAES256GCM ||
		!keyIDPattern.MatchString(envelope.Provider) || !keyIDPattern.MatchString(envelope.KeyReference) {
		return envelope, ErrInvalidCiphertext
	}
	return envelope, nil
}

func (s *RoutingStore) Open(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, providerPrefix) {
		if s.local == nil {
			return "", ErrUnknownKey
		}
		return s.local.Open(value)
	}
	envelope, err := decodeProviderEnvelope(value)
	if err != nil {
		return "", err
	}
	provider, ok := s.providers[envelope.Provider]
	if !ok {
		return "", ErrWrongProvider
	}
	wrapped, wrappedErr := base64.RawStdEncoding.DecodeString(envelope.WrappedDataKey)
	nonce, nonceErr := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	ciphertext, ciphertextErr := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if wrappedErr != nil || nonceErr != nil || ciphertextErr != nil || len(wrapped) == 0 {
		return "", ErrInvalidCiphertext
	}
	reference := KeyReference{Provider: envelope.Provider, KeyID: envelope.KeyReference}
	dataKey, err := provider.DecryptDataKey(context.Background(), reference, wrapped)
	if err != nil {
		return "", err
	}
	aead, err := newAEAD(dataKey)
	clear(dataKey)
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", ErrInvalidCiphertext
	}
	aad := []byte(providerPrefix + envelope.Provider + ":" + envelope.KeyReference + ":" + envelope.WrappedDataKey)
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plaintext), nil
}
