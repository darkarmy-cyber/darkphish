package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
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
	workCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	preview, err := directoryimport.PreviewLDAP(workCtx, request)
	if err != nil {
		code := http.StatusBadGateway
		message := "LDAP directory preview failed"
		if errors.Is(err, directoryimport.ErrInvalidConfig) {
			code = http.StatusBadRequest
			message = "Invalid LDAP import configuration"
		}
		JSONResponse(w, models.Response{Success: false, Message: message}, code)
		return
	}
	host, groupDN, identityErr := directoryimport.NormalizeAuditIdentity(request)
	if identityErr == nil {
		fingerprint := sha256.Sum256([]byte(host + "|" + groupDN))
		user := ctx.Get(r, "user").(models.User)
		authMethod, _ := ctx.Get(r, "auth_method").(string)
		target := fmt.Sprintf("/directory-import/%x:m%d:e%d", fingerprint[:8], preview.Matched, preview.Excluded)
		audit.Record(r, user.Username, user.Id, "directory.import.preview.detail", target, "success", authMethod)
	}
	JSONResponse(w, preview, http.StatusOK)
}
