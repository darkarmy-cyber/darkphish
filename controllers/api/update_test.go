package api

import (
	"fmt"
	"github.com/darkarmy-cyber/darkphish/auth"
	"net/http"
	"net/http/httptest"
	"strings"
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


func TestDecodeUpdateTargetAcceptsOnlyVersion(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		wantErr          bool
	}{
		{name: "valid", body: `{"version":"0.21.0"}`, want: "0.21.0"},
		{name: "missing", body: `{}`, wantErr: true},
		{name: "unknown field", body: `{"version":"0.21.0","url":"https://example.invalid"}`, wantErr: true},
		{name: "multiple objects", body: `{"version":"0.21.0"}{"version":"0.22.0"}`, wantErr: true},
		{name: "oversized", body: `{"version":"` + strings.Repeat("1", 2048) + `"}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/updates/apply", strings.NewReader(tc.body))
			got, err := decodeUpdateTarget(w, r)
			if tc.wantErr {
				if err == nil || got != "" {
					t.Fatalf("invalid target accepted: %q %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("target mismatch: %q %v", got, err)
			}
		})
	}
}
