package controllers

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/controllers/api"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	log "github.com/darkarmy-cyber/darkphish/logger"
	mid "github.com/darkarmy-cyber/darkphish/middleware"
	"github.com/darkarmy-cyber/darkphish/middleware/ratelimit"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/darkarmy-cyber/darkphish/util"
	"github.com/darkarmy-cyber/darkphish/worker"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/jordan-wright/unindexed"
)

// AdminServerOption is a functional option that is used to configure the
// admin server
type AdminServerOption func(*AdminServer)

// AdminServer is an HTTP server that implements the administrative Darkphish
// handlers, including the dashboard and REST API.
type AdminServer struct {
	server  *http.Server
	worker  worker.Worker
	config  config.AdminServer
	limiter *ratelimit.PostLimiter
}

var defaultTLSConfig = &tls.Config{
	CurvePreferences: []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
	},
	MinVersion: tls.VersionTLS12,
	CipherSuites: []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

		// Kept for backwards compatibility with some clients
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	},
}

// WithWorker is an option that sets the background worker.
func WithWorker(w worker.Worker) AdminServerOption {
	return func(as *AdminServer) {
		as.worker = w
	}
}

// NewAdminServer returns a new instance of the AdminServer with the
// provided config and options applied.
func NewAdminServer(conf config.AdminServer, options ...AdminServerOption) *AdminServer {
	if conf.MaxRequestBodyBytes == 0 {
		conf.MaxRequestBodyBytes = config.DefaultMaxRequestBodyBytes
	}
	defaultWorker, _ := worker.New()
	defaultServer := &http.Server{
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		Addr:              conf.ListenURL,
	}
	defaultLimiter := ratelimit.NewPostLimiter()
	as := &AdminServer{
		worker:  defaultWorker,
		server:  defaultServer,
		limiter: defaultLimiter,
		config:  conf,
	}
	for _, opt := range options {
		opt(as)
	}
	as.registerRoutes()
	return as
}

// Start launches the admin server, listening on the configured address.
func (as *AdminServer) Start() {
	if as.worker != nil {
		go as.worker.Start()
	}
	if as.config.UseTLS {
		// Only support TLS 1.2 and above - ref #1691, #1689
		as.server.TLSConfig = defaultTLSConfig
		err := util.CheckAndCreateSSL(as.config.CertPath, as.config.KeyPath)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Starting admin server at https://%s", as.config.ListenURL)
		log.Fatal(as.server.ListenAndServeTLS(as.config.CertPath, as.config.KeyPath))
	}
	// If TLS isn't configured, just listen on HTTP
	log.Infof("Starting admin server at http://%s", as.config.ListenURL)
	log.Fatal(as.server.ListenAndServe())
}

// Shutdown attempts to gracefully shutdown the server.
func (as *AdminServer) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	return as.server.Shutdown(ctx)
}

