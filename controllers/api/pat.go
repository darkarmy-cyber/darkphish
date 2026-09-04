package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/mux"
)

type createPATRequest struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type createPATResponse struct {
	models.PersonalAccessToken
	Token string `json:"token"`
}

func (as *Server) PersonalAccessTokens(w http.ResponseWriter, r *http.Request) {
	userID := ctx.Get(r, "user_id").(int64)
	switch r.Method {
	case http.MethodGet:
		values, err := models.GetPersonalAccessTokens(userID)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to list personal access tokens"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, values, http.StatusOK)
	case http.MethodPost:
		var request createPATRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		pat, raw, err := models.CreatePersonalAccessToken(userID, request.Name, request.Scopes, request.ExpiresAt)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		JSONResponse(w, createPATResponse{PersonalAccessToken: pat, Token: raw}, http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (as *Server) PersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err := models.RevokePersonalAccessToken(id, ctx.Get(r, "user_id").(int64)); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Personal access token not found"}, http.StatusNotFound)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Personal access token revoked"}, http.StatusOK)
}
