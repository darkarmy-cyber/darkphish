package controllers

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func attemptLogin(t *testing.T, ctx *testContext, client *http.Client, username, password, optionalPath string) *http.Response {
	if client == nil {
		client = &http.Client{}
	}
	if client.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("error creating cookie jar: %v", err)
		}
		client.Jar = jar
	}
	resp, err := client.Get(fmt.Sprintf("%s/login", ctx.adminServer.URL))
	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}
	got := resp.StatusCode
	expected := http.StatusOK
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}

	defer resp.Body.Close()
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/login%s", ctx.adminServer.URL, optionalPath), strings.NewReader(url.Values{
		"username": {username},
		"password": {password},
	}.Encode()))
	if err != nil {
		t.Fatalf("error creating new /login request: %v", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", fmt.Sprintf("%s/login", ctx.adminServer.URL))

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}
	return resp
}

func TestLoginCSRF(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/login", ctx.adminServer.URL), strings.NewReader(url.Values{
		"username": {"admin"},
		"password": {"darkphish"},
	}.Encode()))
	if err != nil {
		t.Fatalf("error creating cross-origin login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://attacker.example.test")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}
	defer resp.Body.Close()

	got := resp.StatusCode
	expected := http.StatusForbidden
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestTrustedOriginUsesExactScheme(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	ctx.config.AdminConf.TrustedOrigins = []string{"https://admin.example.test"}
	server := httptest.NewServer(NewAdminServer(ctx.config.AdminConf).server.Handler)
	defer server.Close()

	for _, tc := range []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{name: "trusted HTTPS origin", origin: "https://admin.example.test", wantStatus: http.StatusUnauthorized},
		{name: "same host over HTTP", origin: "http://admin.example.test", wantStatus: http.StatusForbidden},
		{name: "host suffix", origin: "https://admin.example.test.attacker.test", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/login", strings.NewReader(url.Values{
				"username": {"admin"},
				"password": {"incorrect"},
			}.Encode()))
			if err != nil {
				t.Fatalf("error creating login request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("error requesting the login endpoint: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestBearerAPIBypassesBrowserCrossOriginProtection(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	user, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting API user: %v", err)
	}
	user.PasswordChangeRequired = false
	if err := models.PutUser(&user); err != nil {
		t.Fatalf("error enabling normal API access: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ctx.adminServer.URL+"/api/reset", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("error creating token rotation request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ctx.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://external-client.example.test")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error rotating API token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestSessionAPIMutationRejectsCrossOriginRequest(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	client := &http.Client{}
	loginResponse := attemptLogin(t, ctx, client, "admin", "darkphish", "")
	loginResponse.Body.Close()

	req, err := http.NewRequest(http.MethodPost, ctx.adminServer.URL+"/api/reset", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("error creating token rotation request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.example.test")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("error requesting token rotation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestInvalidCredentials(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "admin", "bogus", "")
	got := resp.StatusCode
	expected := http.StatusUnauthorized
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestSuccessfulLogin(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "admin", "darkphish", "")
	got := resp.StatusCode
	expected := http.StatusOK
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestSuccessfulRedirect(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	next := "/campaigns"
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	resp := attemptLogin(t, ctx, client, "admin", "darkphish", fmt.Sprintf("?next=%s", next))
	got := resp.StatusCode
	expected := http.StatusFound
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
	url, err := resp.Location()
	if err != nil {
		t.Fatalf("error parsing response Location header: %v", err)
	}
	if url.Path != next {
		t.Fatalf("unexpected Location header received. expected %s got %s", next, url.Path)
	}
}

func TestAccountLocked(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "houdini", "darkphish", "")
	got := resp.StatusCode
	expected := http.StatusUnauthorized
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}
