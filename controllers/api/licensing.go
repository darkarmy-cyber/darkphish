package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"github.com/darkarmy-cyber/darkphish/models"
)

type licenseActivationRequest struct {
	LicenseKey string `json:"license_key"`
}

func (as *Server) LicenseStatus(w http.ResponseWriter, r *http.Request) {
	status, err := models.GetLicenseStatus(time.Now().UTC())
	if err != nil && !errors.Is(err, models.ErrLicensingNotConfigured) {
		JSONResponse(w, models.Response{Success: false, Message: "Unable to read Community license status"}, http.StatusServiceUnavailable)
		return
	}
	JSONResponse(w, status, http.StatusOK)
}

func (as *Server) LicenseActivate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request licenseActivationRequest
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.LicenseKey) == "" {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid activation request"}, http.StatusBadRequest)
		return
	}
	// Keep the public key out of logs and error responses. The licensing client
	// also deliberately suppresses remote error bodies.
	status, err := models.ActivateCommunityLicense(r.Context(), strings.TrimSpace(request.LicenseKey), time.Now().UTC())
	if err != nil {
		code := http.StatusServiceUnavailable
		switch {
		case errors.Is(err, licensing.ErrInvalidSignature), errors.Is(err, licensing.ErrInvalidLease), errors.Is(err, licensing.ErrUnknownSigningKey):
			code = http.StatusBadGateway
		case errors.Is(err, models.ErrLicensingNotConfigured):
			code = http.StatusServiceUnavailable
		}
		JSONResponse(w, models.Response{Success: false, Message: "Community license activation failed"}, code)
		return
	}
	JSONResponse(w, status, http.StatusOK)
}

func (as *Server) LicenseRefresh(w http.ResponseWriter, r *http.Request) {
	status, err := models.RefreshCommunityLicense(r.Context(), time.Now().UTC())
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Community license refresh failed"}, http.StatusServiceUnavailable)
		return
	}
	JSONResponse(w, status, http.StatusOK)
}
