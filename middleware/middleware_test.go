package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
)

var successHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("success"))
})

type testContext struct {
	apiKey string
	userID int64
}

func setupTest(t *testing.T) *testContext {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	err := models.Setup(conf)
	if err != nil {
		t.Fatalf("Failed creating database: %v", err)
	}
	// Get the API key to use for these tests
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting user: %v", err)
	}
	u.PasswordChangeRequired = false
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("error activating test user: %v", err)
	}
	ctx := &testContext{}
	_, ctx.apiKey, err = models.CreatePersonalAccessToken(u.Id, "middleware tests", models.AllowedPATScopes(), time.Now().UTC().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("error creating test personal access token: %v", err)
	}
	ctx.userID = u.Id
	return ctx
}

// MiddlewarePermissionTest maps an expected HTTP Method to an expected HTTP
// status code
type MiddlewarePermissionTest map[string]int

// TestEnforceViewOnly ensures that only users with the ModifyObjects
// permission have the ability to send non-GET requests.
func TestEnforceViewOnly(t *testing.T) {
	setupTest(t)
	permissionTests := map[string]MiddlewarePermissionTest{
		models.RoleAdmin: MiddlewarePermissionTest{
			http.MethodGet:     http.StatusOK,
			http.MethodHead:    http.StatusOK,
			http.MethodOptions: http.StatusOK,
			http.MethodPost:    http.StatusOK,
			http.MethodPut:     http.StatusOK,
			http.MethodDelete:  http.StatusOK,
		},
		models.RoleUser: MiddlewarePermissionTest{
			http.MethodGet:     http.StatusOK,
			http.MethodHead:    http.StatusOK,
			http.MethodOptions: http.StatusOK,
			http.MethodPost:    http.StatusOK,
			http.MethodPut:     http.StatusOK,
			http.MethodDelete:  http.StatusOK,
		},
	}
	for r, checks := range permissionTests {
		role, err := models.GetRoleBySlug(r)
		if err != nil {
			t.Fatalf("error getting role by slug: %v", err)
		}

		for method, expected := range checks {
			req := httptest.NewRequest(method, "/", nil)
			response := httptest.NewRecorder()

			req = ctx.Set(req, "user", models.User{
				Role:   role,
				RoleID: role.ID,
			})

			EnforceViewOnly(successHandler).ServeHTTP(response, req)
			got := response.Code
			if got != expected {
				t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
			}
		}
	}
}

func TestRequirePermission(t *testing.T) {
	setupTest(t)
	middleware := RequirePermission(models.PermissionModifySystem)
	handler := middleware(successHandler)

	permissionTests := map[string]int{
		models.RoleUser:  http.StatusForbidden,
		models.RoleAdmin: http.StatusOK,
	}

	for role, expected := range permissionTests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()
		// Test that with the requested permission, the request succeeds
		role, err := models.GetRoleBySlug(role)
		if err != nil {
			t.Fatalf("error getting role by slug: %v", err)
		}
		req = ctx.Set(req, "user", models.User{
			Role:   role,
			RoleID: role.ID,
		})
		handler.ServeHTTP(response, req)
		got := response.Code
		if got != expected {
			t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
		}
	}
}

func TestRequireAPIAuthentication(t *testing.T) {
	setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	// Test that making a request without an API key is denied
	RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
	expected := http.StatusUnauthorized
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
}

func TestCORSHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://admin.example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()
	CORS([]string{"https://admin.example.test"})(successHandler).ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example.test" {
		t.Fatalf("unexpected allowed origin %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("administrative CORS must never use a wildcard")
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://untrusted.example.test")
	response := httptest.NewRecorder()
	CORS(nil)(successHandler).ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header %q", got)
	}
}

func TestInvalidAPIKey(t *testing.T) {
	setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	query := req.URL.Query()
	query.Set("api_key", "bogus-api-key")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
	expected := http.StatusUnauthorized
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
}

