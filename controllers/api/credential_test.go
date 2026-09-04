package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/models"
)

func createCredentialReviewCampaign(t *testing.T, userID int64, credential string) (models.Campaign, models.Result) {
	t.Helper()
	group := models.Group{Name: "Credential test group", UserId: userID}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "synthetic@example.test", FirstName: "Synthetic", LastName: "Target"}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatal(err)
	}
	template := models.Template{Name: "Credential test template", Subject: "Synthetic", Text: "Synthetic", HTML: "<p>Synthetic</p>", UserId: userID}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatal(err)
	}
	page := models.Page{Name: "Credential test page", HTML: `<form><input type="password" name="password"></form>`, CaptureCredentials: true, CapturePasswords: true, UserId: userID}
	if err := models.PostPage(&page); err != nil {
		t.Fatal(err)
	}
	smtp := models.SMTP{Name: "Credential test SMTP", Host: "example.test:25", FromAddress: "sender@example.test", UserId: userID}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}
	campaign := models.Campaign{
		Name: "Credential boundary test", UserId: userID, Template: template, Page: page, SMTP: smtp,
		Groups: []models.Group{group}, CredentialCaptureMode: models.CredentialModeEncryptedReview,
		CredentialRetentionHours: 24,
		CredentialPolicy:         models.CredentialPolicy{MinLength: 12, MaxLength: 128},
	}
	if err := models.PostCampaign(&campaign, userID); err != nil {
		t.Fatal(err)
	}
	result := campaign.Results[0]
	if err := models.RecordCredentialSubmission(campaign, result, credential); err != nil {
		t.Fatal(err)
	}
	return campaign, result
}

func apiRequest(t *testing.T, server *Server, token, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestCredentialRevealBoundaryAndAudit(t *testing.T) {
	testCtx := setupTest(t)
	credential := "Synthetic-boundary-password-9!"
	campaign, result := createCredentialReviewCampaign(t, testCtx.admin.Id, credential)
	var logs bytes.Buffer
	previousLogOutput := log.Logger.Out
	log.Logger.SetOutput(&logs)
	t.Cleanup(func() { log.Logger.SetOutput(previousLogOutput) })

	for _, path := range []string{
		"/api/campaigns/",
		fmt.Sprintf("/api/campaigns/%d", campaign.Id),
		fmt.Sprintf("/api/campaigns/%d/results", campaign.Id),
	} {
		response := apiRequest(t, testCtx.apiServer, testCtx.apiKey, http.MethodGet, path)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), credential) {
			t.Fatalf("normal API response %s exposed credential", path)
		}
	}

	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := models.User{Username: "credential-review-denied", Hash: "not-used", ApiKey: "disabled-review-test", Role: role, RoleID: role.ID}
	if err := models.PutUser(&ordinary); err != nil {
		t.Fatal(err)
	}
	_, ordinaryToken, err := models.CreatePersonalAccessToken(ordinary.Id, "denied reveal", []string{"credentials:view"}, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	revealPath := fmt.Sprintf("/api/campaigns/%d/results/%s/credential/reveal", campaign.Id, result.RId)
	denied := apiRequest(t, testCtx.apiServer, ordinaryToken, http.MethodPost, revealPath)
	if denied.Code != http.StatusForbidden || strings.Contains(denied.Body.String(), credential) {
		t.Fatalf("unauthorized reveal returned %d: %s", denied.Code, denied.Body.String())
	}

	revealed := apiRequest(t, testCtx.apiServer, testCtx.apiKey, http.MethodPost, revealPath)
	if revealed.Code != http.StatusOK || !strings.Contains(revealed.Body.String(), credential) {
		t.Fatalf("authorized reveal returned %d: %s", revealed.Code, revealed.Body.String())
	}
	if got := revealed.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("credential reveal is cacheable: %q", got)
	}

	events, total, err := audit.Query(audit.Filter{Action: "credential.view", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("expected failed and successful credential.view audit events, got %d", total)
	}
	for _, event := range events {
		if strings.Contains(event.Metadata, credential) || strings.Contains(event.TargetID, credential) {
			t.Fatal("credential value leaked into audit event")
		}
	}
	if strings.Contains(logs.String(), credential) {
		t.Fatal("credential value leaked into application or audit logs")
	}
}
