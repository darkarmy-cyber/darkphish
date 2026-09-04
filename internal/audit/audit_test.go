package audit

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

func TestRecordWritesSecretFreeStructuredEvent(t *testing.T) {
	var output bytes.Buffer
	previous := log.Logger.Out
	log.Logger.SetOutput(&output)
	t.Cleanup(func() { log.Logger.SetOutput(previous) })

	r := httptest.NewRequest("POST", "/api/reset", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r = WithRequestID(r, "request-123")
	Record(r, "admin", 1, "api_token.rotate", "user:1", "success", "session")

	var event Event
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("audit output is not JSON: %v", err)
	}
	if event.RequestID != "request-123" || event.Action != "api_token.rotate" || event.SourceIP != "192.0.2.10" {
		t.Fatalf("unexpected event: %#v", event)
	}
	for _, forbidden := range []string{"password", "token", "secret"} {
		if strings.Contains(strings.ToLower(output.String()), `"`+forbidden+`":`) {
			t.Fatalf("audit event contains forbidden field %q: %s", forbidden, output.String())
		}
	}
}
