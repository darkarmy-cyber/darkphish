package controllers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAdministrativeReturnDestinations(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"/", "/"}, {"/campaigns", "/campaigns"}, {"/campaigns/123", "/campaigns/123"},
		{"/settings?tab=profile#section", "/settings"}, {"/reset_password", "/reset_password"},
		{"", "/"}, {"campaigns", "/"}, {"https://outside.example.test/campaigns", "/"},
		{"//outside.example.test/campaigns", "/"}, {"javascript:alert(1)", "/"},
		{"/\\outside.example.test", "/"}, {"/%2foutside.example.test", "/"},
		{"/%252foutside.example.test", "/"}, {"/campaigns/../logout", "/"},
		{"/campaigns/1%0d%0aLocation:outside", "/"}, {"/unknown", "/"},
		{"/logout", "/"}, {"/impersonate", "/"}, {"/campaigns/1/extra", "/"},
	} {
		t.Run(test.input, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/login?next="+url.QueryEscape(test.input), nil)
			w := httptest.NewRecorder()
			(&AdminServer{}).nextOrIndex(w, r)
			if w.Code != http.StatusFound || w.Header().Get("Location") != test.want {
				t.Fatalf("got status=%d location=%q, want %q", w.Code, w.Header().Get("Location"), test.want)
			}
		})
	}
}
