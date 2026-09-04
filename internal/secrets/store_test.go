package secrets

import (
	"strings"
	"testing"
)

func TestAESGCMRoundTrip(t *testing.T) {
	store, err := NewAESGCM([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := store.Seal("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "correct horse battery staple" || !strings.HasPrefix(sealed, encryptedPrefix) {
		t.Fatalf("secret was not encrypted: %q", sealed)
	}
	opened, err := store.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if opened != "correct horse battery staple" {
		t.Fatalf("unexpected plaintext %q", opened)
	}
}

func TestAESGCMRejectsTampering(t *testing.T) {
	store, err := NewAESGCM([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := store.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(sealed + "A"); err != ErrInvalidCiphertext {
		t.Fatalf("expected %v, got %v", ErrInvalidCiphertext, err)
	}
}

func TestNewAESGCMRejectsWeakKey(t *testing.T) {
	if _, err := NewAESGCM([]byte("short")); err != ErrInvalidKey {
		t.Fatalf("expected %v, got %v", ErrInvalidKey, err)
	}
}
