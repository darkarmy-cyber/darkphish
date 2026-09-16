package licensing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const stateFileMode os.FileMode = 0o600

type LocalState struct {
	InstallationID string          `json:"installation_id"`
	LeaseEnvelope  json.RawMessage `json:"lease_envelope,omitempty"`
	RefreshToken   string          `json:"refresh_token,omitempty"`
}

func LoadOrCreateState(path string) (LocalState, error) {
	if strings.TrimSpace(path) == "" {
		return LocalState{}, errors.New("license state path is required")
	}
	state, err := LoadState(path)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return LocalState{}, err
	}
	id, err := newInstallationID()
	if err != nil {
		return LocalState{}, err
	}
	state = LocalState{InstallationID: id}
	if err := SaveState(path, state); err != nil {
		return LocalState{}, err
	}
	return state, nil
}

func LoadState(path string) (LocalState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LocalState{}, err
	}
	var state LocalState
	if err := json.Unmarshal(raw, &state); err != nil {
		return LocalState{}, fmt.Errorf("decode license state: %w", err)
	}
	if strings.TrimSpace(state.InstallationID) == "" {
		return LocalState{}, errors.New("license state is missing installation id")
	}
	return state, nil
}

func SaveState(path string, state LocalState) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("license state path is required")
	}
	if strings.TrimSpace(state.InstallationID) == "" {
		return errors.New("installation id is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create license state directory: %w", err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode license state: %w", err)
	}
	tmp, err := createPrivateStateTemp(dir)
	if err != nil {
		return fmt.Errorf("create temporary license state: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(stateFileMode); err != nil {
		return fmt.Errorf("restrict license state permissions: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		return fmt.Errorf("write license state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync license state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close license state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace license state: %w", err)
	}
	committed = true
	return nil
}

func newInstallationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate installation id: %w", err)
	}
	// UUIDv4 layout without introducing another dependency.
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(value[:])
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32], nil
}
