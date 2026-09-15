package licensing

import (
	"errors"
	"fmt"
	"testing"
)

func managedUsers(n int) []string {
	values := make([]string, 0, n)
	for i := 0; i < n; i++ {
		values = append(values, fmt.Sprintf("user%03d@example.test", i))
	}
	return values
}

func TestManagedUserBoundary(t *testing.T) {
	existing := managedUsers(99)
	if err := EnforceManagedUsers(StateActive, 100, existing, []string{"new@example.test"}); err != nil {
		t.Fatalf("99 -> 100 rejected: %v", err)
	}
	existing = append(existing, "new@example.test")
	if err := EnforceManagedUsers(StateActive, 100, existing, []string{"overflow@example.test"}); !errors.Is(err, ErrManagedUserLimitReached) {
		t.Fatalf("100 -> 101 err=%v want limit error", err)
	}
}

func TestManagedUserDuplicateDoesNotConsumeAnotherSlot(t *testing.T) {
	existing := managedUsers(100)
	proposed := []string{" USER000@example.test ", "user000@EXAMPLE.TEST"}
	if got := ProjectManagedUsers(existing, proposed); got != 100 {
		t.Fatalf("projected=%d want 100", got)
	}
	if err := EnforceManagedUsers(StateActive, 100, existing, proposed); err != nil {
		t.Fatalf("duplicate user was rejected: %v", err)
	}
}

func TestExpiredLicenseBlocksExpansionButAllowsReduction(t *testing.T) {
	existing := managedUsers(100)
	if err := EnforceManagedUsers(StateExpired, 100, existing, []string{"new@example.test"}); !errors.Is(err, ErrManagedUserLimitReached) {
		t.Fatalf("expired license expansion err=%v want limit error", err)
	}
	if err := EnforceManagedUsers(StateExpired, 100, existing[:99], nil); err != nil {
		t.Fatalf("expired license reduction rejected: %v", err)
	}
}

func TestExistingOverLimitCanReduceButNotExpand(t *testing.T) {
	existing := managedUsers(120)
	if err := EnforceManagedUsers(StateActive, 100, existing, []string{"new@example.test"}); !errors.Is(err, ErrManagedUserLimitReached) {
		t.Fatalf("over-limit expansion err=%v want limit error", err)
	}
	if err := EnforceManagedUsers(StateActive, 100, existing[:110], nil); err != nil {
		t.Fatalf("over-limit reduction rejected: %v", err)
	}
}

func TestActiveCampaignBoundary(t *testing.T) {
	if err := EnforceActiveCampaigns(StateActive, 1, 0, true); err != nil {
		t.Fatalf("first campaign rejected: %v", err)
	}
	if err := EnforceActiveCampaigns(StateActive, 1, 1, true); !errors.Is(err, ErrActiveCampaignLimitReached) {
		t.Fatalf("second active campaign err=%v want limit error", err)
	}
}

func TestExpiredLicenseAllowsNonCreatingCampaignOperation(t *testing.T) {
	if err := EnforceActiveCampaigns(StateExpired, 1, 1, false); err != nil {
		t.Fatalf("non-creating operation rejected: %v", err)
	}
	if err := EnforceActiveCampaigns(StateExpired, 1, 0, true); !errors.Is(err, ErrActiveCampaignLimitReached) {
		t.Fatalf("expired campaign creation err=%v want limit error", err)
	}
}
