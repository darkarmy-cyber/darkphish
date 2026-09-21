package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/middleware"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/sessions"
)

func sensitivePATRequest(t *testing.T, server *Server, user models.User, binding string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(createPATRequest{
		Name: "credential review token", Scopes: []string{"credentials:view"},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/pats/", bytes.NewReader(payload))
	session := sessions.NewSession(middleware.Store, middleware.CookieName)
	session.Values["id"] = user.Id
	session.Values["session_id"] = binding
	request = ctx.Set(request, "session", session)
	request = ctx.Set(request, "user", user)
	request = ctx.Set(request, "user_id", user.Id)
	request = ctx.Set(request, "auth_method", "session")
	response := httptest.NewRecorder()
	server.PersonalAccessTokens(response, request)
	return response
}

func TestCredentialsViewPATRequiresFreshBrowserReauthentication(t *testing.T) {
	test := setupTest(t)
	t.Cleanup(func() { _ = models.Close() })
	const password = "Synthetic-pat-reauth-password-20!"
	hash, err := auth.GeneratePasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	test.admin.Hash = hash
	if err := models.PutUser(&test.admin); err != nil {
		t.Fatal(err)
	}
	campaign, result := createCredentialReviewCampaign(t, test.admin.Id, "Synthetic-revealed-credential-20!")

	binding, err := models.NewSessionBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := models.CreateBrowserSession(test.admin.Id, binding, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stale := sensitivePATRequest(t, test.apiServer, test.admin, binding)
	if stale.Code != http.StatusPreconditionRequired || strings.Contains(stale.Body.String(), "darkphish_pat_") {
		t.Fatalf("stale session issued sensitive PAT: status=%d body=%s", stale.Code, stale.Body.String())
	}

	if _, err := models.ReauthenticatePrivileged(
		context.Background(), test.admin, binding,
		models.ReauthenticationProof{Method: "password", Secret: password}, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	fresh := sensitivePATRequest(t, test.apiServer, test.admin, binding)
	if fresh.Code != http.StatusCreated {
		t.Fatalf("fresh session PAT status=%d body=%s", fresh.Code, fresh.Body.String())
	}
	var issued createPATResponse
	if err := json.Unmarshal(fresh.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" {
		t.Fatal("fresh session did not receive the one-time token")
	}

	revealPath := fmt.Sprintf("/api/campaigns/%d/results/%s/credential/reveal", campaign.Id, result.RId)
	revealed := apiRequest(t, test.apiServer, issued.Token, http.MethodPost, revealPath)
	if revealed.Code != http.StatusOK || !strings.Contains(revealed.Body.String(), "Synthetic-revealed-credential-20!") {
		t.Fatalf("authorized sensitive PAT could not reveal credential: status=%d body=%s", revealed.Code, revealed.Body.String())
	}
}
