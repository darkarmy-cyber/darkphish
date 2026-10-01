package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/middleware/ratelimit"
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

func TestPreflightRequestsConsumeBudgetAndRefusalsRemainReadable(t *testing.T) {
	for _, origin := range []string{"https://admin.example.test", "https://untrusted.example.test"} {
		t.Run(origin, func(t *testing.T) {
			server := setupTest(t).apiServer
			server.allowedOrigins = []string{"https://admin.example.test"}
			server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
			server.registerRoutes()
			for i := 0; i < 4; i++ {
				r := httptest.NewRequest(http.MethodOptions, "/api/pages/", nil)
				r.RemoteAddr = "192.0.2.26:54321"
				r.Header.Set("Origin", origin)
				r.Header.Set("Access-Control-Request-Method", http.MethodPost)
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				if i == 3 {
					var refusal struct {
						Success bool   `json:"success"`
						Message string `json:"message"`
					}
					if w.Code != http.StatusTooManyRequests || w.Header().Get("Content-Type") != "application/json" || json.Unmarshal(w.Body.Bytes(), &refusal) != nil || refusal.Success || refusal.Message == "" {
						t.Fatalf("refusal is not an actionable JSON error: %d %s", w.Code, w.Body.String())
					}
					if w.Header().Get("Retry-After") != "20" {
						t.Fatal("missing preflight retry information")
					}
				}
				if origin == "https://admin.example.test" && w.Header().Get("Access-Control-Allow-Origin") != origin {
					t.Fatal("allowed client cannot read refusal")
				}
				if origin != "https://admin.example.test" && w.Header().Get("Access-Control-Allow-Origin") != "" {
					t.Fatal("untrusted origin authorized")
				}
			}
		})
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
