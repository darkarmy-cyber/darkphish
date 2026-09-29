package controllers

import "testing"

func TestAdministrativeReturnPathAllowsEditorRoutes(t *testing.T) {
	tests := map[string]string{
		"/templates/new":             "/templates/new",
		"/templates/17/edit":         "/templates/17/edit",
		"/templates/17/copy":         "/templates/17/copy",
		"/landing_pages/new":         "/landing_pages/new",
		"/landing_pages/23/edit":     "/landing_pages/23/edit",
		"/landing_pages/23/copy":     "/landing_pages/23/copy",
		"/templates/not-a-number/edit": "/",
		"/templates/17/delete":       "/",
		"/landing_pages/23/delete":   "/",
	}
	for input, want := range tests {
		if got := administrativeReturnPath(input); got != want {
			t.Errorf("administrativeReturnPath(%q) = %q, want %q", input, got, want)
		}
	}
}
