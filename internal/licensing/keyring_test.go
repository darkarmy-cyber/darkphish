package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTrustedKeyring(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license-keyring.json")
	content := `{"schema":"darkphish-license-keyring/v1","keys":{"DP-COM-2026-01":"` + base64.StdEncoding.EncodeToString(public) + `"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := LoadTrustedKeyring(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys["DP-COM-2026-01"]; string(got) != string(public) {
		t.Fatal("loaded public key does not match")
	}
}

func TestLoadTrustedKeyringRejectsInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "license-keyring.json")
	if err := os.WriteFile(path, []byte(`{"schema":"darkphish-license-keyring/v1","keys":{"bad":"YQ=="}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTrustedKeyring(path); err == nil {
		t.Fatal("expected invalid Ed25519 key to be rejected")
	}
}

func TestLoadTrustedKeyringRejectsUnknownFields(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license-keyring.json")
	content := `{"schema":"darkphish-license-keyring/v1","keys":{"key":"` + base64.StdEncoding.EncodeToString(public) + `"},"private_key":"forbidden"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTrustedKeyring(path); err == nil {
		t.Fatal("expected unknown keyring field to be rejected")
	}
}
