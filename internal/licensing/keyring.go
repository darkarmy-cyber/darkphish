package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type KeyringFile struct {
	Schema string            `json:"schema"`
	Keys   map[string]string `json:"keys"`
}

const KeyringSchema = "darkphish-license-keyring/v1"

func LoadTrustedKeyring(path string) (map[string]ed25519.PublicKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("license keyring path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read license keyring: %w", err)
	}
	var file KeyringFile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode license keyring: %w", err)
	}
	if file.Schema != KeyringSchema || len(file.Keys) == 0 {
		return nil, errors.New("invalid license keyring")
	}
	keys := make(map[string]ed25519.PublicKey, len(file.Keys))
	for id, encoded := range file.Keys {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("license keyring contains an empty key id")
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid Ed25519 public key %q", id)
		}
		keys[id] = append(ed25519.PublicKey(nil), decoded...)
	}
	return keys, nil
}
