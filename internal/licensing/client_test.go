package licensing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientRejectsPlainHTTP(t *testing.T) {
	if _, err := NewClient("http://license.example.test/wp-json/darkphish-license/v1", "0.11.0", nil); err == nil {
		t.Fatal("expected HTTP licensing URL to be rejected")
	}
}

func TestActivateAndRefresh(t *testing.T) {
	var activate activationRequest
	var refresh refreshRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/wp-json/darkphish-license/v1/activate":
			if err := json.NewDecoder(r.Body).Decode(&activate); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"lease":{"key_id":"test","algorithm":"Ed25519","payload":"e30","signature":"sig"},"refresh_token":"refresh-one"}`))
		case "/wp-json/darkphish-license/v1/refresh":
			if err := json.NewDecoder(r.Body).Decode(&refresh); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"lease":{"key_id":"test","algorithm":"Ed25519","payload":"e30","signature":"sig"},"refresh_token":"refresh-two"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newClient(server.URL+"/wp-json/darkphish-license/v1", "0.11.0", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := client.Activate(context.Background(), "DP-COM-SECRET", "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	if activation.RefreshToken != "refresh-one" || activate.LicenseKey != "DP-COM-SECRET" || activate.InstallationID != "installation-1" || activate.ProductVersion != "0.11.0" {
		t.Fatalf("unexpected activation exchange: response=%+v request=%+v", activation, activate)
	}
	refreshed, err := client.Refresh(context.Background(), activation.RefreshToken, "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken != "refresh-two" || refresh.RefreshToken != "refresh-one" || refresh.InstallationID != "installation-1" || refresh.ProductVersion != "0.11.0" {
		t.Fatalf("unexpected refresh exchange: response=%+v request=%+v", refreshed, refresh)
	}
}

func TestClientDoesNotReturnServiceErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "license DP-COM-SECRET belongs to john@example.test", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := newClient(server.URL, "0.11.0", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Activate(context.Background(), "DP-COM-SECRET", "installation")
	if !errors.Is(err, ErrLicenseService) {
		t.Fatalf("err=%v want license service error", err)
	}
	if strings.Contains(err.Error(), "DP-COM-SECRET") || strings.Contains(err.Error(), "john@example.test") {
		t.Fatalf("service response leaked through error: %v", err)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxLicenseResponseBytes)+1)))
	}))
	defer server.Close()
	client, err := newClient(server.URL, "0.11.0", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Activate(context.Background(), "DP-COM-SECRET", "installation"); !errors.Is(err, ErrLicenseService) {
		t.Fatalf("err=%v want license service error", err)
	}
}

func TestClientRejectsUnknownResponseFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lease":{},"refresh_token":"refresh","unexpected":true}`))
	}))
	defer server.Close()
	client, err := newClient(server.URL, "0.11.0", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Activate(context.Background(), "DP-COM-SECRET", "installation"); !errors.Is(err, ErrLicenseService) {
		t.Fatalf("err=%v want license service error", err)
	}
}

func TestClientRejectsRedirectsWithoutForwardingCredentials(t *testing.T) {
	for _, code := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var forwarded atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
			defer target.Close()
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, code) }))
			defer source.Close()
			supplied := source.Client()
			client, err := NewClient(source.URL, "0.11.0", supplied)
			if err != nil {
				t.Fatal(err)
			}
			if supplied.CheckRedirect != nil {
				t.Fatal("mutated supplied client")
			}
			_, err = client.Activate(context.Background(), "private-key", "installation")
			if !errors.Is(err, ErrLicenseService) || forwarded.Load() != 0 {
				t.Fatalf("redirect accepted: %v, forwarded=%d", err, forwarded.Load())
			}
		})
	}
}

func TestClientRequiresUsableActivationResponse(t *testing.T) {
	for _, body := range []string{`{"lease":{}}`, `{"lease":{},"refresh_token":" "}`, `{"lease":null,"refresh_token":"token"}`, `{"lease":{},"refresh_token":"token"} {}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client, err := newClient(server.URL, "0.11.0", server.Client(), true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Activate(context.Background(), "key", "installation"); !errors.Is(err, ErrLicenseService) {
				t.Fatalf("accepted invalid activation response: %v", err)
			}
		})
	}
}

func TestRefreshMayRetainExistingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"lease":{}}`)) }))
	defer server.Close()
	client, err := newClient(server.URL, "0.11.0", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Refresh(context.Background(), "existing-token", "installation"); err != nil {
		t.Fatal(err)
	}
}
