// Package licensetest supplies ephemeral, local-only fixtures for tests. No
// production trust key, license service or real activation is used.
package licensetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/licensing"
)

func Manager(t testing.TB, state licensing.State) *licensing.Manager {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "license.json")
	local, err := licensing.LoadOrCreateState(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	lease := licensing.Lease{Schema: licensing.LeaseSchema, Product: licensing.ProductDarkphish,
		Edition: licensing.EditionCommunity, LicenseID: "LOCAL-TEST-ONLY", InstallationID: local.InstallationID,
		IssuedAt: now.Add(-3 * time.Hour).Unix(), NotBefore: now.Add(-3 * time.Hour).Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(), GraceUntil: now.Add(2 * time.Hour).Unix(),
		Entitlements: licensing.Entitlements{ManagedUsers: 100, ActiveCampaigns: 100}}
	if state == licensing.StateGrace {
		lease.ExpiresAt = now.Add(-time.Hour).Unix()
	}
	if state == licensing.StateExpired {
		lease.ExpiresAt = now.Add(-2 * time.Hour).Unix()
		lease.GraceUntil = now.Add(-time.Hour).Unix()
	}
	if state != licensing.StateMissing {
		payload, err := json.Marshal(lease)
		if err != nil {
			t.Fatal(err)
		}
		signature := ed25519.Sign(private, payload)
		if state == licensing.StateInvalid {
			signature[0] ^= 1
		}
		local.LeaseEnvelope, err = json.Marshal(licensing.Envelope{KeyID: "local-test", Algorithm: licensing.AlgorithmEd25519,
			Payload: base64.RawURLEncoding.EncodeToString(payload), Signature: base64.RawURLEncoding.EncodeToString(signature)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := licensing.SaveState(path, local); err != nil {
		t.Fatal(err)
	}
	manager, err := licensing.OpenManager(path, map[string]ed25519.PublicKey{"local-test": public}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	_, actual, _ := manager.Snapshot(now)
	if actual != state {
		t.Fatalf("fixture state=%s want=%s", actual, state)
	}
	return manager
}
