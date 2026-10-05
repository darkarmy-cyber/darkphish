package directoryimport

import (
	"context"
	"fmt"
	"testing"
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
	if got.Matched != 1 || len(got.Recipients) != 1 || got.Recipients[0].Email != "alice@example.test" {
		t.Fatalf("unexpected recipients: %#v", got)
	}
	if got.Excluded != 1 || got.Skipped != 2 {
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

func TestCollectLeafDNsHandlesCycles(t *testing.T) {
	reader := &fakeReader{entries: map[string]directoryEntry{
		"CN=a,DC=x":    {DN: "CN=a,DC=x", ObjectClass: []string{"group"}, Members: []string{"CN=b,DC=x"}},
		"CN=b,DC=x":    {DN: "CN=b,DC=x", ObjectClass: []string{"group"}, Members: []string{"CN=a,DC=x", "CN=user,DC=x"}},
		"CN=user,DC=x": {DN: "CN=user,DC=x", ObjectClass: []string{"person"}, Values: map[string]string{"mail": "u@example.test"}},
	}}
	got, err := collectLeafDNs(context.Background(), reader, "CN=a,DC=x", []string{"mail"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["cn=user,dc=x"]; !ok || len(got) != 1 {
		t.Fatalf("cycle handling produced %#v", got)
	}
}
