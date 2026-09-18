package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/models"
)

const maxPreviewImages = 12
const maxPreviewImageBytes = 1 << 20
const maxPreviewPixels = 1_000_000

var errImagePreview = errors.New("image unavailable, unsupported, too large, or destination blocked")
var imagePreviewSlots = make(chan struct{}, 2)

type imagePreviewResult struct {
	URL   string `json:"url"`
	Data  string `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func validImagePreviewURL(raw string) bool {
	_, err := parseImagePreviewURL(raw)
	return err == nil
}

// Return the validated representation so the request is built from exactly the
// URL that passed policy, not from the original, independently reparsed input.
func parseImagePreviewURL(raw string) (*url.URL, error) {
	u, err := parseImportURL(raw)
	if err != nil || u.Scheme != "https" {
		return nil, errImagePreview
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number != 443 {
			return nil, errImagePreview
		}
		// Match the browser's numeric default-port handling and use a normal
		// HTTP Host value. Keep the original input URL only in the API result.
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u, nil
}

// A bounded writer prevents re-encoding a small compressed input into a large
// response. DecodeConfig bounds allocations before the full raster is decoded.
type previewBuffer struct{ bytes.Buffer }

func (b *previewBuffer) Write(p []byte) (int, error) {
	if len(p) > maxPreviewImageBytes-b.Len() {
		return 0, errImagePreview
	}
	return b.Buffer.Write(p)
}

func rasterPreview(content []byte) (string, error) {
	if len(content) > maxPreviewImageBytes {
		return "", errImagePreview
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxPreviewPixels/cfg.Height {
		return "", errImagePreview
	}
	picture, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return "", errImagePreview
	}
	var output previewBuffer
	if err := png.Encode(&output, picture); err != nil {
		return "", errImagePreview
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes()), nil
}

func readImagePreview(response *http.Response) (string, error) {
	if response.StatusCode != http.StatusOK || response.ContentLength > maxPreviewImageBytes {
		return "", errImagePreview
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxPreviewImageBytes+1))
	if err != nil {
		return "", errImagePreview
	}
	return rasterPreview(content)
}

// PreviewEmailImages is behind API authentication, CSRF, PAT scope, view-only
// restrictions and the sensitive-operation limiter. No network access on GET.
func (as *Server) PreviewEmailImages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		URLs []string `json:"urls"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || len(input.URLs) == 0 || len(input.URLs) > maxPreviewImages {
		JSONResponse(w, models.Response{Success: false, Message: "Provide between 1 and 12 image URLs"}, http.StatusBadRequest)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid preview request"}, http.StatusBadRequest)
		return
	}
	for _, raw := range input.URLs {
		if !validImagePreviewURL(raw) {
			JSONResponse(w, models.Response{Success: false, Message: "Image previews require HTTPS URLs on port 443 without credentials"}, http.StatusBadRequest)
			return
		}
	}
	select {
	case imagePreviewSlots <- struct{}{}:
		defer func() { <-imagePreviewSlots }()
	default:
		JSONResponse(w, models.Response{Success: false, Message: "Image preview is busy; try again later"}, http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	results := make([]imagePreviewResult, 0, len(input.URLs))
	seen := make(map[string]bool)
	for _, raw := range input.URLs {
		if seen[raw] {
			continue
		}
		seen[raw] = true
		result := imagePreviewResult{URL: raw}
		data, err := fetchImagePreview(ctx, raw)
		if err != nil {
			result.Error = errImagePreview.Error()
		} else {
			result.Data = data
		}
		results = append(results, result)
	}
	JSONResponse(w, results, http.StatusOK)
}
