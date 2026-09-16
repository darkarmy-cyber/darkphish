package models

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testLicenseManager(t *testing.T, managedUsers, activeCampaigns int, editions ...string) *licensing.Manager {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "license-state.json"), map[string]ed25519.PublicKey{"test": public}, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	edition := licensing.EditionCommunity
	if len(editions) > 0 {
		edition = editions[0]
	}
	lease := licensing.Lease{
		Schema: licensing.LeaseSchema, Product: licensing.ProductDarkphish, Edition: edition,
		LicenseID: "DP-COM-TEST", InstallationID: manager.InstallationID(),
		IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(), GraceUntil: now.Add(2 * time.Hour).Unix(),
		Entitlements: licensing.Entitlements{ManagedUsers: managedUsers, ActiveCampaigns: activeCampaigns},
	}
	payload, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(licensing.Envelope{
		KeyID: "test", Algorithm: licensing.AlgorithmEd25519,
		Payload:   base64.RawURLEncoding.EncodeToString(payload),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, payload)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.InstallLease(envelope, "refresh", now); err != nil {
		t.Fatal(err)
	}
	return manager
}

func withLicensingDB(t *testing.T) *gorm.DB {
	t.Helper()
	connection, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE targets (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
		`CREATE TABLE group_targets (group_id INTEGER NOT NULL, target_id INTEGER NOT NULL)`,
		`CREATE TABLE campaigns (id INTEGER PRIMARY KEY, status TEXT NOT NULL)`,
	} {
		if err := connection.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	previousDB := db
	db = connection
	t.Cleanup(func() {
		db = previousDB
		ConfigureLicenseManager(nil)
		sqlDB, err := connection.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return connection
}

func TestEnforceGroupLicenseCountsUniqueUsersOutsideUpdatedGroup(t *testing.T) {
	connection := withLicensingDB(t)
	ConfigureLicenseManager(testLicenseManager(t, 2, 1))
	if err := connection.Exec(`INSERT INTO targets(id,email) VALUES (1,'one@example.test'),(2,'two@example.test')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.Exec(`INSERT INTO group_targets(group_id,target_id) VALUES (10,1),(20,2)`).Error; err != nil {
		t.Fatal(err)
	}

	// Updating group 10 with an address already managed by group 20 remains two
	// unique managed users after replacing group 10's old membership.
	group := &Group{Id: 10, Targets: []Target{{BaseRecipient: BaseRecipient{Email: "TWO@example.test"}}, {BaseRecipient: BaseRecipient{Email: "three@example.test"}}}}
	if err := enforceGroupLicense(connection, group); err != nil {
		t.Fatalf("replacement at managed-user limit rejected: %v", err)
	}

	// A third unique address exceeds the limit; case variants of an existing
	// address must not consume another entitlement.
	group.Targets = append(group.Targets, Target{BaseRecipient: BaseRecipient{Email: "four@example.test"}})
	if err := enforceGroupLicense(connection, group); !errors.Is(err, licensing.ErrManagedUserLimitReached) {
		t.Fatalf("err=%v want managed-user limit", err)
	}

	group.Targets = []Target{{BaseRecipient: BaseRecipient{Email: "TWO@example.test"}}}
	if err := enforceGroupLicense(connection, group); err != nil {
		t.Fatalf("replacement with existing managed user rejected: %v", err)
	}
}

func TestEnforceCampaignLicenseBlocksSecondActiveCampaign(t *testing.T) {
	connection := withLicensingDB(t)
	ConfigureLicenseManager(testLicenseManager(t, 100, 1))
	if err := connection.Exec(`INSERT INTO campaigns(id,status) VALUES (1,?)`, CampaignInProgress).Error; err != nil {
		t.Fatal(err)
	}
	if err := enforceCampaignLicense(connection); !errors.Is(err, licensing.ErrActiveCampaignLimitReached) {
		t.Fatalf("err=%v want active-campaign limit", err)
	}
	if err := connection.Exec(`UPDATE campaigns SET status=? WHERE id=1`, CampaignComplete).Error; err != nil {
		t.Fatal(err)
	}
	if err := enforceCampaignLicense(connection); err != nil {
		t.Fatalf("campaign creation after completion rejected: %v", err)
	}
}

func TestNoManagerLeavesCompatibilityToolingUnrestricted(t *testing.T) {
	connection := withLicensingDB(t)
	ConfigureLicenseManager(nil)
	if err := enforceGroupLicense(connection, &Group{Targets: []Target{{BaseRecipient: BaseRecipient{Email: "one@example.test"}}}}); err != nil {
		t.Fatalf("group enforcement should be disabled without configured manager: %v", err)
	}
	if err := enforceCampaignLicense(connection); err != nil {
		t.Fatalf("campaign enforcement should be disabled without configured manager: %v", err)
	}
}

func TestCreationRejectsCallerIDsBeforeDatabaseAccess(t *testing.T) {
	for _, id := range []int64{-1, 1, 10} {
		if err := PostGroup(&Group{Id: id}); err == nil {
			t.Fatal("group creation accepted caller id")
		}
		if err := PostCampaign(&Campaign{Id: id}, 1); err == nil {
			t.Fatal("campaign creation accepted caller id")
		}
	}
}

func TestDegradedGroupReplacementChecksIdentities(t *testing.T) {
	connection := withLicensingDB(t)
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "state.json"), nil, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ConfigureLicenseManager(manager)
	if err := connection.Exec(`INSERT INTO targets VALUES (1,'one@example.test'),(2,'two@example.test')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.Exec(`INSERT INTO group_targets VALUES (10,1),(10,2)`).Error; err != nil {
		t.Fatal(err)
	}
	group := &Group{Id: 10, Targets: []Target{{BaseRecipient: BaseRecipient{Email: "new@example.test"}}}}
	if err := enforceGroupLicense(connection, group); !errors.Is(err, licensing.ErrManagedUserLimitReached) {
		t.Fatalf("new identity accepted while degraded: %v", err)
	}
	group.Targets[0].Email = " ONE@example.test "
	if err := enforceGroupLicense(connection, group); err != nil {
		t.Fatalf("existing normalized identity rejected: %v", err)
	}
}

func TestCampaignLaunchRechecksMissingLease(t *testing.T) {
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "state.json"), nil, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ConfigureLicenseManager(manager)
	t.Cleanup(func() { ConfigureLicenseManager(nil) })
	if err := CheckCampaignLicense(); !errors.Is(err, licensing.ErrActiveCampaignLimitReached) {
		t.Fatalf("launch permitted without valid lease: %v", err)
	}
	ConfigureLicenseManager(testLicenseManager(t, 100, 1))
	if err := CheckCampaignLicense(); err != nil {
		t.Fatalf("active license blocked launch: %v", err)
	}
}

func TestEnterpriseStatusAndUnlimitedEnforcement(t *testing.T) {
	connection := withLicensingDB(t)
	ConfigureLicenseManager(testLicenseManager(t, licensing.Unlimited, licensing.Unlimited, licensing.EditionEnterprise))
	status, err := GetLicenseStatus(time.Now())
	if err != nil || status.Edition != licensing.EditionEnterprise || status.ManagedUsersLimit != licensing.Unlimited || status.ActiveCampaignLimit != licensing.Unlimited {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if err := enforceGroupLicense(connection, &Group{Targets: []Target{{BaseRecipient: BaseRecipient{Email: "one@example.test"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := enforceCampaignLicense(connection); err != nil {
		t.Fatal(err)
	}
}
