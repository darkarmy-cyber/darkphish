package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/models"
)

func TestAdministrativeAPILimitsUnauthenticatedRequestsAcrossRoutes(t *testing.T) {
	server := setupTest(t).apiServer
	for i := 0; i < 101; i++ {
		path := "/api/pages/"
		if i%2 == 0 {
			path = "/api/groups/"
		}
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "192.0.2.23:54321"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Header().Get("X-RateLimit-Limit") != "100" {
			t.Fatalf("missing API budget: %v", w.Header())
		}
		if i < 100 && w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d denied too early", i)
		}
		if i == 100 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1") {
			t.Fatalf("exhausted API: status=%d headers=%v", w.Code, w.Header())
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
	r.RemoteAddr = "192.0.2.24:54321"
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("independent API client blocked")
	}
}

func TestSensitiveOperationsKeepStricterLimitAndRetryHeaders(t *testing.T) {
	server := setupTest(t).apiServer
	called := 0
	handler := server.limitSensitive(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/updates/check", nil)
		r.RemoteAddr = "192.0.2.25:54321"
		r = ctx.Set(r, "user", models.User{Id: 10})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("X-RateLimit-Limit") != "5" {
			t.Fatal("sensitive budget changed")
		}
		if i == 5 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "12" || w.Header().Get("X-RateLimit-Remaining") != "0") {
			t.Fatalf("sensitive refusal: status=%d headers=%v", w.Code, w.Header())
		}
	}
	if called != 5 {
		t.Fatalf("handler called %d times", called)
	}
}
