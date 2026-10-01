package ratelimit

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// DefaultRequestsPerMinute is the number of requests to allow per minute.
// Any requests over this interval will return a HTTP 429 error.
const DefaultRequestsPerMinute = 5

// DefaultCleanupInterval determines how frequently the cleanup routine
// executes.
const DefaultCleanupInterval = 1 * time.Minute

// DefaultExpiry is the amount of time to track a bucket for a particular
// visitor.
const DefaultExpiry = 10 * time.Minute

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// PostLimiter is a simple rate limiting middleware which only allows n POST
// requests per minute.
type PostLimiter struct {
	visitors        map[string]*bucket
	requestLimit    int
	cleanupInterval time.Duration
	expiry          time.Duration
	sync.RWMutex
}

// PostLimiterOption is a functional option that allows callers to configure
// the rate limiter.
type PostLimiterOption func(*PostLimiter)

// WithRequestsPerMinute sets the number of requests to allow per minute.
func WithRequestsPerMinute(requestLimit int) PostLimiterOption {
	return func(p *PostLimiter) {
		p.requestLimit = requestLimit
	}
}

// WithCleanupInterval sets the interval between cleaning up stale entries in
// the rate limit client list
func WithCleanupInterval(interval time.Duration) PostLimiterOption {
	return func(p *PostLimiter) {
		p.cleanupInterval = interval
	}
}

// WithExpiry sets the amount of time to store client entries before they are
// considered stale.
func WithExpiry(expiry time.Duration) PostLimiterOption {
	return func(p *PostLimiter) {
		p.expiry = expiry
	}
}

// NewPostLimiter returns a new instance of a PostLimiter
func NewPostLimiter(opts ...PostLimiterOption) *PostLimiter {
	limiter := &PostLimiter{
		visitors:        make(map[string]*bucket),
		requestLimit:    DefaultRequestsPerMinute,
		cleanupInterval: DefaultCleanupInterval,
		expiry:          DefaultExpiry,
	}
	for _, opt := range opts {
		opt(limiter)
	}
	go limiter.pollCleanup()
	return limiter
}

func (limiter *PostLimiter) pollCleanup() {
	ticker := time.NewTicker(limiter.cleanupInterval)
	for range ticker.C {
		limiter.Cleanup()
	}
}

// Cleanup removes any buckets that were last seen past the configured expiry.
func (limiter *PostLimiter) Cleanup() {
	limiter.Lock()
	defer limiter.Unlock()
	for ip, bucket := range limiter.visitors {
		if time.Since(bucket.lastSeen) >= limiter.expiry {
			delete(limiter.visitors, ip)
		}
	}
}

// Decision describes the bucket immediately after consuming one request.
type Decision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter int
}

// Check atomically finds or creates a bucket and consumes its capacity.
func (limiter *PostLimiter) Check(key string) Decision {
	limiter.Lock()
	defer limiter.Unlock()
	now := time.Now()
	b, exists := limiter.visitors[key]
	if !exists {
		b = &bucket{limiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(limiter.requestLimit)), limiter.requestLimit)}
		limiter.visitors[key] = b
	}
	b.lastSeen = now
	allowed := b.limiter.AllowN(now, 1)
	tokens := b.limiter.TokensAt(now)
	decision := Decision{Allowed: allowed, Limit: limiter.requestLimit, Remaining: int(math.Floor(math.Max(0, tokens)))}
	if !allowed {
		decision.RetryAfter = int(math.Max(1, math.Ceil((1-tokens)/float64(b.limiter.Limit()))))
	}
	return decision
}

// SetHeaders exposes capacity without reserving future requests.
func (decision Decision) SetHeaders(w http.ResponseWriter) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
	if !decision.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfter))
	}
}

// AllowKey checks a caller-defined identity bucket. Sensitive authenticated
// endpoints use a user-and-source key so one client cannot globally lock out
// every reviewer.
func (limiter *PostLimiter) AllowKey(key string) bool { return limiter.Check(key).Allowed }

// Limit enforces the configured rate limit for POST requests.
//
// TODO: Change the return value to an http.Handler when we clean up the
// way DarkPhish routing is done.
func (limiter *PostLimiter) Limit(next http.Handler) http.HandlerFunc {
	return limiter.limit(next, true)
}

// LimitAll bounds every method, including failed authentication attempts.
func (limiter *PostLimiter) LimitAll(next http.Handler) http.HandlerFunc {
	return limiter.limit(next, false)
}

func (limiter *PostLimiter) limit(next http.Handler, postOnly bool) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			clientIP = r.RemoteAddr
		}
		if !postOnly || r.Method == http.MethodPost {
			decision := limiter.Check(clientIP)
			decision.SetHeaders(w)
			if !decision.Allowed {
				if postOnly {
					http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					json.NewEncoder(w).Encode(struct {
						Success bool   `json:"success"`
						Message string `json:"message"`
					}{Message: "Request limit exceeded; try again later"})
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
