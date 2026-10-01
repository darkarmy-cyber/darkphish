package ratelimit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var successHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
})

func reachLimit(t *testing.T, handler http.Handler, limit int) {
	// Make `expected` requests and ensure that each return a successful
	// response.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "127.0.0.1:"
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("no 200 on req %d got %d", i, w.Code)
		}
	}
	// Then, makes another request to ensure it returns the 429
	// status.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("no 429")
	}
}

func TestConcurrentFirstRequestsShareCapacity(t *testing.T) {
	limiter := NewPostLimiter(WithRequestsPerMinute(5))
	start := make(chan struct{})
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if limiter.Check("same-client").Allowed {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := accepted.Load(); got != 5 {
		t.Fatalf("accepted %d concurrent requests, want 5", got)
	}
}

func TestAllMethodsShareBudgetAndExposeRetry(t *testing.T) {
	limiter := NewPostLimiter(WithRequestsPerMinute(3))
	handler := limiter.LimitAll(successHandler)
	for i, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPost, http.MethodHead} {
		r := httptest.NewRequest(method, "/", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := http.StatusOK
		remaining := 2 - i
		if i >= 3 {
			want = http.StatusTooManyRequests
			remaining = 0
		}
		if w.Code != want || w.Header().Get("X-RateLimit-Limit") != "3" || w.Header().Get("X-RateLimit-Remaining") != strconv.Itoa(remaining) {
			t.Fatalf("request %d: status=%d headers=%v", i, w.Code, w.Header())
		}
		if i >= 3 && w.Header().Get("Retry-After") != "20" {
			t.Fatalf("retry=%q want 20", w.Header().Get("Retry-After"))
		}
		if i >= 3 {
			var refusal struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			if json.Unmarshal(w.Body.Bytes(), &refusal) != nil || refusal.Success || refusal.Message == "" {
				t.Fatal("API refusal is not an actionable JSON error")
			}
		}
		if i < 3 && w.Header().Get("Retry-After") != "" {
			t.Fatal("successful request has retry header")
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.2:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal("another client shares the exhausted budget")
	}
}

func TestRejectedChecksDoNotReserveFutureCapacity(t *testing.T) {
	limiter := NewPostLimiter(WithRequestsPerMinute(5))
	for i := 0; i < 5; i++ {
		if !limiter.Check("client").Allowed {
			t.Fatal("initial request denied")
		}
	}
	for i := 0; i < 50; i++ {
		if limiter.Check("client").Allowed {
			t.Fatal("exhausted request allowed")
		}
	}
	limiter.Lock()
	b := limiter.visitors["client"]
	// Inspect future capacity without sleeping: refusals must not create debt.
	if !b.limiter.AllowN(time.Now().Add(13*time.Second), 1) {
		t.Fatal("denied requests postponed refill")
	}
	limiter.Unlock()
}

func TestRateLimitEnforcement(t *testing.T) {
	expectedLimit := 3
	limiter := NewPostLimiter(WithRequestsPerMinute(expectedLimit))
	handler := limiter.Limit(successHandler)
	reachLimit(t, handler, expectedLimit)
}

func TestRateLimitCleanup(t *testing.T) {
	expectedLimit := 3
	limiter := NewPostLimiter(WithRequestsPerMinute(expectedLimit))
	handler := limiter.Limit(successHandler)
	reachLimit(t, handler, expectedLimit)

	// Set the timeout to be
	bucket, exists := limiter.visitors["127.0.0.1"]
	if !exists {
		t.Fatalf("doesn't exist for some reason")
	}
	bucket.lastSeen = bucket.lastSeen.Add(-limiter.expiry)
	limiter.Cleanup()
	_, exists = limiter.visitors["127.0.0.1"]
	if exists {
		t.Fatalf("exists for some reason")
	}
	reachLimit(t, handler, expectedLimit)
}
