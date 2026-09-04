package controllers

import (
	"net/url"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func TestSubmittedFieldNamesNeverRetainsValues(t *testing.T) {
	form := url.Values{
		models.RecipientParameter: {"abc1234"},
		"username":                {"employee@example.test"},
		"password":                {"should-never-be-stored"},
	}
	got := submittedFieldNames(form, true)
	if _, ok := got[models.RecipientParameter]; ok {
		t.Fatal("recipient identifier leaked into submission metadata")
	}
	for _, name := range []string{"username", "password"} {
		values, ok := got[name]
		if !ok {
			t.Fatalf("missing submitted field name %q", name)
		}
		if len(values) != 0 {
			t.Fatalf("submitted values retained for %q: %#v", name, values)
		}
	}
	if got := submittedFieldNames(form, false); got != nil {
		t.Fatalf("metadata-only mode retained fields: %#v", got)
	}
}
