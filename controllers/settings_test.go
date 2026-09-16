package controllers

import (
	"github.com/darkarmy-cyber/darkphish/models"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSettingsAdminTabsAndBell(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatal(err)
	}
	admin.PasswordChangeRequired = false
	if err = models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	login := attemptLogin(t, ctx, client, admin.Username, "darkphish", "")
	login.Body.Close()
	for _, tab := range []string{"users", "webhooks", "audit", "update", "licensing"} {
		resp, err := client.Get(ctx.adminServer.URL + "/settings?tab=" + tab)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `id="adminNotifications"`) || !strings.Contains(string(body), `href="/settings?tab=api"`) {
			t.Fatalf("missing admin settings content for %s: %d", tab, resp.StatusCode)
		}
	}
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	admin.RoleID = role.ID
	admin.Role = role
	if err = models.PutUser(&admin); err != nil {
		t.Fatal(err)
	}
	for _, tab := range []string{"users", "webhooks", "audit", "update", "licensing"} {
		resp, err := client.Get(ctx.adminServer.URL + "/settings?tab=" + tab)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("non-admin accessed %s: %d", tab, resp.StatusCode)
		}
	}
	resp, err := client.Get(ctx.adminServer.URL + "/settings?tab=account")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), `id="adminNotifications"`) || strings.Contains(string(body), `href="/settings?tab=users"`) {
		t.Fatal("non-admin received privileged navigation")
	}
}

func TestSettingsReturnPaths(t *testing.T) {
	for _, tab := range []string{"account", "ui", "reporting", "api", "users", "webhooks", "audit", "update", "licensing"} {
		raw := "/settings?tab=" + tab
		if got := administrativeReturnPath(raw); got != raw {
			t.Fatalf("%s -> %s", raw, got)
		}
	}
	for _, raw := range []string{"/settings?tab=https://evil.example", "/settings?tab=../update", "/settings?tab=Update"} {
		if got := administrativeReturnPath(raw); got != "/settings" {
			t.Fatalf("unsafe tab retained: %s", got)
		}
	}
	for _, raw := range []string{"https://evil.example/settings?tab=update", "//evil.example/settings?tab=update", "/%73ettings?tab=update"} {
		if got := administrativeReturnPath(raw); got != "/" {
			t.Fatalf("unsafe return path retained: %s", got)
		}
	}
}
