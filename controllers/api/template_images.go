package api

import (
	"bytes"
	"encoding/base64"
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
)

const maxPreviewImageBytes = 1 << 20
const maxPreviewPixels = 1_000_000

var errImagePreview = errors.New("image unavailable, unsupported, too large, or destination blocked")
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
