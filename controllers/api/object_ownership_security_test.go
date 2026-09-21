package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/models"
)

func objectWriteRequest(t *testing.T, server http.Handler, token, method, path string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func TestCreateAndUpdateObjectOwnershipBoundary(t *testing.T) {
	test := setupTest(t)
	t.Cleanup(func() { _ = models.Close() })

	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	attacker := models.User{
		Username: "api-object-attacker", Hash: "not-used-by-pat",
		ApiKey: "disabled-api-object-attacker", Role: role, RoleID: role.ID,
	}
	if err := models.PutUser(&attacker); err != nil {
		t.Fatal(err)
	}
	_, attackerToken, err := models.CreatePersonalAccessToken(
		attacker.Id, "object boundary", models.AllowedPATScopes(), time.Now().UTC().Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}

	template := models.Template{Name: "api owned template", UserId: test.admin.Id, Subject: "original", Text: "original"}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatal(err)
	}
	page := models.Page{Name: "api owned page", UserId: test.admin.Id, HTML: "<p>original</p>"}
	if err := models.PostPage(&page); err != nil {
		t.Fatal(err)
	}
	smtp := models.SMTP{Name: "api owned smtp", UserId: test.admin.Id, Host: "smtp.example.test:25", FromAddress: "owner@example.test"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path    string
		payload interface{}
	}{
		{"/api/templates/", models.Template{Id: template.Id, Name: "overwrite", Text: "overwrite"}},
		{"/api/pages/", models.Page{Id: page.Id, Name: "overwrite", HTML: "<p>overwrite</p>"}},
		{"/api/smtp/", models.SMTP{Id: smtp.Id, Interface: "SMTP", Name: "overwrite", Host: "evil.example:25", FromAddress: "evil@example.test"}},
	}
	for _, testCase := range cases {
		response := objectWriteRequest(t, test.apiServer, attackerToken, http.MethodPost, testCase.path, testCase.payload)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("POST %s accepted a client supplied id: status=%d body=%s", testCase.path, response.Code, response.Body.String())
		}
	}

	putCases := []struct {
		path    string
		payload interface{}
	}{
		{fmt.Sprintf("/api/templates/%d", template.Id), models.Template{Id: template.Id, Name: "overwrite", Text: "overwrite"}},
		{fmt.Sprintf("/api/pages/%d", page.Id), models.Page{Id: page.Id, Name: "overwrite", HTML: "<p>overwrite</p>"}},
		{fmt.Sprintf("/api/smtp/%d", smtp.Id), models.SMTP{Id: smtp.Id, Interface: "SMTP", Name: "overwrite", Host: "evil.example:25", FromAddress: "evil@example.test"}},
	}
	for _, testCase := range putCases {
		response := objectWriteRequest(t, test.apiServer, attackerToken, http.MethodPut, testCase.path, testCase.payload)
		if response.Code != http.StatusNotFound {
			t.Fatalf("PUT %s crossed the ownership boundary: status=%d body=%s", testCase.path, response.Code, response.Body.String())
		}
	}

	reloadedTemplate, err := models.GetTemplate(template.Id, test.admin.Id)
	if err != nil || reloadedTemplate.Subject != "original" {
		t.Fatal("template changed after rejected cross-user writes")
	}
	reloadedPage, err := models.GetPage(page.Id, test.admin.Id)
	if err != nil || reloadedPage.Name != "api owned page" {
		t.Fatal("page changed after rejected cross-user writes")
	}
	reloadedSMTP, err := models.GetSMTP(smtp.Id, test.admin.Id)
	if err != nil || reloadedSMTP.Host != "smtp.example.test:25" {
		t.Fatal("SMTP profile changed after rejected cross-user writes")
	}
}
