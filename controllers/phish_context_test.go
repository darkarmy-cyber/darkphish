package controllers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRecipientIDComesOnlyFromURLQuery(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/?rid=query-recipient", strings.NewReader("rid=body-recipient&username=alice"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if got := recipientIDFromRequest(request); got != "query-recipient" {
		t.Fatalf("POST body overrode trusted routing id: %q", got)
	}
	if got := request.Form.Get("rid"); got != "body-recipient" {
		t.Fatalf("test did not create the intended merged-form collision: %q", got)
	}
}

func TestRecipientIDPreservesGETAndTransparencySuffix(t *testing.T) {
	for _, target := range []string{"/?rid=result-id%2B", "/?rid=result-id+"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		got := recipientIDFromRequest(request)
		if got != "result-id+" {
			t.Fatalf("query recipient id was not decoded correctly for %s: %q", target, got)
		}
	}
}

func TestSubmittedFieldNamesHonorsCaptureGateAndDropsReservedName(t *testing.T) {
	form := url.Values{
		"RID":   {"body-recipient"},
		"email": {"alice@example.test"},
	}
	if got := submittedFieldNames(form, false); got != nil {
		t.Fatalf("capture-disabled submission produced metadata: %#v", got)
	}
	got := submittedFieldNames(form, true)
	if _, exists := got["other"]; exists {
		t.Fatalf("reserved recipient field was classified as submitted data: %#v", got)
	}
	if _, exists := got["email"]; !exists {
		t.Fatalf("ordinary field category was lost: %#v", got)
	}
}
