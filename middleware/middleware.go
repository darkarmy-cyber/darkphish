package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	case method == http.MethodGet && clean == "/api/audit/export":
		return "data.export"
	case method == http.MethodPost && strings.HasSuffix(clean, "/credential/reveal"):
		return "credential.view"
	case clean == "/api/reauthenticate" || strings.Contains(clean, "/reviewers"):
		return ""
	case clean == "/api/pats" || strings.HasPrefix(clean, "/api/pats/"):
		return ""
	case method == http.MethodGet && strings.HasSuffix(clean, "/results") && strings.HasPrefix(clean, "/api/campaigns/"):
		return "campaign.results.view"
	case method == http.MethodPost && clean == "/api/campaigns":
		return "campaign.create"
	case method == http.MethodPost && strings.HasSuffix(clean, "/complete"):
		return "campaign.complete"
	case method == http.MethodDelete && strings.HasPrefix(clean, "/api/campaigns/"):
		return "campaign.delete"
	case clean == "/api/users" && method == http.MethodPost:
		return ""
	case strings.HasPrefix(clean, "/api/users/") && method == http.MethodDelete:
		return "user.delete"
	case strings.HasPrefix(clean, "/api/users/") && method == http.MethodPut:
		return ""
	case strings.HasPrefix(clean, "/api/smtp") && method != http.MethodGet && method != http.MethodHead:
		return "smtp.update"
	case strings.HasPrefix(clean, "/api/imap") && method != http.MethodGet && method != http.MethodHead:
		return "imap.update"
	case strings.HasPrefix(clean, "/api/webhooks") && method != http.MethodGet && method != http.MethodHead:
		return "webhook.update"
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

// RequireAPIAuthentication authenticates external callers with a PAT and browser
// callers with their secure session. Query-string API keys are intentionally
// rejected because URLs are routinely persisted in logs and browser history.
func RequireAPIAuthentication(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "" {
			audit.Record(r, "anonymous", 0, "pat.auth.failure", "api", "failure", "pat")
			JSONError(w, http.StatusUnauthorized, "API keys in URLs are disabled; use Authorization: Bearer")
			return
		}

		var u models.User
		authorization := r.Header.Get("Authorization")
		if authorization != "" {
			token, ok := bearerToken(authorization)
			if !ok {
				audit.Record(r, "anonymous", 0, "pat.auth.failure", "api", "failure", "pat")
				JSONError(w, http.StatusUnauthorized, "Invalid Authorization header")
				return
			}
			authentication, err := models.AuthenticatePersonalAccessToken(token)
			if err != nil {
				audit.Record(r, "anonymous", 0, "pat.auth.failure", "api", "failure", "pat")
				JSONError(w, http.StatusUnauthorized, "Invalid API token")
				return
			}
			u = authentication.User
			r = ctx.Set(r, "auth_method", "pat")
			r = ctx.Set(r, "pat_id", authentication.Token.ID)
			r = ctx.Set(r, "pat_scopes", authentication.Scopes)
		} else {
			current := ctx.Get(r, "user")
			if current == nil {
				audit.Record(r, "anonymous", 0, "auth.session.failure", "api", "failure", "session")
				JSONError(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			u = current.(models.User)
			r = ctx.Set(r, "auth_method", "session")
		}
		if err := auth.CheckAccountState(u.AccountLocked, u.PasswordChangeRequired, false); err != nil {
			if authorization != "" {
				audit.Record(r, u.Username, u.Id, "pat.auth.failure", "api", "failure", "pat")
			}
			JSONError(w, http.StatusForbidden, err.Error())
			return
		}
		r = ctx.Set(r, "user", u)
		r = ctx.Set(r, "user_id", u.Id)
		if authorization != "" {
			audit.Record(r, u.Username, u.Id, "pat.auth.success", "api", "success", "pat")
		}
		handler.ServeHTTP(w, r)
	})
}

