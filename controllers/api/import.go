package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/darkarmy-cyber/darkphish/internal/importhtml"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/darkarmy-cyber/darkphish/util"
	"github.com/jordan-wright/email"
)

type cloneRequest struct {
	URL              string `json:"url"`
	IncludeResources bool   `json:"include_resources"`
}

func (cr *cloneRequest) validate() error {
	_, err := parseImportURL(cr.URL)
	return err
}

var renderImportPageForImport = renderImportPage

// Keep the complete site-import pipeline comfortably inside the admin server's
// 30-second WriteTimeout. The shared context starts before request-body decoding
// and bounds decoding, fetching and rendering, leaving time for the JSON reply.
const importSiteWorkTimeout = 25 * time.Second

type cloneResponse struct {
	HTML     string   `json:"html"`
	Mode     string   `json:"mode"`
	Warnings []string `json:"warnings,omitempty"`
}

type emailResponse struct {
	Text        string              `json:"text"`
	HTML        string              `json:"html"`
	Subject     string              `json:"subject"`
	Attachments []models.Attachment `json:"attachments"`
	Warnings    []string            `json:"warnings,omitempty"`
}

// ImportGroup imports a CSV of group members
func (as *Server) ImportGroup(w http.ResponseWriter, r *http.Request) {
	ts, err := util.ParseCSV(r)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Error parsing CSV"}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, ts, http.StatusOK)
}

// ImportEmail allows for the importing of email.
// Returns a Message object
func (as *Server) ImportEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusBadRequest)
		return
	}
	ir := struct {
		Content      string `json:"content"`
		ConvertLinks bool   `json:"convert_links"`
	}{}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportedEmailBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ir); err != nil || decoder.Decode(&struct{}{}) != io.EOF || len(ir.Content) == 0 || len(ir.Content) > maxImportedEmailBytes {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid or oversized raw email request"}, http.StatusBadRequest)
		return
	}
	e, err := email.NewEmailFromReader(strings.NewReader(ir.Content))
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Unable to parse email; provide the complete raw email source"}, http.StatusBadRequest)
		return
	}

	// Import MIME resources and public external images as real template
	// attachments. HTML references are rewritten to stable CID names, so the
	// result works in an actually sent message instead of only in the editor.
	assets := collectImportedEmailAssets(ir.Content)
	localizedHTML, attachments, warnings := localizeImportedEmailHTML(r.Context(), string(e.HTML), assets)
	e.HTML = []byte(localizedHTML)

	// If the user wants to convert links to point to
	// the landing page, let's make it happen by changing up
	// e.HTML after image resources have been localized.
	if ir.ConvertLinks {
		d, err := goquery.NewDocumentFromReader(bytes.NewReader(e.HTML))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		d.Find("a").Each(func(i int, a *goquery.Selection) {
			a.SetAttr("href", "{{.URL}}")
		})
		h, err := d.Html()
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		e.HTML = []byte(h)
	}
	er := emailResponse{
		Subject:     e.Subject,
		Text:        string(e.Text),
		HTML:        string(e.HTML),
		Attachments: attachments,
		Warnings:    warnings,
	}
	JSONResponse(w, er, http.StatusOK)
}

// ImportSite allows for the importing of HTML from a website
// Downloaded HTML is sanitized before it is returned to the editor.
func (as *Server) ImportSite(w http.ResponseWriter, r *http.Request) {
	workCtx, cancel := context.WithTimeout(r.Context(), importSiteWorkTimeout)
	defer cancel()

	cr := cloneRequest{}
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusBadRequest)
		return
	}
	if deadline, ok := workCtx.Deadline(); ok {
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to enforce request deadline"}, http.StatusInternalServerError)
			return
		}
		defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	}
	err := json.NewDecoder(r.Body).Decode(&cr)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Error decoding JSON Request"}, http.StatusBadRequest)
		return
	}
	if err = cr.validate(); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
		return
	}
	content, sourceURL, err := fetchImportPage(workCtx, cr.URL)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
		return
	}
	mode := "static"
	var warnings []string
	if rendered, renderedURL, renderErr := renderImportPageForImport(workCtx, sourceURL.String()); renderErr == nil {
		content = rendered
		sourceURL = renderedURL
		mode = "rendered"
	} else {
		warnings = append(warnings, renderedImportWarning(renderErr))
	}
	h, err := importhtml.Sanitize(string(content), sourceURL)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		return
	}
	cs := cloneResponse{HTML: h, Mode: mode, Warnings: warnings}
	JSONResponse(w, cs, http.StatusOK)
}
