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

func TestSealAlwaysEncryptsPrefixLikePlaintext(t *testing.T) {
	store, err := NewKeyring("V2", map[string][]byte{"V2": []byte("0123456789abcdef0123456789abcdef")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, plaintext := range []string{encryptedPrefix + "not-an-envelope", legacyEncryptedPrefix + "not-ciphertext"} {
		sealed, err := store.Seal(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		if sealed == plaintext {
			t.Fatalf("prefix-like plaintext was not encrypted: %q", plaintext)
		}
		opened, err := store.Open(sealed)
		if err != nil {
			t.Fatal(err)
		}
		if opened != plaintext {
			t.Fatalf("got %q, want %q", opened, plaintext)
		}
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

func TestKeyringRotationReadsOldAndWritesActiveEnvelope(t *testing.T) {
	keyOne := []byte("0123456789abcdef0123456789abcdef")
	keyTwo := []byte("abcdef0123456789abcdef0123456789")
	oldStore, err := NewKeyring("V1", map[string][]byte{"V1": keyOne}, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldCiphertext, err := oldStore.Seal("rotate me")
	if err != nil {
		t.Fatal(err)
	}
	rotatedStore, err := NewKeyring("V2", map[string][]byte{"V1": keyOne, "V2": keyTwo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rotatedStore.NeedsRewrap(oldCiphertext) {
		t.Fatal("old key was not identified for re-encryption")
	}
	plaintext, err := rotatedStore.Open(oldCiphertext)
	if err != nil || plaintext != "rotate me" {
		t.Fatalf("old ciphertext did not decrypt during rotation: %q, %v", plaintext, err)
	}
	newCiphertext, err := rotatedStore.Seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if rotatedStore.NeedsRewrap(newCiphertext) {
		t.Fatal("active-key ciphertext unexpectedly needs re-encryption")
	}
	newOnlyStore, err := NewKeyring("V2", map[string][]byte{"V2": keyTwo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err = newOnlyStore.Open(newCiphertext)
	if err != nil || plaintext != "rotate me" {
		t.Fatalf("migrated ciphertext did not survive removal of v1: %q, %v", plaintext, err)
	}
}

func TestKeyringRejectsUnknownEnvelopeKey(t *testing.T) {
	keyOne := []byte("0123456789abcdef0123456789abcdef")
	keyTwo := []byte("abcdef0123456789abcdef0123456789")
	writer, _ := NewKeyring("V2", map[string][]byte{"V2": keyTwo}, nil)
	ciphertext, _ := writer.Seal("secret")
	reader, _ := NewKeyring("V1", map[string][]byte{"V1": keyOne}, nil)
	if _, err := reader.Open(ciphertext); err != ErrUnknownKey {
		t.Fatalf("expected %v, got %v", ErrUnknownKey, err)
	}
}

func TestIsCiphertextRecognizesOnlyVersionedEnvelopes(t *testing.T) {
	if IsCiphertext("plaintext") {
		t.Fatal("plaintext was classified as ciphertext")
	}
	if !IsCiphertext(providerPrefix+"payload") || !IsCiphertext(encryptedPrefix+"payload") || !IsCiphertext(legacyEncryptedPrefix+"payload") {
		t.Fatal("versioned envelope prefix was not recognized")
	}
}
