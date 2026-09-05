package audit

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	ChainFormatVersion      = 1
	CheckpointFormatVersion = 1
	ManifestFormatVersion   = 1
	DefaultChainID          = "instance"
)

var (
	ErrBrokenChain           = errors.New("audit hash chain is broken")
	ErrInvalidSignature      = errors.New("audit signature is invalid")
	ErrUnsupportedCheckpoint = errors.New("unsupported audit checkpoint version")
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type canonicalEvent struct {
	FormatVersion int       `json:"format_version"`
	ID            int64     `json:"id"`
	OutboxID      int64     `json:"delivery_id"`
	Sequence      int64     `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	Actor         string    `json:"actor"`
	ActorID       int64     `json:"actor_id"`
	ActorType     string    `json:"actor_type"`
	Action        string    `json:"action"`
	TargetType    string    `json:"target_type"`
	TargetID      string    `json:"target_id"`
	Result        string    `json:"result"`
	RequestID     string    `json:"request_id"`
	SourceIP      string    `json:"source_ip"`
	UserAgent     string    `json:"user_agent"`
	AuthMethod    string    `json:"auth_method"`
	Metadata      string    `json:"metadata"`
	ChainID       string    `json:"chain_id"`
	PreviousHash  string    `json:"previous_hash"`
}

func CanonicalEvent(event Event, previousHash string) ([]byte, error) {
	return json.Marshal(canonicalEvent{
		FormatVersion: ChainFormatVersion, ID: event.ID, OutboxID: event.OutboxID, Sequence: event.Sequence,
		Timestamp: event.Timestamp.UTC(), Actor: event.Actor, ActorID: event.ActorID, ActorType: event.ActorType,
		Action: event.Action, TargetType: event.TargetType, TargetID: event.TargetID, Result: event.Result,
		RequestID: event.RequestID, SourceIP: event.SourceIP, UserAgent: event.UserAgent,
		AuthMethod: event.AuthMethod, Metadata: event.Metadata, ChainID: event.ChainID, PreviousHash: previousHash,
	})
}

func HashEvent(event Event, previousHash string) (string, error) {
	canonical, err := CanonicalEvent(event, previousHash)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

type Checkpoint struct {
	ID            int64     `json:"checkpoint_id"`
	FormatVersion int       `json:"format_version"`
	ChainID       string    `json:"chain_id"`
	FirstEventID  int64     `json:"first_event_id"`
	LastEventID   int64     `json:"last_event_id"`
	FirstSequence int64     `json:"first_sequence"`
	LastSequence  int64     `json:"last_sequence"`
	FinalHash     string    `json:"final_hash"`
	CreatedAt     time.Time `json:"created_at"`
	KeyID         string    `json:"key_id"`
	Signature     string    `json:"signature"`
}

type checkpointPayload struct {
	FormatVersion int       `json:"format_version"`
	ChainID       string    `json:"chain_id"`
	FirstEventID  int64     `json:"first_event_id"`
	LastEventID   int64     `json:"last_event_id"`
	FirstSequence int64     `json:"first_sequence"`
	LastSequence  int64     `json:"last_sequence"`
	FinalHash     string    `json:"final_hash"`
	CreatedAt     time.Time `json:"created_at"`
	KeyID         string    `json:"key_id"`
}

func canonicalCheckpoint(value Checkpoint) ([]byte, error) {
	return json.Marshal(checkpointPayload{
		FormatVersion: value.FormatVersion, ChainID: value.ChainID,
		FirstEventID: value.FirstEventID, LastEventID: value.LastEventID,
		FirstSequence: value.FirstSequence, LastSequence: value.LastSequence,
		FinalHash: value.FinalHash, CreatedAt: value.CreatedAt.UTC(), KeyID: value.KeyID,
	})
}

type SigningKeyring struct {
	active  string
	private map[string]ed25519.PrivateKey
	public  map[string]ed25519.PublicKey
}

func decodeSigningKey(value string) (ed25519.PrivateKey, error) {
	value = strings.TrimSpace(value)
	var decoded []byte
	var err error
	switch {
	case strings.HasPrefix(value, "base64:"):
		decoded, err = base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "base64:"))
	case strings.HasPrefix(value, "hex:"):
		decoded, err = hex.DecodeString(strings.TrimPrefix(value, "hex:"))
	default:
		decoded = []byte(value)
	}
	if err != nil {
		return nil, err
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("audit signing key must contain a %d-byte Ed25519 seed or %d-byte private key", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

func NewSigningKeyring(active string, values map[string]string) (*SigningKeyring, error) {
	if active == "" {
		return nil, errors.New("active audit signing key id is required")
	}
	keyring := &SigningKeyring{active: active, private: make(map[string]ed25519.PrivateKey), public: make(map[string]ed25519.PublicKey)}
	for id, value := range values {
		if !keyIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid audit signing key id %q", id)
		}
		private, err := decodeSigningKey(value)
		if err != nil {
			return nil, fmt.Errorf("audit signing key %s: %w", id, err)
		}
		keyring.private[id] = private
		keyring.public[id] = private.Public().(ed25519.PublicKey)
	}
	if _, ok := keyring.private[active]; !ok {
		return nil, fmt.Errorf("active audit signing key %q is unavailable", active)
	}
	return keyring, nil
}

func NewEphemeralSigningKeyring() (*SigningKeyring, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &SigningKeyring{active: "ephemeral-development", private: map[string]ed25519.PrivateKey{"ephemeral-development": private}, public: map[string]ed25519.PublicKey{"ephemeral-development": private.Public().(ed25519.PublicKey)}}, nil
}

func (s *SigningKeyring) ActiveKeyID() string { return s.active }

func (s *SigningKeyring) SignCheckpoint(value *Checkpoint) error {
	value.KeyID = s.active
	payload, err := canonicalCheckpoint(*value)
	if err != nil {
		return err
	}
	value.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(s.private[s.active], payload))
	return nil
}

func (s *SigningKeyring) VerifyCheckpoint(value Checkpoint) error {
	if value.FormatVersion != CheckpointFormatVersion {
		return ErrUnsupportedCheckpoint
	}
	public, ok := s.public[value.KeyID]
	if !ok {
		return ErrInvalidSignature
	}
	signature, err := base64.RawStdEncoding.DecodeString(value.Signature)
	if err != nil {
		return ErrInvalidSignature
	}
	payload, err := canonicalCheckpoint(value)
	if err != nil || !ed25519.Verify(public, payload, signature) {
		return ErrInvalidSignature
	}
	return nil
}

type ExportManifest struct {
	FormatVersion       int       `json:"format_version"`
	GeneratedAt         time.Time `json:"generated_at"`
	RecordCount         int       `json:"record_count"`
	FirstEventID        int64     `json:"first_event_id"`
	LastEventID         int64     `json:"last_event_id"`
	SHA256              string    `json:"sha256"`
	CheckpointReference int64     `json:"checkpoint_reference,omitempty"`
	ApplicationVersion  string    `json:"application_version"`
	Commit              string    `json:"commit"`
	KeyID               string    `json:"key_id"`
	Signature           string    `json:"signature"`
}

type exportManifestPayload ExportManifest

func canonicalManifest(value ExportManifest) ([]byte, error) {
	value.Signature = ""
	return json.Marshal(exportManifestPayload(value))
}

func (s *SigningKeyring) SignManifest(value *ExportManifest) error {
	value.KeyID = s.active
	payload, err := canonicalManifest(*value)
	if err != nil {
		return err
	}
	value.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(s.private[s.active], payload))
	return nil
}

func (s *SigningKeyring) VerifyManifest(value ExportManifest) error {
	if value.FormatVersion != ManifestFormatVersion {
		return ErrUnsupportedCheckpoint
	}
	public, ok := s.public[value.KeyID]
	if !ok {
		return ErrInvalidSignature
	}
	signature, err := base64.RawStdEncoding.DecodeString(value.Signature)
	if err != nil {
		return ErrInvalidSignature
	}
	payload, err := canonicalManifest(value)
	if err != nil || !ed25519.Verify(public, payload, signature) {
		return ErrInvalidSignature
	}
	return nil
}

func FileSHA256(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
