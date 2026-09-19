package api

import (
	"net/http"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/update"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/sessions"
)

func WithUpdates(service *update.Service) ServerOption {
	return func(as *Server) { as.updates = service }
}

func (as *Server) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if as.updates == nil {
		JSONResponse(w, map[string]string{"unsupported_reason": "Update service is unavailable"}, http.StatusServiceUnavailable)
		return
	}
	status, err := as.updates.Check(r.Context(), r.Method == http.MethodPost)
	if r.Method == http.MethodPost {
		u := ctx.Get(r, "user").(models.User)
		result := "success"
		if err != nil {
			result = "failure"
		}
		audit.Record(r, u.Username, u.Id, "update.check", "release", result, "session")
	}
	JSONResponse(w, status, http.StatusOK)
}

func (as *Server) UpdateApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	u := ctx.Get(r, "user").(models.User)
	method, _ := ctx.Get(r, "auth_method").(string)
	if method != "session" {
		JSONResponse(w, models.Response{Message: "Updates require a browser session"}, http.StatusForbidden)
		return
	}
	session, _ := ctx.Get(r, "session").(*sessions.Session)
	if session == nil {
		JSONResponse(w, models.Response{Message: "Fresh privileged reauthentication is required"}, http.StatusPreconditionRequired)
		return
	}
	binding, _ := session.Values["session_id"].(string)
	fresh, err := models.IsPrivilegedSessionFresh(u.Id, binding, time.Now().UTC())
	if err != nil || !fresh {
		JSONResponse(w, models.Response{Message: "Fresh privileged reauthentication is required"}, http.StatusPreconditionRequired)
		return
	}
	if as.updates == nil {
		JSONResponse(w, models.Response{Message: "Update service is unavailable"}, http.StatusServiceUnavailable)
		return
	}
	// Consume this privilege grant before handing work to the supervisor.
	if err = models.RevokePrivilegedSession(binding); err != nil {
		JSONResponse(w, models.Response{Message: "Unable to consume privileged session"}, http.StatusInternalServerError)
		return
	}
	target, err := as.updates.ApplyVersion()
	result := "requested"
	if err != nil {
		result = "failure"
	}
	audit.Record(r, u.Username, u.Id, "update.apply", "release", result, method)
	if err != nil {
		JSONResponse(w, models.Response{Message: err.Error()}, http.StatusConflict)
		return
	}
	JSONResponse(w, struct {
		models.Response
		Target string `json:"target_version"`
	}{models.Response{Success: true, Message: "Update requested; the application will restart after verification and backup"}, target}, http.StatusAccepted)
}
