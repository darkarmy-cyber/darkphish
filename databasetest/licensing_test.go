package databasetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"github.com/darkarmy-cyber/darkphish/models"
	"path/filepath"
	"testing"
	"time"
)

// Exercise real competing transactions on SQLite, MySQL and PostgreSQL through
// the ordinary model entry points, with exactly one remaining entitlement.
func exerciseLicenseCoordination(t *testing.T, userID int64, prototype models.Campaign) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "license.json"), map[string]ed25519.PublicKey{"test": public}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	models.ConfigureLicenseManager(manager)
	defer models.ConfigureLicenseManager(nil)
	usage, err := models.GetLicenseStatus(now)
	if err != nil {
		t.Fatal(err)
	}
	lease := licensing.Lease{Schema: licensing.LeaseSchema, Product: licensing.ProductDarkphish, Edition: licensing.EditionCommunity,
		LicenseID: "integration-test", InstallationID: manager.InstallationID(), IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(), GraceUntil: now.Add(time.Hour).Unix(),
		Entitlements: licensing.Entitlements{ManagedUsers: usage.ManagedUsers + 1, ActiveCampaigns: usage.ActiveCampaigns + 1}}
	payload, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(licensing.Envelope{KeyID: "test", Algorithm: licensing.AlgorithmEd25519, Payload: base64.RawURLEncoding.EncodeToString(payload), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload))})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = manager.InstallLease(envelope, "test-token", now); err != nil {
		t.Fatal(err)
	}
	compete := func(name string, operation func(int) error, limit error) {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func(i int) { <-start; results <- operation(i) }(i)
		}
		close(start)
		success, rejected := 0, 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				success++
			} else if errors.Is(err, limit) {
				rejected++
			} else {
				t.Errorf("%s unexpected error: %v", name, err)
			}
		}
		if success != 1 || rejected != 1 {
			t.Fatalf("%s admitted %d and rejected %d; expected exactly one of each", name, success, rejected)
		}
	}
	compete("groups", func(i int) error {
		group := models.Group{Name: fmt.Sprintf("licensed group %d", i), UserId: userID, Targets: []models.Target{{BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("licensed-%d@example.test", i)}}}}
		return models.PostGroup(&group)
	}, licensing.ErrManagedUserLimitReached)
	compete("campaigns", func(i int) error {
		campaign := models.Campaign{Name: fmt.Sprintf("licensed campaign %d", i), Template: prototype.Template, Page: prototype.Page, SMTP: prototype.SMTP, Groups: prototype.Groups, URL: prototype.URL}
		return models.PostCampaign(&campaign, userID)
	}, licensing.ErrActiveCampaignLimitReached)
}
