package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/darkarmy-cyber/darkphish/auth"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/sessions"
)

// RequestID gives every administrative request an application-generated
// correlation ID and returns it to the caller.
func RequestID(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := audit.NewRequestID()
		w.Header().Set("X-Request-ID", id)
		handler.ServeHTTP(w, audit.WithRequestID(r, id))
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

// AuditAPI records security-relevant API mutations and credential-result
// access after authentication and authorization have run.
func AuditAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := apiAuditAction(r.Method, r.URL.Path)
		if action == "" {
			next.ServeHTTP(w, r)
			return
		}
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		result := "success"
		if status >= http.StatusBadRequest {
			result = "failure"
		}
		actor := "anonymous"
		var actorID int64
		if value := ctx.Get(r, "user"); value != nil {
			user := value.(models.User)
			actor = user.Username
			actorID = user.Id
		}
		authMethod, _ := ctx.Get(r, "auth_method").(string)
		audit.Record(r, actor, actorID, action, r.URL.Path, result, authMethod)
	})
}

func apiAuditAction(method, path string) string {
	clean := strings.TrimSuffix(path, "/")
	switch {
	case method == http.MethodPost && clean == "/api/reset":
		return "api_token.rotate"
	case method == http.MethodGet && strings.HasSuffix(clean, "/results") && strings.HasPrefix(clean, "/api/campaigns/"):
		return "credential_data.access"
	case method == http.MethodPost && clean == "/api/campaigns":
		return "campaign.create_and_launch"
	case method == http.MethodPost && strings.HasSuffix(clean, "/complete"):
		return "campaign.complete"
	case method == http.MethodDelete && strings.HasPrefix(clean, "/api/campaigns/"):
		return "campaign.delete"
	case strings.HasPrefix(clean, "/api/users") && method == http.MethodPost:
		return "user.create"
	case strings.HasPrefix(clean, "/api/users/") && method == http.MethodDelete:
		return "user.delete"
	case strings.HasPrefix(clean, "/api/users/") && method == http.MethodPut:
		return "user.update"
	case strings.HasPrefix(clean, "/api/smtp") && method != http.MethodGet && method != http.MethodHead:
		return "smtp.modify"
	case strings.HasPrefix(clean, "/api/imap") && method != http.MethodGet && method != http.MethodHead:
		return "imap.modify"
	case strings.HasPrefix(clean, "/api/webhooks") && method != http.MethodGet && method != http.MethodHead:
		return "webhook.modify"
	case method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions:
		return "api.modify"
	default:
		return ""
	}
}

// Use allows us to stack middleware to process the request
// Example taken from https://github.com/gorilla/mux/pull/36#issuecomment-25849172
func Use(handler http.HandlerFunc, mid ...func(http.Handler) http.HandlerFunc) http.HandlerFunc {
	for _, m := range mid {
		handler = m(handler)
	}
	return handler
}

// GetContext wraps each request in a function which fills in the context for a given request.
// This includes setting the User and Session keys and values as necessary for use in later functions.
func GetContext(handler http.Handler) http.HandlerFunc {
	// Set the context here
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the request form
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing request", http.StatusInternalServerError)
		}
		// Set the context appropriately here.
		// Set the session
		session, err := Store.Get(r, CookieName)
		if err != nil {
			session, _ = Store.New(r, CookieName)
		}
		// Put the session in the context so that we can
		// reuse the values in different handlers
		r = ctx.Set(r, "session", session)
		if id, ok := session.Values["id"].(int64); ok {
			u, err := models.GetUser(id)
			if err != nil {
				r = ctx.Set(r, "user", nil)
			} else {
				r = ctx.Set(r, "user", u)
			}
		} else {
			r = ctx.Set(r, "user", nil)
		}
		handler.ServeHTTP(w, r)
		// Remove context contents
		ctx.Clear(r)
	}
}

