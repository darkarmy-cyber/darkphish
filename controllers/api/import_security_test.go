package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

func importSyntheticSite(t *testing.T, target string, allowed []string) *httptest.ResponseRecorder {
	t.Helper()
	original := dialer.DefaultDialer.AllowedHosts()
	if err := dialer.SetAllowedHosts(allowed); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := dialer.SetAllowedHosts(original); err != nil {
			t.Error(err)
		}
	}()
	body, err := json.Marshal(cloneRequest{URL: target})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/import/site", bytes.NewReader(body))
	response := httptest.NewRecorder()
	(&Server{}).ImportSite(response, request)
	return response
}

// These tests exercise the actual HTTP transport and connect-time policy, not
// just URL string matching. No non-loopback server is contacted.
func TestImportEgressBoundary(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.2:1/private", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head></head><body>synthetic content</body></html>"))
	}))
	defer server.Close()
	for _, target := range []string{
		server.URL,
		strings.Replace(server.URL, "127.0.0.1", "localhost", 1),
		strings.Replace(server.URL, "127.0.0.1", "[::ffff:127.0.0.1]", 1),
	} {
		response := importSyntheticSite(t, target, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unapproved loopback import returned %d", response.Code)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("a denied destination received a request")
	}
	response := importSyntheticSite(t, server.URL, []string{"127.0.0.1/32"})
	if response.Code != http.StatusOK || requests.Load() != 1 || !strings.Contains(response.Body.String(), "synthetic content") {
		t.Fatal("explicitly approved synthetic import failed")
	}
	response = importSyntheticSite(t, server.URL+"/redirect", []string{"127.0.0.1/32"})
	if response.Code != http.StatusBadRequest || requests.Load() != 2 {
		t.Fatal("redirect escaped the destination policy")
	}
}

func TestImportRequiresAuthenticatedAdministrativeAPI(t *testing.T) {
	ctx := setupTest(t)
	request := httptest.NewRequest(http.MethodPost, "/api/import/site", strings.NewReader(`{"url":"http://127.0.0.1:1"}`))
	response := httptest.NewRecorder()
	ctx.apiServer.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated import returned %d", response.Code)
	}
}

func TestImportRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head></head><body>untrusted certificate</body></html>"))
	}))
	defer server.Close()
	response := importSyntheticSite(t, server.URL+"/?private=synthetic-marker", []string{"127.0.0.1/32"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("untrusted TLS certificate was accepted: %d", response.Code)
	}
	if strings.Contains(response.Body.String(), "synthetic-marker") {
		t.Fatal("upstream failure disclosed the URL query")
	}
}

func TestImportURLValidation(t *testing.T) {
	for _, target := range []string{"", "file:///private", "gopher://example.test", "//example.test/path", "https://", "https://user:synthetic@example.test", "https://example.test\\private", "https://example.test:0", "https://example.test:65536"} {
		if err := (&cloneRequest{URL: target}).validate(); err == nil {
			t.Errorf("accepted an invalid import destination: %q", target)
		}
	}
	for _, target := range []string{"https://example.test/path?q=one", "http://example.test:8080/page", "https://[2001:4860:4860::8888]/"} {
		if err := (&cloneRequest{URL: target}).validate(); err != nil {
			t.Errorf("rejected a valid URL: %v", err)
		}
	}
}
