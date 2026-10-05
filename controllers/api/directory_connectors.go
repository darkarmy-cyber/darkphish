package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/directorysync"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

func requireProDirectory(w http.ResponseWriter) bool {
	if err := models.CheckDirectoryImportLicense(time.Now().UTC()); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Managed directory connectors require Darkphish Professional or Enterprise"}, http.StatusForbidden)
		return false
	}
	return true
}

func (as *Server) DirectoryConnectors(w http.ResponseWriter, r *http.Request) {
	if !requireProDirectory(w) {
		return
	}
	user := ctx.Get(r, "user").(models.User)
	switch r.Method {
	case http.MethodGet:
		connectors, err := models.GetDirectoryConnectors(user.Id)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to load directory connectors"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, connectors, http.StatusOK)
	case http.MethodPost:
		var connector models.DirectoryConnector
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&connector); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid directory connector request"}, http.StatusBadRequest)
			return
		}
		connector.OwnerUserID = user.Id
		if err := models.PostDirectoryConnector(&connector); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to create directory connector"}, http.StatusBadRequest)
			return
		}
		connector.ClientSecret = ""
		connector.ClientSecretSet = true
		JSONResponse(w, connector, http.StatusCreated)
	}
}

func (as *Server) DirectoryConnector(w http.ResponseWriter, r *http.Request) {
	if !requireProDirectory(w) {
		return
	}
	user := ctx.Get(r, "user").(models.User)
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid directory connector id"}, http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		connector, err := models.GetDirectoryConnector(id, user.Id)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Directory connector not found"}, http.StatusNotFound)
			return
		}
		JSONResponse(w, connector, http.StatusOK)
	case http.MethodPut:
		var connector models.DirectoryConnector
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&connector); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid directory connector request"}, http.StatusBadRequest)
			return
		}
		connector.ID = id
		connector.OwnerUserID = user.Id
		if err := models.PutDirectoryConnector(&connector); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, models.ErrDirectoryConnectorConcurrentEdit) {
				status = http.StatusConflict
			}
			JSONResponse(w, models.Response{Success: false, Message: "Unable to update directory connector"}, status)
			return
		}
		JSONResponse(w, connector, http.StatusOK)
	case http.MethodDelete:
		if err := models.DeleteDirectoryConnector(id, user.Id); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, gorm.ErrRecordNotFound) {
				status = http.StatusNotFound
			}
			JSONResponse(w, models.Response{Success: false, Message: "Unable to delete directory connector"}, status)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Directory connector deleted"}, http.StatusOK)
	}
}

func (as *Server) DirectoryConnectorPreview(w http.ResponseWriter, r *http.Request) {
	if !requireProDirectory(w) {
		return
	}
	user := ctx.Get(r, "user").(models.User)
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid directory connector id"}, http.StatusBadRequest)
		return
	}
	connector, err := models.GetDirectoryConnectorWithSecret(id, user.Id)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Directory connector not found"}, http.StatusNotFound)
		return
	}
	workCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	recipients, err := directorysync.PreviewEntraGroup(workCtx, directorysync.EntraConfig{
		TenantID: connector.TenantID, ClientID: connector.ClientID, ClientSecret: connector.ClientSecret,
		RemoteGroupID: connector.RemoteGroupID, EmailDomains: connector.EmailDomains,
	})
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Microsoft Entra directory preview failed"}, http.StatusBadGateway)
		return
	}
	JSONResponse(w, recipients, http.StatusOK)
}

func (as *Server) DirectoryConnectorHistory(w http.ResponseWriter, r *http.Request) {
	if !requireProDirectory(w) {
		return
	}
	user := ctx.Get(r, "user").(models.User)
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid directory connector id"}, http.StatusBadRequest)
		return
	}
	runs, err := models.GetDirectorySyncRuns(id, user.Id, 25)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Unable to load directory sync history"}, http.StatusNotFound)
		return
	}
	JSONResponse(w, runs, http.StatusOK)
}
