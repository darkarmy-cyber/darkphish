package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	LeaseSchema         = "darkphish-license-lease/v1"
	ProductDarkphish    = "darkphish"
	EditionCommunity    = "community"
	EditionProfessional = "professional"
	EditionEnterprise   = "enterprise"
	AlgorithmEd25519    = "Ed25519"
)

const Unlimited = -1

var (
	ErrInvalidEnvelope      = errors.New("invalid license envelope")
	ErrUnknownSigningKey    = errors.New("unknown license signing key")
	ErrUnsupportedAlgorithm = errors.New("unsupported license signature algorithm")
	ErrInvalidSignature     = errors.New("invalid license signature")
	ErrInvalidLease         = errors.New("invalid license lease")
)

type State string

const (
	StateActive  State = "active"
	StateGrace   State = "grace"
	StateExpired State = "expired"
	StateInvalid State = "invalid"
	StateMissing State = "missing"
)

type Entitlements struct {
	ManagedUsers    int `json:"managed_users"`
	ActiveCampaigns int `json:"active_campaigns"`
}

type Lease struct {
	Schema         string       `json:"schema"`
	Product        string       `json:"product"`
	Edition        string       `json:"edition"`
	LicenseID      string       `json:"license_id"`
	InstallationID string       `json:"installation_id"`
	IssuedAt       int64        `json:"issued_at"`
	NotBefore      int64        `json:"not_before"`
	ExpiresAt      int64        `json:"expires_at"`
	GraceUntil     int64        `json:"grace_until"`
	Entitlements   Entitlements `json:"entitlements"`
}

type Envelope struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type Verifier struct {
	keys      map[string]ed25519.PublicKey
	clockSkew time.Duration
}

func NewVerifier(keys map[string]ed25519.PublicKey, clockSkew time.Duration) *Verifier {
	copied := make(map[string]ed25519.PublicKey, len(keys))
	for id, key := range keys {
		copied[id] = append(ed25519.PublicKey(nil), key...)
	}
	return &Verifier{keys: copied, clockSkew: clockSkew}
}

func (v *Verifier) VerifyEnvelope(raw []byte, expectedInstallationID string, now time.Time) (Lease, State, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Lease{}, StateMissing, nil
	}

	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Lease{}, StateInvalid, fmt.Errorf("%w: decode envelope: %v", ErrInvalidEnvelope, err)
	}
	if envelope.KeyID == "" || envelope.Payload == "" || envelope.Signature == "" {
		return Lease{}, StateInvalid, fmt.Errorf("%w: required envelope field missing", ErrInvalidEnvelope)
	}
	if envelope.Algorithm != AlgorithmEd25519 {
		return Lease{}, StateInvalid, fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, envelope.Algorithm)
	}

	key, ok := v.keys[envelope.KeyID]
	if !ok || len(key) != ed25519.PublicKeySize {
		return Lease{}, StateInvalid, fmt.Errorf("%w: %s", ErrUnknownSigningKey, envelope.KeyID)
	}
	payload, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return Lease{}, StateInvalid, fmt.Errorf("%w: decode payload: %v", ErrInvalidEnvelope, err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Lease{}, StateInvalid, fmt.Errorf("%w: decode signature", ErrInvalidEnvelope)
	}
	if !ed25519.Verify(key, payload, signature) {
		return Lease{}, StateInvalid, ErrInvalidSignature
	}

	var lease Lease
	if err := json.Unmarshal(payload, &lease); err != nil {
		return Lease{}, StateInvalid, fmt.Errorf("%w: decode payload: %v", ErrInvalidLease, err)
	}
	if err := validateLease(lease, expectedInstallationID); err != nil {
		return Lease{}, StateInvalid, err
	}
	return lease, leaseState(lease, now, v.clockSkew), nil
}

func validateLease(lease Lease, expectedInstallationID string) error {
	switch {
	case lease.Schema != LeaseSchema:
		return fmt.Errorf("%w: unsupported schema", ErrInvalidLease)
	case lease.Product != ProductDarkphish:
		return fmt.Errorf("%w: product mismatch", ErrInvalidLease)
	case lease.Edition != EditionCommunity && lease.Edition != EditionProfessional && lease.Edition != EditionEnterprise:
		return fmt.Errorf("%w: edition mismatch", ErrInvalidLease)
	case strings.TrimSpace(lease.LicenseID) == "":
		return fmt.Errorf("%w: missing license id", ErrInvalidLease)
	case strings.TrimSpace(lease.InstallationID) == "":
		return fmt.Errorf("%w: missing installation id", ErrInvalidLease)
	case expectedInstallationID != "" && lease.InstallationID != expectedInstallationID:
		return fmt.Errorf("%w: installation mismatch", ErrInvalidLease)
	case lease.IssuedAt <= 0 || lease.NotBefore <= 0 || lease.ExpiresAt <= 0 || lease.GraceUntil <= 0:
		return fmt.Errorf("%w: invalid lease timestamps", ErrInvalidLease)
	case lease.NotBefore < lease.IssuedAt:
		return fmt.Errorf("%w: not_before precedes issued_at", ErrInvalidLease)
	case lease.ExpiresAt < lease.NotBefore:
		return fmt.Errorf("%w: expires_at precedes not_before", ErrInvalidLease)
	case lease.GraceUntil < lease.ExpiresAt:
		return fmt.Errorf("%w: grace_until precedes expires_at", ErrInvalidLease)
	case lease.Entitlements.ManagedUsers < 1 && !(lease.Entitlements.ManagedUsers == Unlimited && lease.Edition != EditionCommunity):
		return fmt.Errorf("%w: invalid managed-user entitlement", ErrInvalidLease)
	case lease.Entitlements.ActiveCampaigns < 1 && !(lease.Entitlements.ActiveCampaigns == Unlimited && lease.Edition != EditionCommunity):
		return fmt.Errorf("%w: invalid active-campaign entitlement", ErrInvalidLease)
	}
	return nil
}

func leaseState(lease Lease, now time.Time, skew time.Duration) State {
	n := now.UTC()
	notBefore := time.Unix(lease.NotBefore, 0).UTC()
	expiresAt := time.Unix(lease.ExpiresAt, 0).UTC()
	graceUntil := time.Unix(lease.GraceUntil, 0).UTC()

	if n.Add(skew).Before(notBefore) {
		return StateInvalid
	}
	if !n.After(expiresAt) {
		return StateActive
	}
	if !n.After(graceUntil) {
		return StateGrace
	}
	return StateExpired
}

func (s State) AllowsExpansion() bool {
	return s == StateActive || s == StateGrace
}
