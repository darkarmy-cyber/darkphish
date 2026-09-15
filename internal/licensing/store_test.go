package licensing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateStatePersistsInstallationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "licensing", "state.json")
	first, err := LoadOrCreateState(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.InstallationID == "" {
		t.Fatal("installation id was not generated")
	}
	second, err := LoadOrCreateState(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.InstallationID != first.InstallationID {
		t.Fatalf("installation id changed across reload: %q != %q", second.InstallationID, first.InstallationID)
	}
}

func TestSaveStateUsesRestrictedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := LocalState{InstallationID: "90b594f8-d605-4b68-af68-e086749fc4bd"}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != stateFileMode {
		t.Fatalf("mode=%#o want %#o", got, stateFileMode)
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := LocalState{
		InstallationID: "90b594f8-d605-4b68-af68-e086749fc4bd",
		LeaseEnvelope:  json.RawMessage(`{"key_id":"test"}`),
		RefreshToken:   "refresh-secret",
	}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.InstallationID != state.InstallationID || got.RefreshToken != state.RefreshToken || string(got.LeaseEnvelope) != string(state.LeaseEnvelope) {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestLoadStateRejectsMissingInstallationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"refresh_token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("expected invalid state to be rejected")
	}
}

func TestSaveStateRejectsEmptyInstallationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, LocalState{}); err == nil {
		t.Fatal("expected empty installation id to be rejected")
	}
}
