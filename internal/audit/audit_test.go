package audit

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

func TestRecordWritesSecretFreeStructuredEvent(t *testing.T) {
	var output bytes.Buffer
	previous := log.Logger.Out
	log.Logger.SetOutput(&output)
	t.Cleanup(func() { log.Logger.SetOutput(previous) })

	r := httptest.NewRequest("DELETE", "/api/pats/1", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r = WithRequestID(r, "request-123")
	Record(r, "admin", 1, "pat.revoke", "pats/1", "success", "session")

	var event Event
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("audit output is not JSON: %v", err)
	}
	if event.RequestID != "request-123" || event.Action != "pat.revoke" || event.SourceIP != "192.0.2.10" {
		t.Fatalf("unexpected event: %#v", event)
	}
	for _, forbidden := range []string{"password", "token", "secret"} {
		if strings.Contains(strings.ToLower(output.String()), `"`+forbidden+`":`) {
			t.Fatalf("audit event contains forbidden field %q: %s", forbidden, output.String())
		}
	}
}

func TestRecordBoundsUntrustedIdentifiers(t *testing.T) {
	var output bytes.Buffer
	previous := log.Logger.Out
	log.Logger.SetOutput(&output)
	t.Cleanup(func() { log.Logger.SetOutput(previous) })

	r := httptest.NewRequest("POST", "/login", nil)
	Record(r, strings.Repeat("é", 300), 0, strings.Repeat("a", 200), strings.Repeat("t", 400), "failure", "session")
	var event Event
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if len(event.Actor) > 255 || len(event.Action) > 128 || len(event.TargetID) > 255 || !utf8.ValidString(event.Actor) {
		t.Fatalf("audit identifiers were not safely bounded: %#v", event)
	}
}
