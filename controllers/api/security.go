package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type reauthenticationRequest struct {
	Method   string `json:"method"`
	Password string `json:"password"`
}

func (as *Server) sensitiveKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	user, _ := ctx.Get(r, "user").(models.User)
	return strconv.FormatInt(user.Id, 10) + ":" + host
}

func (as *Server) limitSensitive(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !as.sensitiveLimiter.AllowKey(as.sensitiveKey(r)) {
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/reauthenticate" {
				user, _ := ctx.Get(r, "user").(models.User)
				authMethod, _ := ctx.Get(r, "auth_method").(string)
				audit.Record(r, user.Username, user.Id, "auth.reauthentication.failure", "privileged-session", "rate_limited", authMethod)
			}
			JSONResponse(w, models.Response{Success: false, Message: "Sensitive operation rate limit exceeded"}, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (as *Server) Reauthenticate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	user := ctx.Get(r, "user").(models.User)
	if method, _ := ctx.Get(r, "auth_method").(string); method != "session" {
		audit.Record(r, user.Username, user.Id, "auth.reauthentication.failure", "privileged-session", "failure", method)
		JSONResponse(w, models.Response{Success: false, Message: "Privileged reauthentication is available only to browser sessions"}, http.StatusForbidden)
		return
	}
	var request reauthenticationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		audit.Record(r, user.Username, user.Id, "auth.reauthentication.failure", "privileged-session", "failure", "session")
		JSONResponse(w, models.Response{Success: false, Message: "Invalid reauthentication proof"}, http.StatusBadRequest)
		return
	}
	if request.Method == "" {
		request.Method = "password"
	}
	session := ctx.Get(r, "session").(*sessions.Session)
	binding, _ := session.Values["session_id"].(string)
	if binding == "" {
		var err error
		binding, err = models.NewSessionBinding()
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to create privileged session"}, http.StatusInternalServerError)
			return
		}
	}
	privileged, err := models.ReauthenticatePrivileged(r.Context(), user, binding, models.ReauthenticationProof{Method: request.Method, Secret: request.Password}, time.Now().UTC())
	request.Password = ""
	if err != nil {
		audit.Record(r, user.Username, user.Id, "auth.reauthentication.failure", "privileged-session", "failure", "session")
		status := http.StatusUnauthorized
		if err != auth.ErrInvalidPassword {
			status = http.StatusBadRequest
		}
		JSONResponse(w, models.Response{Success: false, Message: "Reauthentication failed"}, status)
		return
	}
	session.Values["session_id"] = binding
	if err := session.Save(r, w); err != nil {
		_ = models.RevokePrivilegedSession(binding)
		JSONResponse(w, models.Response{Success: false, Message: "Unable to bind privileged session"}, http.StatusInternalServerError)
		return
	}
	audit.Record(r, user.Username, user.Id, "auth.reauthentication.success", "privileged-session", "success", "session")
	JSONResponse(w, map[string]interface{}{"reauthenticated_at": privileged.ReauthenticatedAt, "expires_at": privileged.ExpiresAt}, http.StatusOK)
}

type reviewerAssignmentRequest struct {
	UserID    int64      `json:"user_id"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type reviewerManagementResponse struct {
	Assignments []models.CampaignReviewer `json:"assignments"`
	Candidates  []models.User             `json:"candidates"`
}

func reviewerAuditEvent(r *http.Request, user models.User) audit.Event {
	method, _ := ctx.Get(r, "auth_method").(string)
	return audit.NewRequestEvent(r, user.Username, user.Id, "", "", "success", method)
}

func (as *Server) CampaignReviewers(w http.ResponseWriter, r *http.Request) {
	user := ctx.Get(r, "user").(models.User)
	campaignID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if r.Method == http.MethodGet {
		values, err := models.GetCampaignReviewers(campaignID, user)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Reviewer assignments are not available"}, http.StatusForbidden)
			return
		}
		candidates, err := models.GetEligibleSecurityReviewers()
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Reviewer candidates are not available"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, reviewerManagementResponse{Assignments: values, Candidates: candidates}, http.StatusOK)
		return
	}
	var request reviewerAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.UserID == 0 {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid reviewer assignment"}, http.StatusBadRequest)
		return
	}
	value, err := models.AssignCampaignReviewer(campaignID, request.UserID, user, request.ExpiresAt, reviewerAuditEvent(r, user))
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid reviewer assignment"}, http.StatusBadRequest)
		return
	}
	JSONResponse(w, value, http.StatusCreated)
}

func (as *Server) CampaignReviewer(w http.ResponseWriter, r *http.Request) {
	user := ctx.Get(r, "user").(models.User)
	vars := mux.Vars(r)
	campaignID, _ := strconv.ParseInt(vars["id"], 10, 64)
	reviewerID, _ := strconv.ParseInt(vars["user_id"], 10, 64)
	if err := models.RemoveCampaignReviewer(campaignID, reviewerID, user, reviewerAuditEvent(r, user)); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Reviewer assignment not found"}, http.StatusNotFound)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Reviewer assignment removed"}, http.StatusOK)
}