func TestBearerToken(t *testing.T) {
	testCtx := setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
	expected := http.StatusOK
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
	_, total, err := audit.Query(audit.Filter{Action: "pat.auth.success", Page: 1, PerPage: 10})
	if err != nil || total != 1 {
		t.Fatalf("expected one PAT authentication audit event, got total=%d err=%v", total, err)
	}
}

func TestPATAuthorizationFailureIsAudited(t *testing.T) {
	testCtx := setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	req.Header.Set("Authorization", "Bearer "+testCtx.apiKey)
	response := httptest.NewRecorder()
	RequireAPIAuthentication(EnforcePATScopes(successHandler)).ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, response.Code)
	}
	_, total, err := audit.Query(audit.Filter{Action: "pat.authorization.failure", Page: 1, PerPage: 10})
	if err != nil || total != 1 {
		t.Fatalf("expected one PAT authorization failure event, got total=%d err=%v", total, err)
	}
}

func TestRequiredPATScopesAreResourceSpecific(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/groups/", "groups:read"},
		{http.MethodPost, "/api/groups/", "groups:write"},
		{http.MethodGet, "/api/templates/1", "templates:read"},
		{http.MethodPut, "/api/pages/1", "landing-pages:write"},
		{http.MethodPost, "/api/smtp/", "sending-profiles:write"},
		{http.MethodGet, "/api/imap/", "integrations:read"},
		{http.MethodPost, "/api/import/site", "landing-pages:write"},
		{http.MethodPost, "/api/campaigns/1/results/rid/credential/reveal", "credentials:view"},
		{http.MethodGet, "/api/unknown", ""},
	}
	for _, test := range tests {
		if got := requiredPATScope(test.method, test.path); got != test.want {
			t.Errorf("%s %s: got %q, want %q", test.method, test.path, got, test.want)
		}
	}
}

func TestAPIKeyInURLIsRejected(t *testing.T) {
	testCtx := setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/?api_key="+testCtx.apiKey, nil)
	response := httptest.NewRecorder()
	RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestBearerTokenEnforcesAccountState(t *testing.T) {
	tests := []struct {
		name           string
		locked         bool
		changeRequired bool
	}{
		{name: "locked", locked: true},
		{name: "password change required", changeRequired: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCtx := setupTest(t)
			u, err := models.GetUser(testCtx.userID)
			if err != nil {
				t.Fatal(err)
			}
			u.AccountLocked = tt.locked
			u.PasswordChangeRequired = tt.changeRequired
			if err := models.PutUser(&u); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
			response := httptest.NewRecorder()
			RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected %d, got %d", http.StatusForbidden, response.Code)
			}
		})
	}
}

func TestSessionAuthentication(t *testing.T) {
	setupTest(t)
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = ctx.Set(req, "user", u)
	response := httptest.NewRecorder()
	RequireAPIAuthentication(successHandler).ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, response.Code)
	}
}

func TestPasswordResetRequired(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = ctx.Set(req, "user", models.User{
		PasswordChangeRequired: true,
	})
	response := httptest.NewRecorder()
	RequireLogin(successHandler).ServeHTTP(response, req)
	gotStatus := response.Code
	expectedStatus := http.StatusTemporaryRedirect
	if gotStatus != expectedStatus {
		t.Fatalf("incorrect status code received. expected %d got %d", expectedStatus, gotStatus)
	}
	expectedLocation := "/reset_password?next=%2F"
	gotLocation := response.Header().Get("Location")
	if gotLocation != expectedLocation {
		t.Fatalf("incorrect location header received. expected %s got %s", expectedLocation, gotLocation)
	}
}

func TestApplySecurityHeaders(t *testing.T) {
	expected := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	ApplySecurityHeaders(successHandler).ServeHTTP(response, req)
	for header, value := range expected {
		got := response.Header().Get(header)
		if got != value {
			t.Fatalf("incorrect security header received for %s: expected %s got %s", header, value, got)
		}
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("content security policy was not set")
	}
}
