package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkarmy-cyber/darkphish/middleware/ratelimit"
)

func TestUntrustedForwardingHeadersCannotRotateRateLimitIdentity(t *testing.T) {
	limiter := ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(2))
	handler := TrustedProxyHeaders(limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), nil)

	for i, headers := range []map[string]string{
		{"X-Forwarded-For": "198.51.100.1"},
		{"X-Real-IP": "198.51.100.2"},
		{"X-Forwarded-For": "198.51.100.3", "X-Real-IP": "198.51.100.4"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/login", nil)
		request.RemoteAddr = "203.0.113.10:45123"
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := http.StatusNoContent
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("request %d status=%d want=%d", i, response.Code, want)
		}
	}
}

func TestTrustedProxySelectsFirstUntrustedHop(t *testing.T) {
	var remote string
	handler := TrustedProxyHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote = r.RemoteAddr
		w.WriteHeader(http.StatusNoContent)
	}), []string{"10.0.0.0/8"})

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.3")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if remote != "198.51.100.7:443" {
		t.Fatalf("trusted chain resolved to %q", remote)
	}

	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Real-IP", "198.51.100.8")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if remote != "198.51.100.8:443" {
		t.Fatalf("trusted X-Real-IP resolved to %q", remote)
	}
}

func TestMalformedOrUntrustedProxyHeadersAreIgnored(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		remoteAddr string
		forwarded  string
		trusted    []string
	}{
		{name: "untrusted peer", remoteAddr: "203.0.113.20:1234", forwarded: "198.51.100.9", trusted: []string{"10.0.0.0/8"}},
		{name: "malformed chain", remoteAddr: "10.0.0.2:1234", forwarded: "198.51.100.9, invalid", trusted: []string{"10.0.0.0/8"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var remote string
			handler := TrustedProxyHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				remote = r.RemoteAddr
			}), testCase.trusted)
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = testCase.remoteAddr
			request.Header.Set("X-Forwarded-For", testCase.forwarded)
			handler.ServeHTTP(httptest.NewRecorder(), request)
			if remote != testCase.remoteAddr {
				t.Fatalf("header changed address to %q", remote)
			}
		})
	}
}
