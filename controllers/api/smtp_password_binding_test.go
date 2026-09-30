package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/internal/licensetest"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"github.com/darkarmy-cyber/darkphish/models"
)

type passwordBindingWorker struct {
	licenseTestWorker
	profile models.SMTP
}

func (w *passwordBindingWorker) SendTestEmail(req *models.EmailRequest) error {
	w.calls++
	w.profile = req.SMTP
	return nil
}

func TestSMTPPasswordBinding(t *testing.T) {
	ctx := setupTest(t)
	models.ConfigureLicenseManager(licensetest.Manager(t, licensing.StateActive))
	t.Cleanup(func() { models.ConfigureLicenseManager(nil) })
	worker := &passwordBindingWorker{}
	ctx.apiServer.worker = worker
	stored := models.SMTP{UserId: ctx.admin.Id, Name: "saved", Interface: "SMTP", Host: "smtp.example.test:587", Username: "owner", Password: "stored-secret", FromAddress: "sender@example.test"}
	if err := models.PostSMTP(&stored); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, profile map[string]interface{}) *httptest.ResponseRecorder {
		t.Helper()
		var payload interface{} = profile
		if method == http.MethodPost {
			payload = map[string]interface{}{"email": "recipient@example.test", "smtp": profile}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+ctx.apiKey)
		response := httptest.NewRecorder()
		ctx.apiServer.ServeHTTP(response, r)
		if strings.Contains(response.Body.String(), stored.Password) {
			t.Fatal("response exposed the stored password")
		}
		return response
	}
	profile := func() map[string]interface{} {
		return map[string]interface{}{"id": stored.Id, "name": stored.Name, "host": stored.Host, "username": stored.Username, "password": "", "from_address": stored.FromAddress, "ignore_cert_errors": false}
	}
	for _, tc := range []struct {
		field string
		value interface{}
	}{
		{"host", "other.example.test:587"},
		{"host", "smtp.example.test:25"},
		{"username", "another-user"},
		{"ignore_cert_errors", true},
		{"interface_type", "OTHER"},
	} {
		t.Run(fmt.Sprint(tc.field, tc.value), func(t *testing.T) {
			for _, method := range []string{http.MethodPost, http.MethodPut} {
				p := profile()
				p[tc.field] = tc.value
				path := "/api/util/send_test_email"
				if method == http.MethodPut {
					path = fmt.Sprintf("/api/smtp/%d", stored.Id)
				}
				before := worker.calls
				response := request(method, path, p)
				if response.Code != http.StatusBadRequest || worker.calls != before {
					t.Fatalf("%s accepted changed binding: %d %s", method, response.Code, response.Body.String())
				}
				got, err := models.GetSMTP(stored.Id, stored.UserId)
				if err != nil || got.Host != stored.Host || got.Username != stored.Username || got.IgnoreCertErrors != stored.IgnoreCertErrors || got.Password != stored.Password {
					t.Fatal("rejected request changed stored connection")
				}
			}
		})
	}
	for _, p := range []map[string]interface{}{profile(), {"name": stored.Name}} {
		response := request(http.MethodPost, "/api/util/send_test_email", p)
		if response.Code != http.StatusOK || worker.profile.Password != stored.Password || worker.profile.Host != stored.Host {
			t.Fatalf("unchanged or name-only reuse failed: %d %s", response.Code, response.Body.String())
		}
	}
	p := profile()
	p["host"], p["password"] = "replacement.example.test:587", "explicit-new-secret"
	response := request(http.MethodPost, "/api/util/send_test_email", p)
	if response.Code != http.StatusOK || worker.profile.Password != "explicit-new-secret" {
		t.Fatalf("explicit test password rejected: %d %s", response.Code, response.Body.String())
	}
	foreign := stored
	foreign.Id, foreign.UserId, foreign.Name = 0, stored.UserId+100, "foreign"
	if err := models.PostSMTP(&foreign); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{foreign.Id, foreign.Id + 1000} {
		p = profile()
		p["id"] = id
		before := worker.calls
		if response = request(http.MethodPost, "/api/util/send_test_email", p); response.Code != http.StatusBadRequest || worker.calls != before {
			t.Fatal("foreign or missing ID reached worker")
		}
	}
	p = profile()
	p["name"], p["from_address"] = "renamed", "new-sender@example.test"
	response = request(http.MethodPut, fmt.Sprintf("/api/smtp/%d", stored.Id), p)
	got, err := models.GetSMTP(stored.Id, stored.UserId)
	if response.Code != http.StatusOK || err != nil || got.Password != stored.Password || got.Name != "renamed" {
		t.Fatalf("non-connection edit failed: %d %s", response.Code, response.Body.String())
	}
	p["host"], p["password"] = "replacement.example.test:587", "replacement-secret"
	response = request(http.MethodPut, fmt.Sprintf("/api/smtp/%d", stored.Id), p)
	got, err = models.GetSMTP(stored.Id, stored.UserId)
	if response.Code != http.StatusOK || err != nil || got.Password != "replacement-secret" || got.Host != p["host"] {
		t.Fatalf("explicit password update failed: %d %s", response.Code, response.Body.String())
	}
}
