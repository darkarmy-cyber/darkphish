package directorysync

import "testing"

func TestValidateGraphURLBoundary(t *testing.T) {
	valid := []string{
		"https://graph.microsoft.com/v1.0/groups/11111111-1111-1111-1111-111111111111/transitiveMembers/microsoft.graph.user?$top=999",
		"https://graph.microsoft.com/v1.0/groups/11111111-1111-1111-1111-111111111111/transitiveMembers/microsoft.graph.user?$skiptoken=opaque",
	}
	for _, value := range valid {
		if err := validateGraphURL(value); err != nil {
			t.Fatalf("valid Graph URL rejected: %s: %v", value, err)
		}
	}
	invalid := []string{
		"http://graph.microsoft.com/v1.0/groups/x",
		"https://evil.example/v1.0/groups/x",
		"https://graph.microsoft.com@evil.example/v1.0/groups/x",
		"https://graph.microsoft.com/beta/groups/x",
	}
	for _, value := range invalid {
		if err := validateGraphURL(value); err == nil {
			t.Fatalf("unsafe Graph URL accepted: %s", value)
		}
	}
}

func TestNormalizeEmailPreservesMailboxCase(t *testing.T) {
	if got := normalizeEmail("User@EXAMPLE.COM"); got != "User@example.com" {
		t.Fatalf("email=%q", got)
	}
}

func TestAllowedDomainIsExact(t *testing.T) {
	domains := map[string]struct{}{"example.com": {}}
	if !allowedDomain("u@example.com", domains) {
		t.Fatal("expected allowed domain")
	}
	if allowedDomain("u@sub.example.com", domains) {
		t.Fatal("subdomain unexpectedly allowed")
	}
}
