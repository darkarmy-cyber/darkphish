package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIntegrationSecretsAreWriteOnlyJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		setField string
	}{
		{name: "smtp", value: SMTP{Password: "smtp-secret", PasswordSet: true}, setField: `"password_set":true`},
		{name: "imap", value: IMAP{Password: "imap-secret", PasswordSet: true}, setField: `"password_set":true`},
		{name: "webhook", value: Webhook{Secret: "webhook-secret", SecretSet: true}, setField: `"secret_set":true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			text := string(encoded)
			if strings.Contains(text, "-secret") {
				t.Fatalf("serialized secret: %s", text)
			}
			if !strings.Contains(text, tt.setField) {
				t.Fatalf("missing secret metadata in %s", text)
			}
		})
	}
}

func TestIntegrationSecretsCanBeSubmitted(t *testing.T) {
	var smtp SMTP
	if err := json.Unmarshal([]byte(`{"name":"smtp","password":"write-only"}`), &smtp); err != nil {
		t.Fatal(err)
	}
	if smtp.Password != "write-only" || !smtp.PasswordSet {
		t.Fatalf("SMTP password was not accepted as write-only input: %#v", smtp)
	}

	var webhook Webhook
	if err := json.Unmarshal([]byte(`{"name":"hook","secret":"write-only"}`), &webhook); err != nil {
		t.Fatal(err)
	}
	if webhook.Secret != "write-only" || !webhook.SecretSet {
		t.Fatalf("webhook secret was not accepted as write-only input: %#v", webhook)
	}
}
