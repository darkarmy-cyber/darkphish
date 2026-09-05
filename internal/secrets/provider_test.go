package secrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func vaultTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/encrypt/"):
			_, _ = w.Write([]byte(`{"data":{"ciphertext":"vault:v1:` + body["plaintext"] + `"}}`))
		case strings.Contains(r.URL.Path, "/decrypt/"):
			value := strings.TrimPrefix(body["ciphertext"], "vault:v1:")
			_, _ = w.Write([]byte(`{"data":{"plaintext":"` + value + `"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestVaultEnvelopeRoundTripAndLocalCompatibility(t *testing.T) {
	server := vaultTestServer(t)
	defer server.Close()
	provider, err := NewVaultTransitProvider(VaultTransitOptions{
		Address: server.URL, Token: "test-token", Mount: "transit", KeyName: "darkphish", Client: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	local, err := NewKeyring("V2", map[string][]byte{"V2": []byte("0123456789abcdef0123456789abcdef")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := local.Seal("old local value")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewRoutingStore("vault", local, provider)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := store.Seal("external envelope value")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, providerPrefix) || !store.NeedsRewrap(old) {
		t.Fatalf("unexpected envelope or migration state: %q", sealed)
	}
	opened, err := store.Open(sealed)
	if err != nil || opened != "external envelope value" {
		t.Fatalf("provider envelope did not round trip: %q %v", opened, err)
	}
	opened, err = store.Open(old)
	if err != nil || opened != "old local value" {
		t.Fatalf("0.2 local envelope compatibility failed: %q %v", opened, err)
	}
}

func TestVaultEnvelopeFailuresAreClosed(t *testing.T) {
	server := vaultTestServer(t)
	provider, _ := NewVaultTransitProvider(VaultTransitOptions{Address: server.URL, Token: "test-token", KeyName: "key1", Client: server.Client()})
	store, _ := NewRoutingStore("vault", nil, provider)
	sealed, err := store.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := decodeProviderEnvelope(sealed)
	if err != nil {
		t.Fatal(err)
	}
	envelope.KeyReference = "wrong-key"
	encoded, _ := json.Marshal(envelope)
	wrongReference := providerPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if _, err := store.Open(wrongReference); err == nil {
		t.Fatal("wrong key reference did not fail closed")
	}
	envelope.KeyReference = "key1"
	envelope.WrappedDataKey += "corrupt"
	encoded, _ = json.Marshal(envelope)
	corruptWrappedKey := providerPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if _, err := store.Open(corruptWrappedKey); err == nil {
		t.Fatal("corrupt wrapped DEK did not fail closed")
	}
	envelope, err = decodeProviderEnvelope(sealed)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Ciphertext += "A"
	encoded, _ = json.Marshal(envelope)
	corrupt := providerPrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if _, err := store.Open(corrupt); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("corrupt ciphertext returned %v", err)
	}
	server.Close()
	if _, err := store.Open(sealed); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("provider outage returned %v", err)
	}
}
