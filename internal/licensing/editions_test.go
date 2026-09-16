package licensing

import (
	"testing"
	"time"
)

func TestSignedEditions(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	for _, edition := range []string{EditionCommunity, EditionProfessional, EditionEnterprise} {
		t.Run(edition, func(t *testing.T) {
			lease := validLease(now)
			lease.Edition = edition
			lease.Entitlements = Entitlements{ManagedUsers: 500, ActiveCampaigns: 5}
			raw := signedEnvelope(t, private, "DP-COM-2026-01", lease)
			got, state, err := verifier.VerifyEnvelope(raw, lease.InstallationID, now)
			if err != nil || state != StateActive || got.Edition != edition || got.Entitlements != lease.Entitlements {
				t.Fatalf("edition=%s state=%s lease=%+v err=%v", edition, state, got, err)
			}
			if _, _, err = verifier.VerifyEnvelope(raw, "different-installation", now); err == nil {
				t.Fatal("edition bypassed installation binding")
			}
		})
	}
	lease := validLease(now)
	lease.Edition = "unknown"
	if _, _, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2026-01", lease), lease.InstallationID, now); err == nil {
		t.Fatal("unknown signed edition accepted")
	}
}

func TestUnlimitedPaidEntitlements(t *testing.T) {
	now := time.Unix(1789473600, 0).UTC()
	verifier, private := testVerifier(t)
	for _, edition := range []string{EditionProfessional, EditionEnterprise, EditionCommunity} {
		lease := validLease(now)
		lease.Edition = edition
		lease.Entitlements = Entitlements{ManagedUsers: Unlimited, ActiveCampaigns: Unlimited}
		_, _, err := verifier.VerifyEnvelope(signedEnvelope(t, private, "DP-COM-2026-01", lease), lease.InstallationID, now)
		if (err != nil) != (edition == EditionCommunity) {
			t.Fatalf("unexpected unlimited validation for %s: %v", edition, err)
		}
	}
	for _, state := range []State{StateActive, StateGrace} {
		if err := EnforceManagedUserCounts(state, Unlimited, 100000, 200000); err != nil {
			t.Fatal(err)
		}
		if err := EnforceActiveCampaigns(state, Unlimited, 100000, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []State{StateExpired, StateInvalid, StateMissing} {
		if err := EnforceManagedUserCounts(state, Unlimited, 100000, 200000); err == nil {
			t.Fatal("unlimited bypassed expiry for users")
		}
		if err := EnforceActiveCampaigns(state, Unlimited, 100000, true); err == nil {
			t.Fatal("unlimited bypassed expiry for campaigns")
		}
	}
	if err := EnforceManagedUserCounts(StateActive, -2, 1, 2); err == nil {
		t.Fatal("invalid negative entitlement accepted")
	}
	if err := EnforceActiveCampaigns(StateActive, Unlimited, -1, true); err == nil {
		t.Fatal("invalid campaign usage accepted")
	}
}