// EnforcePATScopes applies least-privilege scopes in addition to normal user
// role permissions. Browser sessions are unaffected.
func EnforcePATScopes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if method, _ := ctx.Get(r, "auth_method").(string); method != "pat" {
			next.ServeHTTP(w, r)
			return
		}
		required := requiredPATScope(r.Method, r.URL.Path)
		scopes, _ := ctx.Get(r, "pat_scopes").(map[string]struct{})
		if required == "" {
			recordPATAuthorizationFailure(r)
			JSONError(w, http.StatusForbidden, "Personal access tokens cannot access this route")
			return
		}
		if _, ok := scopes[required]; !ok {
			recordPATAuthorizationFailure(r)
			JSONError(w, http.StatusForbidden, "Personal access token lacks required scope: "+required)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recordPATAuthorizationFailure(r *http.Request) {
	user, _ := ctx.Get(r, "user").(models.User)
	audit.Record(r, user.Username, user.Id, "pat.authorization.failure", r.URL.Path, "failure", "pat")
}

func requiredPATScope(method, path string) string {
	clean := strings.TrimSuffix(path, "/")
	write := method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	switch {
	case strings.HasPrefix(clean, "/api/pats"):
		return "tokens:manage"
	case strings.HasPrefix(clean, "/api/audit"):
		return "audit:read"
	case strings.HasSuffix(clean, "/credential/reveal"):
		return "credentials:view"
	case strings.HasPrefix(clean, "/api/campaigns"):
		if !write && (strings.HasSuffix(clean, "/results") || strings.HasSuffix(clean, "/summary")) {
			return "reports:read"
		}
		if write {
			return "campaigns:write"
		}
		return "campaigns:read"
	case strings.HasPrefix(clean, "/api/groups"):
		if write {
			return "groups:write"
		}
		return "groups:read"
	case strings.HasPrefix(clean, "/api/templates"):
		if write {
			return "templates:write"
		}
		return "templates:read"
	case strings.HasPrefix(clean, "/api/pages"):
		if write {
			return "landing-pages:write"
		}
		return "landing-pages:read"
	case strings.HasPrefix(clean, "/api/smtp") || strings.HasPrefix(clean, "/api/util/send_test_email"):
		if write {
			return "sending-profiles:write"
		}
		return "sending-profiles:read"
	case strings.HasPrefix(clean, "/api/users"):
		if write {
			return "users:write"
		}
		return "users:read"
	case strings.HasPrefix(clean, "/api/imap") || strings.HasPrefix(clean, "/api/webhooks"):
		if write {
			return "integrations:write"
		}
		return "integrations:read"
	case clean == "/api/import/group":
		return "groups:write"
	case clean == "/api/import/email":
		return "templates:write"
	case clean == "/api/import/site":
		return "landing-pages:write"
	case strings.HasPrefix(clean, "/api/reset"):
		return ""
	default:
		return ""
	}
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
			clean := strings.TrimSuffix(r.URL.Path, "/")
			if clean == "/api/reauthenticate" || strings.HasSuffix(clean, "/credential/reveal") || strings.Contains(clean, "/reviewers") {
				next.ServeHTTP(w, r)
				return
			}
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

// RequireCampaignCredentialReview centralizes the role and assignment check,
// plus the browser-only privileged-session boundary. PAT callers remain
// subject to both credentials:view scope and the user's RBAC permissions.
func RequireCampaignCredentialReview(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ctx.Get(r, "user").(models.User)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			JSONError(w, http.StatusBadRequest, "Invalid campaign")
			return
		}
		campaignID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid campaign")
			return
		}
		allowed, err := models.CanReviewCredential(user, campaignID, time.Now().UTC())
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Unable to authorize credential review")
			return
		}
		if !allowed {
			JSONError(w, http.StatusForbidden, "Credential review is not authorized for this campaign")
			return
		}
		if method, _ := ctx.Get(r, "auth_method").(string); method == "session" {
			session, _ := ctx.Get(r, "session").(*sessions.Session)
			binding, _ := session.Values["session_id"].(string)
			fresh, freshErr := models.IsPrivilegedSessionFresh(user.Id, binding, time.Now().UTC())
			if freshErr != nil {
				JSONError(w, http.StatusInternalServerError, "Unable to validate privileged session")
				return
			}
			if !fresh {
				JSONError(w, http.StatusPreconditionRequired, "Fresh privileged reauthentication is required")
				return
			}
		}
		next.ServeHTTP(w, r)
	}
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