// SetupAdminRoutes creates the routes for handling requests to the web interface.
// This function returns an http.Handler to be used in http.ListenAndServe().
func (as *AdminServer) registerRoutes() {
	router := mux.NewRouter()
	router.HandleFunc("/healthz", as.Health).Methods(http.MethodGet, http.MethodHead)
	router.HandleFunc("/readyz", as.Ready).Methods(http.MethodGet, http.MethodHead)
	// Base Front-end routes
	router.HandleFunc("/", mid.Use(as.Base, mid.RequireLogin))
	router.HandleFunc("/login", mid.Use(as.Login, as.limiter.Limit))
	router.HandleFunc("/logout", mid.Use(as.Logout, mid.RequireLogin))
	router.HandleFunc("/reset_password", mid.Use(as.ResetPassword, mid.RequireLogin))
	router.HandleFunc("/campaigns", mid.Use(as.Campaigns, mid.RequireLogin))
	router.HandleFunc("/campaigns/{id:[0-9]+}", mid.Use(as.CampaignID, mid.RequireLogin))
	router.HandleFunc("/templates", mid.Use(as.Templates, mid.RequireLogin))
	router.HandleFunc("/groups", mid.Use(as.Groups, mid.RequireLogin))
	router.HandleFunc("/landing_pages", mid.Use(as.LandingPages, mid.RequireLogin))
	router.HandleFunc("/sending_profiles", mid.Use(as.SendingProfiles, mid.RequireLogin))
	router.HandleFunc("/settings", mid.Use(as.Settings, mid.RequireLogin))
	router.HandleFunc("/users", mid.Use(as.UserManagement, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	router.HandleFunc("/webhooks", mid.Use(as.Webhooks, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	router.HandleFunc("/audit", mid.Use(as.Audit, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	router.HandleFunc("/impersonate", mid.Use(as.Impersonate, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	// Create the API routes
	api := api.NewServer(
		api.WithWorker(as.worker),
		api.WithLimiter(as.limiter),
		api.WithAllowedOrigins(as.config.CORSAllowedOrigins),
	)
	router.PathPrefix("/api/").Handler(api)

	// Setup static file serving
	router.PathPrefix("/").Handler(http.FileServer(unindexed.Dir("./static/")))

	// Apply Go's scheme-aware cross-origin protection to all session-backed
	// browser requests. Exact trusted origins are validated during config load.
	crossOrigin := http.NewCrossOriginProtection()
	for _, origin := range as.config.TrustedOrigins {
		if err := crossOrigin.AddTrustedOrigin(origin); err != nil {
			log.Errorf("Ignoring invalid trusted origin %q: %v", origin, err)
		}
	}
	crossOriginProtected := crossOrigin.Handler(router)
	var adminHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bearer-only API calls do not depend on ambient browser credentials.
		// Their Authorization header is validated by the API middleware.
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != "" {
			router.ServeHTTP(w, r)
			return
		}
		crossOriginProtected.ServeHTTP(w, r)
	})
	adminHandler = mid.Use(
		adminHandler.ServeHTTP,
		mid.LimitRequestBody(as.config.MaxRequestBodyBytes),
		mid.GetContext,
		mid.ApplyAdminSecurityHeaders(as.config.UseTLS),
		mid.RequestID,
	)

	// Setup GZIP compression
	gzipWrapper, _ := gziphandler.NewGzipLevelHandler(gzip.BestCompression)
	adminHandler = gzipWrapper(adminHandler)

	// Respect X-Forwarded-For and X-Real-IP headers in case we're behind a
	// reverse proxy.
	adminHandler = handlers.ProxyHeaders(adminHandler)

	// Setup logging
	adminHandler = handlers.CombinedLoggingHandler(log.Writer(), adminHandler)
	as.server.Handler = adminHandler
}

// Health reports that the HTTP process is alive.
func (as *AdminServer) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// Ready reports whether the database is currently reachable.
func (as *AdminServer) Ready(w http.ResponseWriter, _ *http.Request) {
	if err := models.Health(); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready\n"))
}

type templateParams struct {
	Title           string
	Flashes         []interface{}
	User            models.User
	Version         string
	ModifySystem    bool
	ViewCredentials bool
	ManageReviewers bool
	OperateCampaign bool
}

// newTemplateParams returns the default template parameters for a user.
func newTemplateParams(r *http.Request) templateParams {
	user := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)
	modifySystem, _ := user.HasPermission(models.PermissionModifySystem)
	viewCredentials, _ := user.HasPermission(models.PermissionViewCredentials)
	return templateParams{
		User:            user,
		ModifySystem:    modifySystem,
		ViewCredentials: viewCredentials,
		Version:         config.Version,
		Flashes:         session.Flashes(),
	}
}

// Base handles the default path and template execution
func (as *AdminServer) Base(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Dashboard"
	getTemplate(w, "dashboard").ExecuteTemplate(w, "base", params)
}

// Campaigns handles the default path and template execution
func (as *AdminServer) Campaigns(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Campaigns"
	getTemplate(w, "campaigns").ExecuteTemplate(w, "base", params)
}

// CampaignID handles the default path and template execution
func (as *AdminServer) CampaignID(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Campaign Results"
	campaignID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	params.ManageReviewers, _ = models.CanManageCampaignReviewers(campaignID, params.User)
	params.OperateCampaign = params.ManageReviewers
	getTemplate(w, "campaign_results").ExecuteTemplate(w, "base", params)
}

// Templates handles the default path and template execution
func (as *AdminServer) Templates(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Email Templates"
	getTemplate(w, "templates").ExecuteTemplate(w, "base", params)
}

// Groups handles the default path and template execution
func (as *AdminServer) Groups(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Users & Groups"
	getTemplate(w, "groups").ExecuteTemplate(w, "base", params)
}

// LandingPages handles the default path and template execution
func (as *AdminServer) LandingPages(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Landing Pages"
	getTemplate(w, "landing_pages").ExecuteTemplate(w, "base", params)
}

// SendingProfiles handles the default path and template execution
func (as *AdminServer) SendingProfiles(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Sending Profiles"
	getTemplate(w, "sending_profiles").ExecuteTemplate(w, "base", params)
}

// Settings handles the changing of settings
func (as *AdminServer) Settings(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		params := newTemplateParams(r)
		params.Title = "Settings"
		session := ctx.Get(r, "session").(*sessions.Session)
		session.Save(r, w)
		getTemplate(w, "settings").ExecuteTemplate(w, "base", params)
	case r.Method == "POST":
		u := ctx.Get(r, "user").(models.User)
		currentPw := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_new_password")
		// Check the current password
		err := auth.ValidatePassword(currentPw, u.Hash)
		msg := models.Response{Success: true, Message: "Settings Updated Successfully"}
		if err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusBadRequest)
			return
		}
		newHash, err := auth.ValidatePasswordChange(u.Hash, newPassword, confirmPassword)
		if err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusBadRequest)
			return
		}
		u.Hash = string(newHash)
		if err = models.PutUser(&u); err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusInternalServerError)
			return
		}
		_ = models.RevokeUserPrivilegedSessions(u.Id)
		recordBrowserAudit(r, u.Username, u.Id, "password.change", u.Username, "success")
		api.JSONResponse(w, msg, http.StatusOK)
	}
}

