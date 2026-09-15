package licensing

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Manager owns the locally persisted activation state and exposes a small,
// concurrency-safe entitlement surface to the rest of Darkphish. Network
// activation/refresh is deliberately kept outside this type.
type Manager struct {
	mu           sync.RWMutex
	statePath    string
	local        LocalState
	verifier     *Verifier
	lease        Lease
	state        State
	lastVerifyErr error
}

func OpenManager(statePath string, trustedKeys map[string]ed25519.PublicKey, clockSkew time.Duration, now time.Time) (*Manager, error) {
	local, err := LoadOrCreateState(statePath)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		statePath: statePath,
		local:     local,
		verifier:  NewVerifier(trustedKeys, clockSkew),
		state:     StateMissing,
	}
	m.verifyLocked(now)
	return m, nil
}

func (m *Manager) InstallationID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.local.InstallationID
}

func (m *Manager) Snapshot(now time.Time) (Lease, State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyLocked(now)
	return m.lease, m.state, m.lastVerifyErr
}

func (m *Manager) InstallLease(envelope json.RawMessage, refreshToken string, now time.Time) (Lease, State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, state, err := m.verifier.VerifyEnvelope(envelope, m.local.InstallationID, now)
	if err != nil || !state.AllowsExpansion() {
		if err == nil {
			err = errors.New("activation lease is not currently usable")
		}
		return Lease{}, state, err
	}

	candidate := m.local
	candidate.LeaseEnvelope = append(json.RawMessage(nil), envelope...)
	candidate.RefreshToken = refreshToken
	if err := SaveState(m.statePath, candidate); err != nil {
		return Lease{}, StateInvalid, err
	}
	m.local = candidate
	m.lease = lease
	m.state = state
	m.lastVerifyErr = nil
	return lease, state, nil
}

func (m *Manager) ReplaceTrustedKeys(keys map[string]ed25519.PublicKey, clockSkew time.Duration, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifier = NewVerifier(keys, clockSkew)
	m.verifyLocked(now)
}

func (m *Manager) RefreshToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.local.RefreshToken
}

func (m *Manager) verifyLocked(now time.Time) {
	lease, state, err := m.verifier.VerifyEnvelope(m.local.LeaseEnvelope, m.local.InstallationID, now)
	m.lease = lease
	m.state = state
	m.lastVerifyErr = err
}