// RequireAPIKey authenticates external callers with a bearer token and browser
// callers with their secure session. Query-string API keys are intentionally
// rejected because URLs are routinely persisted in logs and browser history.
func RequireAPIKey(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "" {
			JSONError(w, http.StatusUnauthorized, "API keys in URLs are disabled; use Authorization: Bearer")
			return
		}

		var u models.User
		authorization := r.Header.Get("Authorization")
		if authorization != "" {
			token, ok := bearerToken(authorization)
			if !ok {
				JSONError(w, http.StatusUnauthorized, "Invalid Authorization header")
				return
			}
			var err error
			u, err = models.GetUserByAPIKey(token)
			if err != nil {
				JSONError(w, http.StatusUnauthorized, "Invalid API token")
				return
			}
			r = ctx.Set(r, "auth_method", "bearer")
		} else {
			current := ctx.Get(r, "user")
			if current == nil {
				JSONError(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			u = current.(models.User)
			r = ctx.Set(r, "auth_method", "session")
		}
		if err := auth.CheckAccountState(u.AccountLocked, u.PasswordChangeRequired, false); err != nil {
			JSONError(w, http.StatusForbidden, err.Error())
			return
		}
		r = ctx.Set(r, "user", u)
		r = ctx.Set(r, "user_id", u.Id)
		handler.ServeHTTP(w, r)
	})
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// RequireLogin checks to see if the user is currently logged in.
// If not, the function returns a 302 redirect to the login page.
func RequireLogin(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u := ctx.Get(r, "user"); u != nil {
			currentUser := u.(models.User)
			allowPasswordChange := r.URL.Path == "/reset_password"
			err := auth.CheckAccountState(currentUser.AccountLocked, currentUser.PasswordChangeRequired, allowPasswordChange)
			if err == auth.ErrAccountLocked {
				if session, ok := ctx.Get(r, "session").(*sessions.Session); ok {
					session.Options.MaxAge = -1
					delete(session.Values, "id")
					_ = session.Save(r, w)
				}
				http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
				return
			}
			if err == auth.ErrPasswordChangeRequired {
				q := r.URL.Query()
				q.Set("next", r.URL.Path)
				http.Redirect(w, r, fmt.Sprintf("/reset_password?%s", q.Encode()), http.StatusTemporaryRedirect)
				return
			}
			handler.ServeHTTP(w, r)
			return
		}
		q := r.URL.Query()
		q.Set("next", r.URL.Path)
		http.Redirect(w, r, fmt.Sprintf("/login?%s", q.Encode()), http.StatusTemporaryRedirect)
	}
}

// CORS enables cross-origin administrative API access only for exact,
// explicitly configured origins. It never emits a wildcard or credentials.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" && origin != "*" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				if r.Method == http.MethodOptions {
					JSONError(w, http.StatusForbidden, "CORS origin is not allowed")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LimitRequestBody caps request bodies before handlers decode forms or JSON.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.HandlerFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		}
	}
}

// EnforceViewOnly is a global middleware that limits the ability to edit
// objects to accounts with the PermissionModifyObjects permission.
func EnforceViewOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the request is for any non-GET HTTP method, e.g. POST, PUT,
		// or DELETE, we need to ensure the user has the appropriate
		// permission.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			user := ctx.Get(r, "user").(models.User)
			access, err := user.HasPermission(models.PermissionModifyObjects)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if !access {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission checks to see if the user has the requested permission
// before executing the handler. If the request is unauthorized, a JSONError
// is returned.
func RequirePermission(perm string) func(http.Handler) http.HandlerFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			user := ctx.Get(r, "user").(models.User)
			access, err := user.HasPermission(perm)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !access {
				JSONError(w, http.StatusForbidden, http.StatusText(http.StatusForbidden))
				return
			}
			next.ServeHTTP(w, r)
		}
	}
}

// ApplySecurityHeaders applies various security headers according to best-
// practices.
func ApplySecurityHeaders(next http.Handler) http.HandlerFunc {
	return ApplyAdminSecurityHeaders(false)(next)
}

// ApplyAdminSecurityHeaders applies the administrative UI policy. Simulation
// landing pages intentionally use a separate trust boundary and do not receive
// this CSP.
func ApplyAdminSecurityHeaders(tlsEnabled bool) func(http.Handler) http.HandlerFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			csp := "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'"
			w.Header().Set("Content-Security-Policy", csp)
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
			if tlsEnabled {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		}
	}
}

// JSONError returns an error in JSON format with the given
// status code and message
func JSONError(w http.ResponseWriter, c int, m string) {
	cj, _ := json.MarshalIndent(models.Response{Success: false, Message: m}, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c)
	fmt.Fprintf(w, "%s", cj)
}
