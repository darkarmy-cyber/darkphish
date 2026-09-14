package api

import (
	"fmt"
	"github.com/darkarmy-cyber/darkphish/auth"
	"net/http"
	"net/http/httptest"
	"testing"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/models"
)

func TestUpdateRequiresFreshBrowserReauthentication(t *testing.T) {
	testCtx := setupTest(t)
	w, r := newReauthenticationHTTPRequest(t, testCtx.admin, `{}`)
	testCtx.apiServer.UpdateApply(w, r)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("without fresh proof: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = ctx.Set(r, "auth_method", "pat")
	testCtx.apiServer.UpdateApply(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("PAT accepted: %d", w.Code)
	}
	password := "update-reauthentication-test-password"
	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	testCtx.admin.Hash = hash
	if err = models.PutUser(&testCtx.admin); err != nil {
		t.Fatal(err)
	}
	w, r = newReauthenticationHTTPRequest(t, testCtx.admin, `{"method":"password","password":"`+password+`"}`)
	testCtx.apiServer.Reauthenticate(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("reauth failed: %d", w.Code)
	}
	w = httptest.NewRecorder()
	testCtx.apiServer.UpdateApply(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("fresh proof did not reach update service boundary: %d", w.Code)
	}
}

func TestUpdateRBAC(t *testing.T) {
	testCtx := setupTest(t)
	user := createUnpriviledgedUser(t, models.RoleUser)
	for _, tc := range []struct{ method, path string }{{http.MethodGet, "/api/updates"}, {http.MethodPost, "/api/updates/check"}, {http.MethodPost, "/api/updates/apply"}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", user.ApiKey))
		w := httptest.NewRecorder()
		testCtx.apiServer.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
