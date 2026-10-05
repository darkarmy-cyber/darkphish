package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/directoryimport"
	"github.com/darkarmy-cyber/darkphish/models"
)

type directoryImportCapability struct {
	Enabled bool `json:"enabled"`
}

func (as *Server) LDAPImportCapability(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, directoryImportCapability{Enabled: models.CheckDirectoryImportLicense(time.Now().UTC()) == nil}, http.StatusOK)
}

func (as *Server) PreviewLDAPImport(w http.ResponseWriter, r *http.Request) {
	if err := models.CheckDirectoryImportLicense(time.Now().UTC()); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "LDAP / Active Directory import requires Darkphish Professional or Enterprise"}, http.StatusForbidden)
		return
	}
	var request directoryimport.Config
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid LDAP import request"}, http.StatusBadRequest)
		return
	}
	preview, err := directoryimport.PreviewLDAP(r.Context(), request)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, directoryimport.ErrInvalidConfig) {
			code = http.StatusBadRequest
		}
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, code)
		return
	}
	JSONResponse(w, preview, http.StatusOK)
}
