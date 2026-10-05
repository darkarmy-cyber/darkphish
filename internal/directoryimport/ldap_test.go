package directoryimport

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

type fakeReader struct {
	entries map[string]directoryEntry
}

func (f *fakeReader) ReadEntry(_ context.Context, dn string, _ []string) (directoryEntry, error) {
	entry, ok := f.entries[dn]
	if !ok {
		return directoryEntry{}, fmt.Errorf("missing %s", dn)
	}
	return entry, nil
}

func (f *fakeReader) Close() error { return nil }

func TestPreviewNestedExclusionDomainAndDedup(t *testing.T) {
	cfg, err := normalizeConfig(Config{
		URL:              "ldaps://dc.example.test",
		BindDN:           "CN=svc,DC=example,DC=test",
		BindPassword:     "secret",
		GroupDN:          "CN=all,DC=example,DC=test",
		ExclusionGroupDN: "CN=exclude,DC=example,DC=test",
		EmailDomains:     []string{"example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &fakeReader{entries: map[string]directoryEntry{
		"CN=all,DC=example,DC=test": {
			DN: "CN=all,DC=example,DC=test", ObjectClass: []string{"group"},
			Members: []string{"CN=nested,DC=example,DC=test", "CN=bob,DC=example,DC=test", "CN=outside,DC=example,DC=test"},
		},
		"CN=nested,DC=example,DC=test": {
			DN: "CN=nested,DC=example,DC=test", ObjectClass: []string{"group"},
			Members: []string{"CN=alice,DC=example,DC=test", "CN=alice-copy,DC=example,DC=test"},
		},
		"CN=exclude,DC=example,DC=test": {
			DN: "CN=exclude,DC=example,DC=test", ObjectClass: []string{"group"},
			Members: []string{"CN=bob,DC=example,DC=test"},
		},
		"CN=alice,DC=example,DC=test": {
			DN: "CN=alice,DC=example,DC=test", ObjectClass: []string{"person"},
			Values: map[string]string{"givenName": "Alice", "sn": "Admin", "mail": "alice@example.test", "title": "Admin"},
		},
		"CN=alice-copy,DC=example,DC=test": {
			DN: "CN=alice-copy,DC=example,DC=test", ObjectClass: []string{"person"},
			Values: map[string]string{"givenName": "Alice", "sn": "Duplicate", "mail": "ALICE@example.test", "title": "Other"},
		},
		"CN=bob,DC=example,DC=test": {
			DN: "CN=bob,DC=example,DC=test", ObjectClass: []string{"person"},
			Values: map[string]string{"givenName": "Bob", "sn": "Excluded", "mail": "bob@example.test", "title": "User"},
		},
		"CN=outside,DC=example,DC=test": {
			DN: "CN=outside,DC=example,DC=test", ObjectClass: []string{"person"},
			Values: map[string]string{"givenName": "Eve", "sn": "Outside", "mail": "eve@other.test", "title": "User"},
		},
	}}
	got, err := previewWithReader(context.Background(), reader, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Matched != 2 || len(got.Recipients) != 2 {
		t.Fatalf("unexpected recipients: %#v", got)
	}
	if got.Recipients[0].Email != "ALICE@example.test" || got.Recipients[1].Email != "alice@example.test" {
		t.Fatalf("local-part case was not preserved: %#v", got.Recipients)
	}
	if got.Excluded != 1 || got.Skipped != 1 {
		t.Fatalf("unexpected counters: %#v", got)
	}
}

func TestNormalizeConfigRejectsPlainLDAPAndBadDN(t *testing.T) {
	for _, cfg := range []Config{
		{URL: "ldap://dc.example.test", BindDN: "CN=svc,DC=example,DC=test", BindPassword: "x", GroupDN: "CN=g,DC=example,DC=test"},
		{URL: "ldaps://dc.example.test", BindDN: "not-a-dn", BindPassword: "x", GroupDN: "CN=g,DC=example,DC=test"},
	} {
		if _, err := normalizeConfig(cfg); err == nil {
			t.Fatalf("unsafe config accepted: %#v", cfg)
		}
	}
}

func TestNormalizeConfigRejectsUnsafeAttributeName(t *testing.T) {
	_, err := normalizeConfig(Config{
		URL:          "ldaps://dc.example.test",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "secret",
		GroupDN:      "CN=all,DC=example,DC=test",
		Attributes:   AttributeMapping{Email: "mail)(objectClass=*"},
	})
	if err == nil {
		t.Fatal("unsafe LDAP attribute name accepted")
	}
}

func TestCollectLeafEntriesHandlesCycles(t *testing.T) {
	reader := &fakeReader{entries: map[string]directoryEntry{
		"CN=a,DC=x":    {DN: "CN=a,DC=x", ObjectClass: []string{"group"}, Members: []string{"CN=b,DC=x"}},
		"CN=b,DC=x":    {DN: "CN=b,DC=x", ObjectClass: []string{"group"}, Members: []string{"CN=a,DC=x", "CN=user,DC=x"}},
		"CN=user,DC=x": {DN: "CN=user,DC=x", ObjectClass: []string{"person"}, Values: map[string]string{"mail": "u@example.test"}},
	}}
	got, err := collectLeafEntries(context.Background(), reader, "CN=a,DC=x", []string{"mail"})
	if err != nil {
		t.Fatal(err)
	}
	key, keyErr := canonicalDNKey("CN=user,DC=x")
	if keyErr != nil {
		t.Fatal(keyErr)
	}
	if _, ok := got[key]; !ok || len(got) != 1 {
		t.Fatalf("cycle handling produced %#v", got)
	}
}

func TestNormalizeConfigNormalizesLDAPSAndDomains(t *testing.T) {
	cfg, err := normalizeConfig(Config{
		URL:          "LDAPS://DC.EXAMPLE.TEST:636",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "secret",
		GroupDN:      "CN=g,DC=example,DC=test",
		EmailDomains: []string{"Example.Test", "@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "ldaps://dc.example.test:636" {
		t.Fatalf("normalized URL=%q", cfg.URL)
	}
	if len(cfg.EmailDomains) != 1 || cfg.EmailDomains[0] != "example.test" {
		t.Fatalf("domains=%#v", cfg.EmailDomains)
	}
}

func TestNormalizeConfigBoundsDomains(t *testing.T) {
	domains := make([]string, maxEmailDomains+1)
	for i := range domains {
		domains[i] = fmt.Sprintf("d%d.example.test", i)
	}
	_, err := normalizeConfig(Config{
		URL:          "ldaps://dc.example.test",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "secret",
		GroupDN:      "CN=g,DC=example,DC=test",
		EmailDomains: domains,
	})
	if err == nil {
		t.Fatal("too many domains accepted")
	}
}

func TestCanonicalDNKeyPreservesValueCaseAndNormalizesTypeOrder(t *testing.T) {
	a, err := canonicalDNKey("CN=Alice+OU=People,DC=Example,DC=Test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalDNKey("ou=People+cn=Alice,dc=Example,dc=Test")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("canonical keys differ: %q != %q", a, b)
	}
	c, err := canonicalDNKey("CN=alice+OU=People,DC=Example,DC=Test")
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatalf("case-exact values collapsed: %q", a)
	}
}

func TestRangedMemberValues(t *testing.T) {
	entry := &ldap.Entry{Attributes: []*ldap.EntryAttribute{
		{Name: "member;range=0-1499", Values: []string{"CN=a,DC=x", "CN=b,DC=x"}},
	}}
	values, next, done := rangedMemberValues(entry)
	if done || next != 1500 || len(values) != 2 {
		t.Fatalf("values=%#v next=%d done=%v", values, next, done)
	}
	entry = &ldap.Entry{Attributes: []*ldap.EntryAttribute{
		{Name: "member;range=1500-*", Values: []string{"CN=c,DC=x"}},
	}}
	values, next, done = rangedMemberValues(entry)
	if !done || next != 0 || len(values) != 1 {
		t.Fatalf("final values=%#v next=%d done=%v", values, next, done)
	}
}

func TestNormalizeAuditIdentityIncludesEffectivePort(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want string
	}{
		{"ldaps://Directory.Example", "directory.example:636"},
		{"ldaps://Directory.Example:1636", "directory.example:1636"},
	} {
		got, _, err := NormalizeAuditIdentity(Config{
			URL:          tc.url,
			BindDN:       "CN=svc,DC=example,DC=test",
			BindPassword: "secret",
			GroupDN:      "CN=g,DC=example,DC=test",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("identity=%q want=%q", got, tc.want)
		}
	}
}

func TestUniqueMemberDNHandlesUIDSuffix(t *testing.T) {
	got, err := uniqueMemberDN("CN=alice,DC=example,DC=test#'0101'B")
	if err != nil {
		t.Fatal(err)
	}
	if got != "CN=alice,DC=example,DC=test" {
		t.Fatalf("dn=%q", got)
	}
	if _, err := uniqueMemberDN("not-a-dn#'0101'B"); err == nil {
		t.Fatal("invalid uniqueMember DN accepted")
	}
}

func TestValidAttributeDescription(t *testing.T) {
	for _, value := range []string{"mail", "cn;lang-de", "2.5.4.0", "2.5.4.0;binary"} {
		if !validAttributeName(value) {
			t.Fatalf("valid attribute description rejected: %q", value)
		}
	}
	for _, value := range []string{"", "1", "1..2", "01.2", "mail;", "mail)(objectClass=*"} {
		if validAttributeName(value) {
			t.Fatalf("invalid attribute description accepted: %q", value)
		}
	}
}

func TestNormalizeEmailDomainPreservesLocalPart(t *testing.T) {
	got := normalizeEmailDomain("User@EXAMPLE.COM")
	if got != "User@example.com" {
		t.Fatalf("email=%q", got)
	}
}

func TestPreviewSkipsOversizedLeafBeforeResponse(t *testing.T) {
	cfg, err := normalizeConfig(Config{
		URL:          "ldaps://dc.example.test",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "secret",
		GroupDN:      "CN=all,DC=example,DC=test",
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &fakeReader{entries: map[string]directoryEntry{
		"CN=all,DC=example,DC=test": {
			DN: "CN=all,DC=example,DC=test", ObjectClass: []string{"group"},
			Members: []string{"CN=big,DC=example,DC=test"},
		},
		"CN=big,DC=example,DC=test": {
			DN: "CN=big,DC=example,DC=test", ObjectClass: []string{"person"},
			Oversized: true,
		},
	}}
	got, err := previewWithReader(context.Background(), reader, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Matched != 0 || got.Skipped != 1 {
		t.Fatalf("unexpected preview: %#v", got)
	}
}

func TestBoundedValuesRejectsLargeMemberAndObjectClassSets(t *testing.T) {
	tooMany := make([]string, maxDirectoryMembers+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("CN=user-%d,DC=example,DC=test", i)
	}
	if _, _, err := boundedValues(tooMany, maxDirectoryMembers, maxMembershipBytes, maxDNBytes); err == nil {
		t.Fatal("oversized member set accepted")
	}
	classes := make([]string, maxObjectClasses+1)
	for i := range classes {
		classes[i] = fmt.Sprintf("class%d", i)
	}
	if _, _, err := boundedValues(classes, maxObjectClasses, maxObjectClassBytes, 256); err == nil {
		t.Fatal("oversized objectClass set accepted")
	}
}

func TestPreviewAttributeMapKeysAreCaseInsensitive(t *testing.T) {
	cfg, err := normalizeConfig(Config{
		URL:          "ldaps://dc.example.test",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "secret",
		GroupDN:      "CN=all,DC=example,DC=test",
		Attributes: AttributeMapping{
			FirstName: "MAIL",
			LastName:  "sn",
			Email:     "mail",
			Position:  "title",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &fakeReader{entries: map[string]directoryEntry{
		"CN=all,DC=example,DC=test": {
			DN: "CN=all,DC=example,DC=test", ObjectClass: []string{"group"},
			Members: []string{"CN=user,DC=example,DC=test"},
		},
		"CN=user,DC=example,DC=test": {
			DN: "CN=user,DC=example,DC=test", ObjectClass: []string{"person"},
			Values: map[string]string{
				"mail":  "User@example.test",
				"sn":    "Case",
				"title": "Admin",
			},
		},
	}}
	got, err := previewWithReader(context.Background(), reader, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Matched != 1 || got.Recipients[0].Email != "User@example.test" || got.Recipients[0].FirstName != "User@example.test" {
		t.Fatalf("unexpected preview: %#v", got)
	}
}

func TestValidBareMailboxAcceptsQuotedLocalPart(t *testing.T) {
	for _, email := range []string{`"a b"@example.com`, "User@example.com"} {
		if !validBareMailbox(email) {
			t.Fatalf("valid mailbox rejected: %q", email)
		}
	}
	for _, email := range []string{"Display <user@example.com>", "<user@example.com>", "not-an-email"} {
		if validBareMailbox(email) {
			t.Fatalf("non-bare mailbox accepted: %q", email)
		}
	}
}
