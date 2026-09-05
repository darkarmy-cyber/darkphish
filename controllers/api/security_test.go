package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/auth"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/middleware"
	"github.com/darkarmy-cyber/darkphish/models"
)

func newReauthenticationHTTPRequest(t *testing.T, user models.User, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/reauthenticate", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	session, err := middleware.Store.New(request, middleware.CookieName)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := models.NewSessionBinding()
	if err != nil {
		t.Fatal(err)
	}
	session.Values["session_id"] = binding
	request = ctx.Set(request, "user", user)
	request = ctx.Set(request, "auth_method", "session")
	request = ctx.Set(request, "session", session)
	return httptest.NewRecorder(), request
}

func TestPrivilegedReauthenticationAuditsSuccessAndFailureWithoutPassword(t *testing.T) {
	testCtx := setupTest(t)
	password := "reauthentication-password"
	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	testCtx.admin.Hash = hash
	if err := models.PutUser(&testCtx.admin); err != nil {
		t.Fatal(err)
	}

	failed, request := newReauthenticationHTTPRequest(t, testCtx.admin, `{"method":"password","password":"incorrect password"}`)
	testCtx.apiServer.Reauthenticate(failed, request)
	if failed.Code != http.StatusUnauthorized {
		t.Fatalf("failed reauthentication returned %d: %s", failed.Code, failed.Body.String())
	}
	succeeded, request := newReauthenticationHTTPRequest(t, testCtx.admin, `{"method":"password","password":"`+password+`"}`)
	testCtx.apiServer.Reauthenticate(succeeded, request)
	if succeeded.Code != http.StatusOK {
		t.Fatalf("successful reauthentication returned %d: %s", succeeded.Code, succeeded.Body.String())
	}
	if !strings.Contains(succeeded.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("reauthentication response may be cached")
	}
	for _, action := range []string{"auth.reauthentication.failure", "auth.reauthentication.success"} {
		events, total, err := audit.Query(audit.Filter{Action: action, Page: 1, PerPage: 10})
		if err != nil || total != 1 {
			t.Fatalf("expected one %s event, got %d: %v", action, total, err)
		}
		for _, event := range events {
			if strings.Contains(event.Metadata, password) || strings.Contains(event.TargetID, password) {
				t.Fatalf("password leaked into %s audit event", action)
			}
		}
	}
}
