package controllers

import "testing"

func TestAdministrativeReturnPathAllowsEditorRoutes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "/templates/new", want: "/templates/new"},
		{input: "/templates/17/edit", want: "/templates/17/edit"},
		{input: "/templates/17/copy", want: "/templates/17/copy"},
		{input: "/landing_pages/new", want: "/landing_pages/new"},
		{input: "/landing_pages/23/edit", want: "/landing_pages/23/edit"},
		{input: "/landing_pages/23/copy", want: "/landing_pages/23/copy"},
		{input: "/templates/not-a-number/edit", want: "/"},
		{input: "/templates/17/delete", want: "/"},
		{input: "/landing_pages/23/delete", want: "/"},
	}
	for _, tc := range tests {
		if got := administrativeReturnPath(tc.input); got != tc.want {
			t.Errorf("administrativeReturnPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
