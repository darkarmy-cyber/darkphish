package controllers

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func TestSubmittedFieldNamesNeverRetainsValues(t *testing.T) {
	form := url.Values{
		models.RecipientParameter:      {"abc1234"},
		"username":                     {"employee@example.test"},
		"password":                     {"should-never-be-stored"},
		"should-never-be-a-field-name": {"value"},
	}
	got := submittedFieldNames(form, true)
	if _, ok := got["should-never-be-a-field-name"]; ok {
		t.Fatal("untrusted field name was persisted verbatim")
	}
	if _, ok := got["other"]; !ok {
		t.Fatal("unknown field was not reduced to the other category")
	}
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
	encoded, err := json.Marshal(models.EventDetails{Payload: got})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"employee@example.test", "should-never-be-stored", "should-never-be-a-field-name"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("webhook-compatible event metadata exposed submitted data %q: %s", secret, encoded)
		}
	}
	if got := submittedFieldNames(form, false); got != nil {
		t.Fatalf("metadata-only mode retained fields: %#v", got)
	}
}
