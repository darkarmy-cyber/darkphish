package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectoryConnectorRejectsUnknownJSONFields(t *testing.T) {
	var connector DirectoryConnector
	err := json.Unmarshal([]byte(`{
		"name":"Entra",
		"provider":"entra",
		"tenant_id":"Tenant.Example",
		"client_id":"11111111-2222-3333-4444-555555555555",
		"remote_group_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"email_domain":["example.com"]
	}`), &connector)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
}

func TestDirectoryConnectorValidateCanonicalizesTenantID(t *testing.T) {
	connector := DirectoryConnector{
		Name:          "Entra",
		Provider:      "entra",
		TenantID:      " Tenant.Example ",
		ClientID:      "11111111-2222-3333-4444-555555555555",
		RemoteGroupID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	}
	if err := connector.Validate(); err != nil {
		t.Fatal(err)
	}
	if connector.TenantID != "tenant.example" {
		t.Fatalf("tenant id was not canonicalized: %q", connector.TenantID)
	}
}
