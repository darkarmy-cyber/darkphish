// Package audit writes security-relevant administrative events as JSON lines.
package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

type requestIDKey struct{}

var writeMu sync.Mutex

// Event is the stable, secret-free audit event envelope.
type Event struct {
	Timestamp  time.Time `json:"timestamp"`
	Actor      string    `json:"actor"`
	ActorID    int64     `json:"actor_id,omitempty"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Result     string    `json:"result"`
	RequestID  string    `json:"request_id"`
	SourceIP   string    `json:"source_ip,omitempty"`
	AuthMethod string    `json:"auth_method,omitempty"`
}

// NewRequestID returns a cryptographically random request correlation ID.
func NewRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

// WithRequestID attaches an application-generated request ID to a request.
func WithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
}

// RequestID gets the application-generated request ID.
func RequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

// Record writes a single JSON object. Callers must pass identifiers and
// metadata only; request bodies, credentials, tokens, and secrets are never
// accepted by this API.
func Record(r *http.Request, actor string, actorID int64, action, target, result, authMethod string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	e := Event{
		Timestamp:  time.Now().UTC(),
		Actor:      actor,
		ActorID:    actorID,
		Action:     action,
		Target:     target,
		Result:     result,
		RequestID:  RequestID(r),
		SourceIP:   host,
		AuthMethod: authMethod,
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		log.Errorf("encode audit event: %v", err)
		return
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	_, _ = log.Logger.Out.Write(append(encoded, '\n'))
}
