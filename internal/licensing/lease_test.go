package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func signedEnvelope(t *testing.T, private ed25519.PrivateKey, keyID string, lease Lease) []byte {
	t.Helper()
	payload, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{
		KeyID:     keyID,
		Algorithm: AlgorithmEd25519,
		Payload:   base64.RawURLEncoding.EncodeToString(payload),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload)),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func validLease(now time.Time) Lease {
	issued := now.Add(-time.Hour).Unix()
	return Lease{
		Schema:         LeaseSchema,
		Product:        ProductDarkphish,
		Edition:        EditionCommunity,
		LicenseID:      "DP-COM-000001",
		InstallationID: "90b594f8-d605-4b68-af68-e086749fc4bd",
		IssuedAt:       issued,
		NotBefore:      issued,
		ExpiresAt:      now.Add(24 * time.Hour).Unix(),
		GraceUntil:     now.Add(31 * 24 * time.Hour).Unix(),
		Entitlements: Entitlements{
			ManagedUsers:    100,
			ActiveCampaigns: 1,
		},
	}
}

func testVerifier(t *testing.T) (*Verifier, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return NewVerifier(map[string]ed25519.PublicKey{"DP-COM-2026-01": public}, 5*time.Minute), private
}

func TestVerifyEnvelopeActive(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	lease := validLease(now)

	got, state, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2026-01", lease), lease.InstallationID, now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateActive {
		t.Fatalf("state=%q want %q", state, StateActive)
	}
	if got.Entitlements.ManagedUsers != 100 || got.Entitlements.ActiveCampaigns != 1 {
		t.Fatalf("unexpected entitlements: %+v", got.Entitlements)
	}
}

func TestVerifyEnvelopeMissing(t *testing.T) {
	verifier, _ := testVerifier(t)
	_, state, err := verifier.VerifyEnvelope(nil, "installation", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if state != StateMissing {
		t.Fatalf("state=%q want %q", state, StateMissing)
	}
}

func TestVerifyEnvelopeRejectsTampering(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	raw := signedEnvelope(t, private, "DP-COM-2026-01", validLease(now))

	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var lease Lease
	if err := json.Unmarshal(payload, &lease); err != nil {
		t.Fatal(err)
	}
	lease.Entitlements.ManagedUsers = 1000000
	tampered, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = base64.RawURLEncoding.EncodeToString(tampered)
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	_, state, err := verifier.VerifyEnvelope(raw, lease.InstallationID, now)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err=%v want invalid signature", err)
	}
	if state != StateInvalid {
		t.Fatalf("state=%q want %q", state, StateInvalid)
	}
}

func TestVerifyEnvelopeRejectsInstallationMismatch(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	lease := validLease(now)

	_, state, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2026-01", lease), "other-installation", now)
	if !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("err=%v want invalid lease", err)
	}
	if state != StateInvalid {
		t.Fatalf("state=%q want %q", state, StateInvalid)
	}
}

func TestVerifyEnvelopeRejectsUnknownKey(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	lease := validLease(now)

	_, state, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2027-01", lease), lease.InstallationID, now)
	if !errors.Is(err, ErrUnknownSigningKey) {
		t.Fatalf("err=%v want unknown signing key", err)
	}
	if state != StateInvalid {
		t.Fatalf("state=%q want %q", state, StateInvalid)
	}
}

func TestLeaseStates(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)

	tests := []struct {
		name  string
		lease Lease
		at    time.Time
		want  State
	}{
		{name: "active", lease: validLease(now), at: now, want: StateActive},
		{name: "grace", lease: validLease(now), at: now.Add(2 * 24 * time.Hour), want: StateGrace},
		{name: "expired", lease: validLease(now), at: now.Add(32 * 24 * time.Hour), want: StateExpired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := signedEnvelope(t, private, "DP-COM-2026-01", tc.lease)
			_, state, err := verifier.VerifyEnvelope(raw, tc.lease.InstallationID, tc.at)
			if err != nil {
				t.Fatal(err)
			}
			if state != tc.want {
				t.Fatalf("state=%q want %q", state, tc.want)
			}
		})
	}
}

func TestFutureLeaseOutsideClockSkewIsInvalid(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	lease := validLease(now)
	lease.IssuedAt = now.Add(10 * time.Minute).Unix()
	lease.NotBefore = lease.IssuedAt

	_, state, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2026-01", lease), lease.InstallationID, now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateInvalid {
		t.Fatalf("state=%q want %q", state, StateInvalid)
	}
}

func TestAllowsExpansion(t *testing.T) {
	if !StateActive.AllowsExpansion() || !StateGrace.AllowsExpansion() {
		t.Fatal("active and grace must allow licensed expansion")
	}
	if StateExpired.AllowsExpansion() || StateInvalid.AllowsExpansion() || StateMissing.AllowsExpansion() {
		t.Fatal("expired, invalid and missing must block licensed expansion")
	}
}
