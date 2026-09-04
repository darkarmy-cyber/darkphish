package api

import (
	"net/http"

	"github.com/darkarmy-cyber/darkphish/models"
)

// Reset is retained for one release as an explicit migration response. Legacy
// permanent API keys are never issued or returned.
func (as *Server) Reset(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, models.Response{Success: false, Message: "Legacy API keys are disabled; create an expiring personal access token under Settings"}, http.StatusGone)
}
