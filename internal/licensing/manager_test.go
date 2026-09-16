package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerStartsMissingAndKeepsInstallationID(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license-state.json")
	now := time.Unix(1789473600, 0).UTC()

	first, err := OpenManager(path, map[string]ed25519.PublicKey{"key": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	id := first.InstallationID()
	if id == "" {
		t.Fatal("missing installation id")
	}
	_, state, err := first.Snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateMissing {
		t.Fatalf("state=%q want %q", state, StateMissing)
	}

	second, err := OpenManager(path, map[string]ed25519.PublicKey{"key": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if second.InstallationID() != id {
		t.Fatal("installation id changed after reopen")
	}
}

func TestManagerInstallsAndReloadsLease(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license-state.json")
	now := time.Unix(1789473600, 0).UTC()
	manager, err := OpenManager(path, map[string]ed25519.PublicKey{"DP-COM-2026-01": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	lease := validLease(now)
	lease.InstallationID = manager.InstallationID()
	envelope := json.RawMessage(signedEnvelope(t, private, "DP-COM-2026-01", lease))

	got, state, err := manager.InstallLease(envelope, "refresh-secret", now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateActive || got.LicenseID != lease.LicenseID {
		t.Fatalf("unexpected installed lease: state=%q lease=%+v", state, got)
	}
	if manager.RefreshToken() != "refresh-secret" {
		t.Fatal("refresh token not retained")
	}

	reopened, err := OpenManager(path, map[string]ed25519.PublicKey{"DP-COM-2026-01": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	got, state, err = reopened.Snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateActive || got.LicenseID != lease.LicenseID || reopened.RefreshToken() != "refresh-secret" {
		t.Fatalf("reloaded state mismatch: state=%q lease=%+v token=%q", state, got, reopened.RefreshToken())
	}
}

func TestManagerRejectsInvalidLeaseWithoutReplacingGoodState(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license-state.json")
	now := time.Unix(1789473600, 0).UTC()
	manager, err := OpenManager(path, map[string]ed25519.PublicKey{"DP-COM-2026-01": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	lease := validLease(now)
	lease.InstallationID = manager.InstallationID()
	good := json.RawMessage(signedEnvelope(t, private, "DP-COM-2026-01", lease))
	if _, _, err := manager.InstallLease(good, "good-token", now); err != nil {
		t.Fatal(err)
	}

	bad := append(json.RawMessage(nil), good...)
	bad[len(bad)-2] ^= 1
	if _, _, err := manager.InstallLease(bad, "bad-token", now); err == nil {
		t.Fatal("tampered lease unexpectedly accepted")
	}
	got, state, err := manager.Snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateActive || got.LicenseID != lease.LicenseID || manager.RefreshToken() != "good-token" {
		t.Fatal("good state changed after rejected lease")
	}
}
