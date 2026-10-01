package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/middleware/ratelimit"
	"github.com/darkarmy-cyber/darkphish/models"
)

func TestTokenCreationBudgetIsSharedAcrossMintedTokens(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	token := testCtx.apiKey
	body, err := json.Marshal(createPATRequest{Name: "budget", Scopes: []string{"tokens:manage"}, ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/pats/", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if i < 5 {
			if w.Code != http.StatusCreated {
				t.Fatalf("token creation status=%d", w.Code)
			}
			var created createPATResponse
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			token = created.Token
		} else if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "12" {
			t.Fatalf("minted tokens bypassed creation budget: %d", w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/pats/", nil)
	r = ctx.Set(r, "user", testCtx.admin)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal("token management read capacity was exhausted by creation")
	}
}

func TestEquivalentBearerHeadersShareAdmissionAcrossIPs(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	for i := 0; i < 12; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.RemoteAddr = fmt.Sprintf("192.0.2.%d:54321", i+1)
		format := []string{"Bearer %s", "bearer  %s", "  BEARER\t%s  "}[i%3]
		token := testCtx.apiKey
		if i%2 == 1 {
			prefixEnd := len(token) - 44
			token = token[:14] + strings.ToUpper(token[14:prefixEnd]) + token[prefixEnd:]
		}
		r.Header.Set("Authorization", fmt.Sprintf(format, token))
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if i >= 3 && w.Code != http.StatusTooManyRequests {
			t.Fatal("equivalent header bypassed admission")
		}
	}
	_, total, err := audit.Query(audit.Filter{Action: "pat.auth.success", Page: 1, PerPage: 100})
	if err != nil || total != 3 {
		t.Fatalf("equivalent headers reached live authentication: %d %v", total, err)
	}
}

func TestRotatingInvalidTokensCannotSpendRecognizedPATCapacity(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	server.authenticationLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	if !models.RecognizedPATForAdmission(testCtx.apiKey) {
		t.Fatal("valid token not recognized")
	}
	for i := 0; i < 12; i++ {
		secret := make([]byte, 32)
		secret[0] = byte(i)
		invalid := testCtx.apiKey[:len(testCtx.apiKey)-43] + base64.RawURLEncoding.EncodeToString(secret)
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.Header.Set("Authorization", "Bearer "+invalid)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if i >= 3 && w.Code != http.StatusTooManyRequests {
			t.Fatal("rotating credential bypassed ceiling")
		}
	}
	_, total, err := audit.Query(audit.Filter{Action: "pat.auth.failure", Page: 1, PerPage: 100})
	if err != nil || total != 3 {
		t.Fatalf("rotating tokens still reached authentication audit: %d %v", total, err)
	}
	pat, fresh, err := models.CreatePersonalAccessToken(testCtx.admin.Id, "fresh after exhaustion", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{testCtx.apiKey, fresh} {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("valid PAT admission blocked: %d %s", w.Code, w.Body.String())
		}
	}
	if err := models.RevokePersonalAccessToken(pat.ID, testCtx.admin.Id); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
	r.Header.Set("Authorization", "Bearer "+fresh)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("recognition snapshot bypassed revocation: %d", w.Code)
	}
}

func TestInvalidTokenBudgetStopsAuthenticationAuditing(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	invalid := testCtx.apiKey[:len(testCtx.apiKey)-2] + "AA"
	if invalid == testCtx.apiKey {
		invalid = testCtx.apiKey[:len(testCtx.apiKey)-2] + "BA"
	}
	var admitted int64
	for i := 0; i < 8; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.Header.Set("Authorization", "Bearer "+invalid)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		_, total, err := audit.Query(audit.Filter{Action: "pat.auth.failure", Page: 1, PerPage: 100})
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			admitted = total
		}
		if i >= 3 && (w.Code != http.StatusTooManyRequests || total != admitted) {
			t.Fatalf("exhausted authentication still audited: status=%d events=%d expected=%d", w.Code, total, admitted)
		}
	}
}

func TestScopedPATCannotExhaustBrowserOrAnotherToken(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	_, other, err := models.CreatePersonalAccessToken(testCtx.admin.Id, "independent", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.Header.Set("Authorization", "Bearer "+testCtx.apiKey)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if i == 3 && w.Code != http.StatusTooManyRequests {
			t.Fatal("PAT budget not enforced")
		}
	}
	for _, token := range []string{"", other} {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		if token == "" {
			r = ctx.Set(r, "user", testCtx.admin)
		} else {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("independent credential blocked: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestAdministrativeAPILimitsUnauthenticatedRequestsAcrossRoutes(t *testing.T) {
	server := setupTest(t).apiServer
	initial := httptest.NewRecorder()
	server.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/api/pages/", nil))
	if initial.Header().Get("X-RateLimit-Limit") != "100" {
		t.Fatal("default API limit is not 100")
	}
	server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	for i := 0; i < 4; i++ {
		path := "/api/pages/"
		if i%2 == 0 {
			path = "/api/groups/"
		}
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "192.0.2.23:54321"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Header().Get("X-RateLimit-Limit") != "3" {
			t.Fatalf("missing API budget: %v", w.Header())
		}
		if i < 3 && w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d denied too early", i)
		}
		if i == 3 && (w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "20") {
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

func TestAnonymousExhaustionDoesNotBlockAuthenticatedBudget(t *testing.T) {
	testCtx := setupTest(t)
	server := testCtx.apiServer
	server.requestLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.authenticationLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	for i := 0; i < 8; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.RemoteAddr = "127.0.0.1:54321"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
	}
	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
		r.RemoteAddr = "127.0.0.1:54321"
		if i == 0 {
			r.Header.Set("Authorization", "Bearer "+testCtx.apiKey)
		} else {
			r = ctx.Set(r, "user", testCtx.admin)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if i < 4 && w.Code != http.StatusOK {
			t.Fatalf("anonymous traffic blocked authenticated request %d: %d %s", i, w.Code, w.Body.String())
		}
		if i == 4 && w.Code != http.StatusTooManyRequests {
			t.Fatal("browser budget was not enforced independently of PAT requests")
		}
	}
	server.allowedOrigins = []string{"https://admin.example.test"}
	server.authenticationLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
	server.registerRoutes()
	preflight := httptest.NewRequest(http.MethodOptions, "/api/pages/", nil)
	preflight.RemoteAddr = "127.0.0.1:54321"
	preflight.Header.Set("Origin", "https://admin.example.test")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	server.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatal("general budget prevents browser from reaching readable refusal")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/pages/", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	r = ctx.Set(r, "user", models.User{Id: testCtx.admin.Id + 1})
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("another user's authenticated budget was exhausted")
	}
}

func TestPreflightAbuseBudgetPreservesOriginPolicy(t *testing.T) {
	for _, origin := range []string{"https://admin.example.test", "https://untrusted.example.test"} {
		t.Run(origin, func(t *testing.T) {
			server := setupTest(t).apiServer
			server.allowedOrigins = []string{"https://admin.example.test"}
			server.preflightLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
			server.authenticationLimiter = ratelimit.NewPostLimiter(ratelimit.WithRequestsPerMinute(3))
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
