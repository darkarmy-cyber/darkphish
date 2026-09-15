package licensing

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrManagedUserLimitReached    = errors.New("community managed-user limit reached")
	ErrActiveCampaignLimitReached = errors.New("community active-campaign limit reached")
)

func NormalizeManagedUser(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ProjectManagedUsers(existing []string, proposed []string) int {
	unique := make(map[string]struct{}, len(existing)+len(proposed))
	for _, email := range existing {
		if normalized := NormalizeManagedUser(email); normalized != "" {
			unique[normalized] = struct{}{}
		}
	}
	for _, email := range proposed {
		if normalized := NormalizeManagedUser(email); normalized != "" {
			unique[normalized] = struct{}{}
		}
	}
	return len(unique)
}

func EnforceManagedUsers(state State, entitlement int, existing []string, proposed []string) error {
	projected := ProjectManagedUsers(existing, proposed)
	current := ProjectManagedUsers(existing, nil)

	// Safe degraded mode permits operations that do not increase licensed use.
	if !state.AllowsExpansion() {
		if projected > current {
			return fmt.Errorf("%w: licensing state %s does not permit adding managed users", ErrManagedUserLimitReached, state)
		}
		return nil
	}
	if entitlement < 1 {
		return fmt.Errorf("%w: invalid entitlement", ErrManagedUserLimitReached)
	}
	// Existing upgraded installations that are already over limit may reduce or
	// retain their current footprint; they may not increase it further.
	if current > entitlement {
		if projected > current {
			return fmt.Errorf("%w: %d existing, %d projected, %d licensed", ErrManagedUserLimitReached, current, projected, entitlement)
		}
		return nil
	}
	if projected > entitlement {
		return fmt.Errorf("%w: %d projected, %d licensed", ErrManagedUserLimitReached, projected, entitlement)
	}
	return nil
}

func EnforceActiveCampaigns(state State, entitlement int, active int, creatingNew bool) error {
	if !creatingNew {
		return nil
	}
	if !state.AllowsExpansion() {
		return fmt.Errorf("%w: licensing state %s does not permit creating campaigns", ErrActiveCampaignLimitReached, state)
	}
	if entitlement < 1 || active >= entitlement {
		return fmt.Errorf("%w: %d active, %d licensed", ErrActiveCampaignLimitReached, active, entitlement)
	}
	return nil
}
