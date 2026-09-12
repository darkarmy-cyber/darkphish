package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctxpkg "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/models"
)

func seedIMAPHotfixTest(t *testing.T, enabled bool, password string) {
	t.Helper()
	im := models.IMAP{
		Enabled:  enabled,
		Host:     "127.0.0.1",
		Port:     993,
		Username: "reports@example.test",
		Password: password,
		TLS:      true,
		Folder:   models.DefaultIMAPFolder,
		IMAPFreq: models.DefaultIMAPFreq,
	}
	if err := models.PostIMAP(&im, 1); err != nil {
		t.Fatalf("seed IMAP config: %v", err)
	}
}

func postIMAPHotfixTest(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/imapserver/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = ctxpkg.Set(req, "user_id", int64(1))
	resp := httptest.NewRecorder()
	server.IMAPServer(resp, req)
	return resp
}

func getIMAPHotfixTest(t *testing.T, server *Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/imapserver/", nil)
	req = ctxpkg.Set(req, "user_id", int64(1))
	resp := httptest.NewRecorder()
	server.IMAPServer(resp, req)
	return resp
}

func TestIMAPUpdateWithoutPasswordPreservesSecretAndEnabledState(t *testing.T) {
	testCtx := setupTest(t)
	seedIMAPHotfixTest(t, true, "old-secret")

	resp := postIMAPHotfixTest(t, testCtx.apiServer, `{"enabled":false,"host":"127.0.0.1","port":"993","username":"reports@example.test","password":"","tls":true,"folder":"INBOX","imap_freq":"60"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	stored, err := models.GetIMAP(1)
	if err != nil || len(stored) != 1 {
		t.Fatalf("read IMAP config: len=%d err=%v", len(stored), err)
	}
	if stored[0].Enabled {
		t.Fatal("expected monitoring to be disabled")
	}
	if stored[0].Password != "old-secret" {
		t.Fatalf("password was not preserved")
	}
}

func TestIMAPUpdateFieldsWithoutPasswordPreservesSecret(t *testing.T) {
	testCtx := setupTest(t)
	seedIMAPHotfixTest(t, true, "old-secret")

	resp := postIMAPHotfixTest(t, testCtx.apiServer, `{"enabled":true,"host":"localhost","port":"994","username":"reports@example.test","password":"","tls":false,"folder":"INBOX","imap_freq":"60"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	stored, err := models.GetIMAP(1)
	if err != nil || len(stored) != 1 {
		t.Fatalf("read IMAP config: len=%d err=%v", len(stored), err)
	}
	if stored[0].Password != "old-secret" || stored[0].Host != "localhost" || stored[0].Port != 994 || stored[0].TLS {
		t.Fatalf("unexpected stored config: host=%q port=%d tls=%v password-preserved=%v", stored[0].Host, stored[0].Port, stored[0].TLS, stored[0].Password == "old-secret")
	}
}

func TestIMAPExplicitPasswordReplacement(t *testing.T) {
	testCtx := setupTest(t)
	seedIMAPHotfixTest(t, true, "old-secret")

	resp := postIMAPHotfixTest(t, testCtx.apiServer, `{"enabled":true,"host":"127.0.0.1","port":"993","username":"reports@example.test","password":"new-secret","tls":true,"folder":"INBOX","imap_freq":"60"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	stored, err := models.GetIMAP(1)
	if err != nil || len(stored) != 1 {
		t.Fatalf("read IMAP config: len=%d err=%v", len(stored), err)
	}
	if stored[0].Password != "new-secret" {
		t.Fatal("explicit replacement password was not persisted")
	}
}

func TestIMAPFirstConfigurationWithoutPasswordRejected(t *testing.T) {
	testCtx := setupTest(t)
	resp := postIMAPHotfixTest(t, testCtx.apiServer, `{"enabled":false,"host":"127.0.0.1","port":"993","username":"reports@example.test","password":"","tls":true,"folder":"INBOX","imap_freq":"60"}`)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), models.ErrIMAPPasswordNotSpecified.Error()) {
		t.Fatalf("expected missing-password validation error, got %s", resp.Body.String())
	}
}

func TestIMAPGETExposesPasswordSetWithoutPasswordMaterial(t *testing.T) {
	testCtx := setupTest(t)
	seedIMAPHotfixTest(t, true, "top-secret-value")

	resp := getIMAPHotfixTest(t, testCtx.apiServer)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, "top-secret-value") || strings.Contains(body, `"password":`) {
		t.Fatalf("GET exposed password material: %s", body)
	}
	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0]["password_set"] != true {
		t.Fatalf("expected password_set=true, got %v", payload)
	}
}
