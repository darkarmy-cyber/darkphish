package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

// The CodeQL path for #9 enters attachment validation via both template writes.
// Exercise the complete authenticated router, including the audit status writer,
// with HTML in a non-XML ZIP member. ZIP bytes must never become HTTP HTML.
func TestTemplateAttachmentAdministrativeSerialization(t *testing.T) {
	ctx := setupTest(t)
	t.Cleanup(func() { _ = models.Close() })
	const html = `<script>window.syntheticMarker="attachment-boundary"</script>`
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	member, err := writer.Create("synthetic.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte(html)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	template := models.Template{Name: "boundary template", HTML: html, Attachments: []models.Attachment{{Name: "synthetic.docx", Type: "application/zip", Content: base64.StdEncoding.EncodeToString(archive.Bytes())}}}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/api/templates/"
		wantStatus := http.StatusCreated
		if method == http.MethodPut {
			path = fmt.Sprintf("/api/templates/%d", template.Id)
			wantStatus = http.StatusOK
		}
		body, err := json.Marshal(template)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+ctx.apiKey)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ctx.apiServer.ServeHTTP(w, r)
		if w.Code != wantStatus {
			t.Fatalf("%s failed: status %d", method, w.Code)
		}
		if w.Header().Get("Content-Type") != "application/json" || strings.Contains(w.Body.String(), "<script") || bytes.Contains(w.Body.Bytes(), []byte("PK\x03\x04")) {
			t.Fatal("administrative response exposed executable HTML or raw ZIP bytes")
		}
		var returned models.Template
		if err := json.Unmarshal(w.Body.Bytes(), &returned); err != nil {
			t.Fatal("response is not a single JSON document")
		}
		if returned.HTML != html || len(returned.Attachments) != 1 || returned.Attachments[0].Content != template.Attachments[0].Content {
			t.Fatal("safe serialization must preserve authorized simulation content")
		}
		template.Id = returned.Id
	}
}
