package api

import (
	"encoding/json"
	"errors"
	"io"
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

func decodeUpdateTarget(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	defer r.Body.Close()
	var input struct {
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return "", errors.New("invalid update request")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", errors.New("invalid update request")
	}
	if input.Version == "" {
		return "", errors.New("target version is required")
	}
	return input.Version, nil
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
	version, err := decodeUpdateTarget(w, r)
	if err != nil {
		JSONResponse(w, models.Response{Message: err.Error()}, http.StatusBadRequest)
		return
	}
	// Consume this privilege grant before handing work to the supervisor.
	if err = models.RevokePrivilegedSession(binding); err != nil {
		JSONResponse(w, models.Response{Message: "Unable to consume privileged session"}, http.StatusInternalServerError)
		return
	}
	target, err := as.updates.ApplyVersion(version)
	result := "requested"
	if err != nil {
		result = "failure"
	}
	audit.Record(r, u.Username, u.Id, "update.apply", "release:"+version, result, method)
	if err != nil {
		JSONResponse(w, models.Response{Message: err.Error()}, http.StatusConflict)
		return
	}
	JSONResponse(w, struct {
		models.Response
		Target string `json:"target_version"`
	}{models.Response{Success: true, Message: "Update requested; the application will restart after verification and backup"}, target}, http.StatusAccepted)
}
