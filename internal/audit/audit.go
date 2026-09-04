// Package audit records secret-free security events to both structured logs
// and an append-only persistent store.
package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

type requestIDKey struct{}

var (
	writeMu sync.Mutex
	storeMu sync.RWMutex
	store   Store
)

type Event struct {
	ID         int64     `json:"id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Actor      string    `json:"actor"`
	ActorID    int64     `json:"actor_id,omitempty"`
	ActorType  string    `json:"actor_type"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Result     string    `json:"result"`
	RequestID  string    `json:"request_id"`
	SourceIP   string    `json:"source_ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	AuthMethod string    `json:"auth_method,omitempty"`
	Metadata   string    `json:"metadata"`
}

type Filter struct {
	Action     string
	Actor      string
	ActorID    int64
	TargetType string
	TargetID   string
	Result     string
	From       *time.Time
	To         *time.Time
	Page       int
	PerPage    int
}

type Store interface {
	Append(Event) (int64, error)
	Query(Filter) ([]Event, int64, error)
	DeleteBefore(time.Time) (int64, error)
}

func SetStore(value Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = value
}

func NewRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

func WithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
}

func RequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func targetParts(target string) (string, string) {
	trimmed := strings.Trim(target, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 3 && parts[0] == "api" {
		return parts[1], strings.Join(parts[2:], "/")
	}
	if len(parts) >= 2 {
		return parts[0], strings.Join(parts[1:], "/")
	}
	return "system", target
}

func bounded(value string, maximum int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func persist(event Event) {
	encoded, err := json.Marshal(event)
	if err == nil {
		writeMu.Lock()
		_, _ = log.Logger.Out.Write(append(encoded, '\n'))
		writeMu.Unlock()
	}
	storeMu.RLock()
	current := store
	storeMu.RUnlock()
	if current != nil {
		if _, err := current.Append(event); err != nil {
			log.Errorf("persist audit event: %v", err)
		}
	}
}

// Record persists identifiers and bounded request metadata only. It has no API
// for request bodies, credentials, tokens, or secrets.
func Record(r *http.Request, actor string, actorID int64, action, target, result, authMethod string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	targetType, targetID := targetParts(target)
	actorType := "user"
	if actorID == 0 {
		actorType = "anonymous"
	}
	persist(Event{
		Timestamp: time.Now().UTC(), Actor: bounded(actor, 255), ActorID: actorID, ActorType: actorType,
		Action: bounded(action, 128), TargetType: bounded(targetType, 64), TargetID: bounded(targetID, 255), Result: bounded(result, 32),
		RequestID: bounded(RequestID(r), 64), SourceIP: bounded(host, 64), UserAgent: bounded(r.UserAgent(), 512),
		AuthMethod: bounded(authMethod, 32), Metadata: "{}",
	})
}

func RecordSystem(action, targetType, targetID, result string) {
	persist(Event{
		Timestamp: time.Now().UTC(), Actor: "darkphish", ActorType: "system",
		Action: bounded(action, 128), TargetType: bounded(targetType, 64), TargetID: bounded(targetID, 255), Result: bounded(result, 32),
		RequestID: NewRequestID(), AuthMethod: "system", Metadata: "{}",
	})
}

func Query(filter Filter) ([]Event, int64, error) {
	storeMu.RLock()
	current := store
	storeMu.RUnlock()
	if current == nil {
		return []Event{}, 0, nil
	}
	return current.Query(filter)
}

func DeleteBefore(before time.Time) (int64, error) {
	storeMu.RLock()
	current := store
	storeMu.RUnlock()
	if current == nil {
		return 0, nil
	}
	return current.DeleteBefore(before)
}
