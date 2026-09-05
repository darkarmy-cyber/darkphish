package audit

import (
	"errors"
	"testing"
	"time"
)

func signingSeeds() map[string]string {
	return map[string]string{
		"v1": "0123456789abcdef0123456789abcdef",
		"v2": "abcdef0123456789abcdef0123456789",
	}
}

func TestCanonicalEventAndHashAreDeterministic(t *testing.T) {
	event := Event{
		ID: 7, Sequence: 3, Timestamp: time.Date(2026, 9, 5, 10, 11, 12, 13, time.FixedZone("test", 7200)),
		Actor: "reviewer", ActorID: 9, ActorType: "user", Action: "credential.view",
		TargetType: "campaign", TargetID: "42/results/test", Result: "success",
		RequestID: "request", SourceIP: "192.0.2.1", UserAgent: "test", AuthMethod: "session",
		Metadata: "{}", ChainID: DefaultChainID,
	}
	first, err := HashEvent(event, "previous")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashEvent(event, "previous")
	if err != nil || first != second {
		t.Fatalf("non-deterministic audit hash: %q %q %v", first, second, err)
	}
	event.Action = "credential.denied"
	changed, err := HashEvent(event, "previous")
	if err != nil || changed == first {
		t.Fatal("changed canonical data did not change the audit hash")
	}
}

func TestCheckpointRejectsTamperingAndUnsupportedVersion(t *testing.T) {
	keyring, err := NewSigningKeyring("v1", signingSeeds())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := Checkpoint{
		FormatVersion: CheckpointFormatVersion, ChainID: DefaultChainID,
		FirstEventID: 1, LastEventID: 5, FirstSequence: 1, LastSequence: 5,
		FinalHash: "0123456789abcdef", CreatedAt: time.Now().UTC(),
	}
	if err := keyring.SignCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}
	tampered := checkpoint
	tampered.FinalHash = "changed"
	if !errors.Is(keyring.VerifyCheckpoint(tampered), ErrInvalidSignature) {
		t.Fatal("tampered checkpoint was accepted")
	}
	unsupported := checkpoint
	unsupported.FormatVersion++
	if !errors.Is(keyring.VerifyCheckpoint(unsupported), ErrUnsupportedCheckpoint) {
		t.Fatal("unsupported checkpoint version was accepted")
	}
}

func TestSigningKeyRotationPreservesOldVerification(t *testing.T) {
	oldKeyring, err := NewSigningKeyring("v1", signingSeeds())
	if err != nil {
		t.Fatal(err)
	}
	oldCheckpoint := Checkpoint{FormatVersion: CheckpointFormatVersion, ChainID: DefaultChainID, LastEventID: 1, LastSequence: 1, FinalHash: "old", CreatedAt: time.Now().UTC()}
	if err := oldKeyring.SignCheckpoint(&oldCheckpoint); err != nil {
		t.Fatal(err)
	}
	rotatedKeyring, err := NewSigningKeyring("v2", signingSeeds())
	if err != nil {
		t.Fatal(err)
	}
	if err := rotatedKeyring.VerifyCheckpoint(oldCheckpoint); err != nil {
		t.Fatalf("old checkpoint did not verify after rotation: %v", err)
	}
	newCheckpoint := oldCheckpoint
	newCheckpoint.Signature = ""
	if err := rotatedKeyring.SignCheckpoint(&newCheckpoint); err != nil {
		t.Fatal(err)
	}
	if newCheckpoint.KeyID != "v2" || newCheckpoint.Signature == oldCheckpoint.Signature {
		t.Fatal("new checkpoint did not use the rotated active key")
	}
}

func TestManifestSignatureDetectsMetadataChange(t *testing.T) {
	keyring, err := NewSigningKeyring("v1", signingSeeds())
	if err != nil {
		t.Fatal(err)
	}
	manifest := ExportManifest{
		FormatVersion: ManifestFormatVersion, GeneratedAt: time.Now().UTC(), RecordCount: 4,
		FirstEventID: 1, LastEventID: 4, SHA256: "digest", CheckpointReference: 2,
		ApplicationVersion: "0.3.0", Commit: "test",
	}
	if err := keyring.SignManifest(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := keyring.VerifyManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.RecordCount++
	if !errors.Is(keyring.VerifyManifest(manifest), ErrInvalidSignature) {
		t.Fatal("tampered manifest metadata was accepted")
	}
}
