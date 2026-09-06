package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/mux"
)

// Campaigns returns a list of campaigns if requested via GET.
// If requested via POST, APICampaigns creates a new campaign and returns a reference to it.
func (as *Server) Campaigns(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet:
		cs, err := models.GetAccessibleCampaigns(ctx.Get(r, "user").(models.User), time.Now().UTC())
		if err != nil {
			log.Error(err)
		}
		JSONResponse(w, cs, http.StatusOK)
	//POST: Create a new campaign and return it as JSON
	case r.Method == "POST":
		c := models.Campaign{}
		// Put the request into a campaign
		err := json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		err = models.PostCampaign(&c, ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		// If the campaign is scheduled to launch immediately, send it to the worker.
		// Otherwise, the worker will pick it up at the scheduled time
		if c.Status == models.CampaignInProgress {
			go as.worker.LaunchCampaign(c)
		}
		JSONResponse(w, c, http.StatusCreated)
	}
}

// CampaignsSummary returns the summary for the current user's campaigns
func (as *Server) CampaignsSummary(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		cs, err := models.GetAccessibleCampaignSummaries(ctx.Get(r, "user").(models.User), time.Now().UTC())
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, cs, http.StatusOK)
	}
}

// Campaign returns details about the requested campaign. If the campaign is not
// valid, APICampaign returns null.
func (as *Server) Campaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	c, err := models.GetAccessibleCampaign(id, ctx.Get(r, "user").(models.User))
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
		return
	}
	switch {
	case r.Method == "GET":
		JSONResponse(w, c, http.StatusOK)
	case r.Method == "DELETE":
		err = models.DeleteCampaign(id)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Error deleting campaign"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Campaign deleted successfully!"}, http.StatusOK)
	}
}

// CampaignResults returns just the results for a given campaign to
// significantly reduce the information returned.
func (as *Server) CampaignResults(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	cr, err := models.GetCampaignResults(id, ctx.Get(r, "user_id").(int64))
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
		return
	}
	if r.Method == "GET" {
		JSONResponse(w, cr, http.StatusOK)
		return
	}
}

// CampaignCredentialReveal returns one retained credential only after an
// explicit POST to the dedicated reveal route. The value is never included in
// normal campaign results or export routes.
func (as *Server) CampaignCredentialReveal(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 10, 64)
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	reveal, err := models.RevealCredential(id, vars["rid"], ctx.Get(r, "user_id").(int64))
	if err != nil {
		switch {
		case errors.Is(err, models.ErrCredentialExpired):
			JSONResponse(w, models.Response{Success: false, Message: "Retained credential has expired"}, http.StatusGone)
		case errors.Is(err, models.ErrCredentialReviewDisabled):
			JSONResponse(w, models.Response{Success: false, Message: "Credential review is not enabled"}, http.StatusForbidden)
		case errors.Is(err, secretpkg.ErrProviderUnavailable), errors.Is(err, secretpkg.ErrWrongProvider):
			JSONResponse(w, models.Response{Success: false, Message: "Credential key provider is unavailable"}, http.StatusServiceUnavailable)
		case errors.Is(err, secretpkg.ErrUnknownKey), errors.Is(err, secretpkg.ErrInvalidCiphertext), errors.Is(err, models.ErrCredentialEncryption):
			JSONResponse(w, models.Response{Success: false, Message: "Retained credential is unavailable"}, http.StatusServiceUnavailable)
		default:
			JSONResponse(w, models.Response{Success: false, Message: "Retained credential not found"}, http.StatusNotFound)
		}
		return
	}
	JSONResponse(w, reveal, http.StatusOK)
}

// CampaignSummary returns the summary for a given campaign.
func (as *Server) CampaignSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	switch {
	case r.Method == "GET":
		cs, err := models.GetAccessibleCampaignSummary(id, ctx.Get(r, "user").(models.User))
		if err != nil {
			if errors.Is(err, models.ErrRecordNotFound) {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
			} else {
				JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			}
			log.Error(err)
			return
		}
		JSONResponse(w, cs, http.StatusOK)
	}
}

// CampaignComplete effectively "ends" a campaign.
// Future phishing emails clicked will return a simple "404" page.
func (as *Server) CampaignComplete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	switch {
	case r.Method == http.MethodPost:
		err := models.CompleteCampaign(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Error completing campaign"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Campaign completed successfully!"}, http.StatusOK)
	}
}