// UserManagement is an admin-only handler that allows for the registration
// and management of user accounts within Darkphish.
func (as *AdminServer) UserManagement(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "User Management"
	getTemplate(w, "users").ExecuteTemplate(w, "base", params)
}

func (as *AdminServer) nextOrIndex(w http.ResponseWriter, r *http.Request) {
	next := "/"
	url, err := url.Parse(r.FormValue("next"))
	if err == nil {
		path := url.EscapedPath()
		if path != "" {
			next = "/" + strings.TrimLeft(path, "/")
		}
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (as *AdminServer) handleInvalidLogin(w http.ResponseWriter, r *http.Request, message string) {
	session := ctx.Get(r, "session").(*sessions.Session)
	Flash(w, r, "danger", message)
	params := struct {
		User    models.User
		Title   string
		Flashes []interface{}
	}{Title: "Login"}
	params.Flashes = session.Flashes()
	session.Save(r, w)
	templates := template.New("template")
	_, err := templates.ParseFiles("templates/login.html", "templates/flashes.html")
	if err != nil {
		log.Error(err)
	}
	// w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	template.Must(templates, err).ExecuteTemplate(w, "base", params)
}

func recordBrowserAudit(r *http.Request, actor string, actorID int64, action, target, result string) {
	audit.Record(r, actor, actorID, action, target, result, "session")
}

// Webhooks is an admin-only handler that handles webhooks
func (as *AdminServer) Webhooks(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Webhooks"
	getTemplate(w, "webhooks").ExecuteTemplate(w, "base", params)
}

func (as *AdminServer) Audit(w http.ResponseWriter, r *http.Request) {
	params := newTemplateParams(r)
	params.Title = "Audit Log"
	getTemplate(w, "audit").ExecuteTemplate(w, "base", params)
}

// Impersonate allows an admin to login to a user account without needing the password
func (as *AdminServer) Impersonate(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		actor := ctx.Get(r, "user").(models.User)
		username := r.FormValue("username")
		u, err := models.GetUserByUsername(username)
		if err != nil {
			recordBrowserAudit(r, actor.Username, actor.Id, "impersonation.start", username, "failure")
			log.Error(err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		session := ctx.Get(r, "session").(*sessions.Session)
		session.Values = make(map[interface{}]interface{})
		session.Values["id"] = u.Id
		binding, bindingErr := models.NewSessionBinding()
		if bindingErr != nil {
			http.Error(w, "Unable to create session", http.StatusInternalServerError)
			return
		}
		session.Values["session_id"] = binding
		session.Values["impersonator_id"] = actor.Id
		session.Values["impersonator_username"] = actor.Username
		session.Save(r, w)
		recordBrowserAudit(r, actor.Username, actor.Id, "impersonation.start", u.Username, "success")
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// Login handles the authentication flow for a user. If credentials are valid,
// a session is created
func (as *AdminServer) Login(w http.ResponseWriter, r *http.Request) {
	params := struct {
		User    models.User
		Title   string
		Flashes []interface{}
	}{Title: "Login"}
	session := ctx.Get(r, "session").(*sessions.Session)
	switch {
	case r.Method == "GET":
		params.Flashes = session.Flashes()
		session.Save(r, w)
		templates := template.New("template")
		_, err := templates.ParseFiles("templates/login.html", "templates/flashes.html")
		if err != nil {
			log.Error(err)
		}
		template.Must(templates, err).ExecuteTemplate(w, "base", params)
	case r.Method == "POST":
		// Find the user with the provided username
		username, password := r.FormValue("username"), r.FormValue("password")
		u, err := models.GetUserByUsername(username)
		if err != nil {
			recordBrowserAudit(r, username, 0, "auth.login.failure", username, "failure")
			log.Error(err)
			as.handleInvalidLogin(w, r, "Invalid Username/Password")
			return
		}
		// Validate the user's password
		upgradedHash, validationErr := auth.ValidatePasswordWithUpgrade(password, u.Hash)
		err = validationErr
		if err != nil {
			recordBrowserAudit(r, username, u.Id, "auth.login.failure", username, "failure")
			log.Error(err)
			as.handleInvalidLogin(w, r, "Invalid Username/Password")
			return
		}
		if u.AccountLocked {
			recordBrowserAudit(r, username, u.Id, "auth.login.failure", username, "failure")
			as.handleInvalidLogin(w, r, "Account Locked")
			return
		}
		lastLogin := time.Now().UTC()
		u.LastLogin = &lastLogin
		if upgradedHash != "" {
			u.Hash = upgradedHash
		}
		err = models.PutUser(&u)
		if err != nil {
			log.Error(err)
		}
		// If we've logged in, save the session and redirect to the dashboard
		binding, bindingErr := models.NewSessionBinding()
		if bindingErr != nil {
			as.handleInvalidLogin(w, r, "Unable to create session")
			return
		}
		session.Values = map[interface{}]interface{}{"id": u.Id, "session_id": binding}
		session.Save(r, w)
		recordBrowserAudit(r, u.Username, u.Id, "auth.login.success", u.Username, "success")
		as.nextOrIndex(w, r)
	}
}

// Logout destroys the current user session
func (as *AdminServer) Logout(w http.ResponseWriter, r *http.Request) {
	u := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)
	binding, _ := session.Values["session_id"].(string)
	_ = models.RevokePrivilegedSession(binding)
	impersonatorID, impersonating := session.Values["impersonator_id"].(int64)
	impersonatorUsername, _ := session.Values["impersonator_username"].(string)
	session.Values = make(map[interface{}]interface{})
	session.Options.MaxAge = -1
	session.Save(r, w)
	if impersonating {
		recordBrowserAudit(r, impersonatorUsername, impersonatorID, "impersonation.stop", u.Username, "success")
	}
	recordBrowserAudit(r, u.Username, u.Id, "auth.logout", u.Username, "success")
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ResetPassword handles the password reset flow when a password change is
// required either by the Darkphish system or an administrator.
//
// This handler is meant to be used when a user is required to reset their
// password, not just when they want to.
//
// This is an important distinction since in this handler we don't require
// the user to re-enter their current password, as opposed to the flow
// through the settings handler.
//
// To that end, if the user doesn't require a password change, we will
// redirect them to the settings page.
func (as *AdminServer) ResetPassword(w http.ResponseWriter, r *http.Request) {
	u := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)
	if !u.PasswordChangeRequired {
		Flash(w, r, "info", "Please reset your password through the settings page")
		session.Save(r, w)
		http.Redirect(w, r, "/settings", http.StatusTemporaryRedirect)
		return
	}
	params := newTemplateParams(r)
	params.Title = "Reset Password"
	switch {
	case r.Method == http.MethodGet:
		params.Flashes = session.Flashes()
		session.Save(r, w)
		getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
		return
	case r.Method == http.MethodPost:
		newPassword := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")
		newHash, err := auth.ValidatePasswordChange(u.Hash, newPassword, confirmPassword)
		if err != nil {
			Flash(w, r, "danger", err.Error())
			params.Flashes = session.Flashes()
			session.Save(r, w)
			w.WriteHeader(http.StatusBadRequest)
			getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
			return
		}
		u.PasswordChangeRequired = false
		u.Hash = newHash
		if err = models.PutUser(&u); err != nil {
			Flash(w, r, "danger", err.Error())
			params.Flashes = session.Flashes()
			session.Save(r, w)
			w.WriteHeader(http.StatusInternalServerError)
			getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
			return
		}
		if u.Username == models.DefaultAdminUsername {
			models.RemoveInitialAdminPasswordFile()
		}
		_ = models.RevokeUserPrivilegedSessions(u.Id)
		recordBrowserAudit(r, u.Username, u.Id, "password.change", u.Username, "success")
		// TODO: We probably want to flash a message here that the password was
		// changed successfully. The problem is that when the user resets their
		// password on first use, they will see two flashes on the dashboard-
		// one for their password reset, and one for the "no campaigns created".
		//
		// The solution to this is to revamp the empty page to be more useful,
		// like a wizard or something.
		as.nextOrIndex(w, r)
	}
}

// TODO: Make this execute the template, too
func getTemplate(w http.ResponseWriter, tmpl string) *template.Template {
	templates := template.New("template")
	_, err := templates.ParseFiles("templates/base.html", "templates/nav.html", "templates/"+tmpl+".html", "templates/flashes.html")
	if err != nil {
		log.Error(err)
	}
	return template.Must(templates, err)
}

// Flash handles the rendering flash messages
func Flash(w http.ResponseWriter, r *http.Request, t string, m string) {
	session := ctx.Get(r, "session").(*sessions.Session)
	session.AddFlash(models.Flash{
		Type:    t,
		Message: m,
	})
}
