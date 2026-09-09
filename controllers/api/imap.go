package api

import (
	"encoding/json"
	"net/http"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/imap"
	"github.com/darkarmy-cyber/darkphish/models"
)

// IMAPServerValidate handles requests for the /api/imapserver/validate endpoint
func (as *Server) IMAPServerValidate(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		JSONResponse(w, models.Response{Success: false, Message: "Only POSTs allowed"}, http.StatusBadRequest)
	case r.Method == "POST":
		im := models.IMAP{}
		err := json.NewDecoder(r.Body).Decode(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid request"}, http.StatusBadRequest)
			return
		}
		if im.Password == "" {
			existing, getErr := models.GetIMAP(ctx.Get(r, "user_id").(int64))
			if getErr == nil && len(existing) > 0 {
				im.Password = existing[0].Password
			}
		}
		err = imap.Validate(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusOK)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Successful login."}, http.StatusCreated)
	}
}

// IMAPServer handles requests for the /api/imapserver/ endpoint
func (as *Server) IMAPServer(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		ss, err := models.GetIMAP(ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, ss, http.StatusOK)

	// POST: Update database
	case r.Method == "POST":
		im := models.IMAP{}
		err := json.NewDecoder(r.Body).Decode(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid data. Please check your IMAP settings."}, http.StatusBadRequest)
			return
		}
		uid := ctx.Get(r, "user_id").(int64)
		// Passwords are write-only and are never returned by GET. An empty password
		// on an update therefore means "keep the existing protected password", not
		// "replace it with an empty password". A first-time configuration still
		// reaches model validation and requires an explicit password.
		if im.Password == "" {
			existing, getErr := models.GetIMAP(uid)
			if getErr != nil {
				JSONResponse(w, models.Response{Success: false, Message: getErr.Error()}, http.StatusInternalServerError)
				return
			}
			if len(existing) > 0 {
				im.Password = existing[0].Password
			}
		}
		im.ModifiedDate = time.Now().UTC()
		im.UserId = uid
		err = models.PostIMAP(&im, uid)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Successfully saved IMAP settings."}, http.StatusCreated)
	}
}
