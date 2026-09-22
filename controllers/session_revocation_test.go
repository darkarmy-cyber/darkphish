package controllers

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func sessionTestClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func copiedSessionCookie(t *testing.T, client *http.Client, rawURL string) *http.Cookie {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "darkphish" {
			return &http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: "/"}
		}
	}
	t.Fatal("authenticated session cookie not found")
	return nil
}

func requestWithCopiedCookie(t *testing.T, method, target string, cookie *http.Cookie) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func allowNormalSession(t *testing.T) {
	t.Helper()
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	admin.PasswordChangeRequired = false
	if err := models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}
}

func assertSessionReplayRejected(t *testing.T, baseURL string, cookie *http.Cookie) {
	t.Helper()
	ui := requestWithCopiedCookie(t, http.MethodGet, baseURL+"/", cookie)
	ui.Body.Close()
	if ui.StatusCode != http.StatusTemporaryRedirect || !strings.HasPrefix(ui.Header.Get("Location"), "/login") {
		t.Fatalf("revoked cookie reached UI: status=%d location=%q", ui.StatusCode, ui.Header.Get("Location"))
	}
	api := requestWithCopiedCookie(t, http.MethodGet, baseURL+"/api/campaigns/", cookie)
	api.Body.Close()
	if api.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked cookie reached API: status=%d", api.StatusCode)
	}
}

func TestLogoutRevokesCopiedBrowserSession(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	allowNormalSession(t)
	client := sessionTestClient(t)
	login := attemptLogin(t, ctx, client, "admin", "darkphish", "")
	login.Body.Close()
	if login.StatusCode != http.StatusFound {
		t.Fatalf("login status=%d", login.StatusCode)
	}
	copied := copiedSessionCookie(t, client, ctx.adminServer.URL)

	logout, err := client.Get(ctx.adminServer.URL + "/logout")
	if err != nil {
		t.Fatal(err)
	}
	logout.Body.Close()
	if logout.StatusCode != http.StatusFound {
		t.Fatalf("logout status=%d", logout.StatusCode)
	}
	assertSessionReplayRejected(t, ctx.adminServer.URL, copied)

	newClient := sessionTestClient(t)
	newLogin := attemptLogin(t, ctx, newClient, "admin", "darkphish", "")
	newLogin.Body.Close()
	if newLogin.StatusCode != http.StatusFound {
		t.Fatalf("new login status=%d", newLogin.StatusCode)
	}
}

func TestPasswordChangeRotatesBrowserSessions(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	allowNormalSession(t)
	client := sessionTestClient(t)
	login := attemptLogin(t, ctx, client, "admin", "darkphish", "")
	login.Body.Close()
	copied := copiedSessionCookie(t, client, ctx.adminServer.URL)

	form := url.Values{
		"current_password":     {"darkphish"},
		"new_password":         {"Synthetic-session-password-20!"},
		"confirm_new_password": {"Synthetic-session-password-20!"},
	}
	request, err := http.NewRequest(http.MethodPost, ctx.adminServer.URL+"/settings", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", ctx.adminServer.URL+"/settings")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("password change status=%d", response.StatusCode)
	}
	assertSessionReplayRejected(t, ctx.adminServer.URL, copied)

	current, err := client.Get(ctx.adminServer.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	current.Body.Close()
	if current.StatusCode != http.StatusOK {
		t.Fatalf("rotated session status=%d", current.StatusCode)
	}
	newClient := sessionTestClient(t)
	newLogin := attemptLogin(t, ctx, newClient, "admin", "Synthetic-session-password-20!", "")
	newLogin.Body.Close()
	if newLogin.StatusCode != http.StatusFound {
		t.Fatalf("new-password login status=%d", newLogin.StatusCode)
	}
}

func TestRequiredPasswordResetRotatesBrowserSessions(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	admin.PasswordChangeRequired = true
	if err := models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}

	client := sessionTestClient(t)
	login := attemptLogin(t, ctx, client, "admin", "darkphish", "")
	login.Body.Close()
	copied := copiedSessionCookie(t, client, ctx.adminServer.URL)

	form := url.Values{
		"password":         {"Synthetic-reset-password-20!"},
		"confirm_password": {"Synthetic-reset-password-20!"},
	}
	request, err := http.NewRequest(http.MethodPost, ctx.adminServer.URL+"/reset_password?next=/", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", ctx.adminServer.URL+"/reset_password")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("password reset status=%d", response.StatusCode)
	}
	assertSessionReplayRejected(t, ctx.adminServer.URL, copied)

	newClient := sessionTestClient(t)
	newLogin := attemptLogin(t, ctx, newClient, "admin", "Synthetic-reset-password-20!", "")
	newLogin.Body.Close()
	if newLogin.StatusCode != http.StatusFound {
		t.Fatalf("post-reset login status=%d", newLogin.StatusCode)
	}
}
