package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func rawRelatedEmail(t *testing.T, cidBody []byte, html string) string {
	t.Helper()
	return "Subject: Imported resource test\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=darkphish-boundary\r\n\r\n" +
		"--darkphish-boundary\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" +
		html + "\r\n" +
		"--darkphish-boundary\r\n" +
		"Content-Type: image/png; name=logo.png\r\n" +
		"Content-Disposition: inline; filename=logo.png\r\n" +
		"Content-ID: <logo@example.test>\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString(cidBody) + "\r\n" +
		"--darkphish-boundary--\r\n"
}

func TestEmailImportLocalizesCIDAndExternalImages(t *testing.T) {
	picture, preview := tinyPNG(t)
	raw := rawRelatedEmail(t, picture, `<html><body>
		<img src="cid:logo@example.test">
		<img src="https://images.example.test/photo.jpg">
		<div style="background-image:url('https://images.example.test/photo.jpg')">x</div>
	</body></html>`)

	imported := collectImportedEmailAssets(raw)
	if len(imported.Attachments) != 1 || imported.CIDNames["logo@example.test"] == "" {
		t.Fatalf("CID resource not collected: %+v", imported)
	}
	fetches := 0
	html, attachments, warnings := localizeImportedEmailHTMLWithFetcher(context.Background(),
		`<html><body><img src="cid:logo@example.test"><img src="https://images.example.test/photo.jpg"><div style="background:url(https://images.example.test/photo.jpg)">x</div></body></html>`,
		imported, func(ctx context.Context, raw string) (string, error) {
			fetches++
			if raw != "https://images.example.test/photo.jpg" {
				t.Fatalf("unexpected fetch: %s", raw)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("automatic image import is not deadline bounded")
			}
			return preview, nil
		})
	if fetches != 1 {
		t.Fatalf("external image was not deduplicated: %d fetches", fetches)
	}
	if len(attachments) != 2 || len(warnings) != 0 {
		t.Fatalf("unexpected localized assets: %d warnings=%v", len(attachments), warnings)
	}
	if strings.Contains(html, "https://images.example.test/") || strings.Contains(html, "cid:logo@example.test") {
		t.Fatalf("original image references survived localization: %s", html)
	}
	for _, attachment := range attachments {
		if !strings.HasSuffix(attachment.Name, ".png") || attachment.Type != "image/png" {
			t.Fatalf("unsafe image representation: %+v", attachment)
		}
		if _, err := base64.StdEncoding.DecodeString(attachment.Content); err != nil {
			t.Fatal("attachment is not base64 encoded")
		}
		if !strings.Contains(html, "cid:"+attachment.Name) {
			t.Fatalf("attachment is not referenced by localized HTML: %s", attachment.Name)
		}
	}
}

func TestEmailImportPreservesContentLocationImage(t *testing.T) {
	picture, _ := tinyPNG(t)
	raw := "Subject: Content-Location test\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=location-boundary\r\n\r\n" +
		"--location-boundary\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" +
		"<html><body><img src=\"images/logo.png\"></body></html>\r\n" +
		"--location-boundary\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-Location: images/logo.png\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString(picture) + "\r\n" +
		"--location-boundary--\r\n"

	imported := collectImportedEmailAssets(raw)
	name := imported.ResourceNames["images/logo.png"]
	if name == "" || len(imported.Attachments) != 1 {
		t.Fatalf("Content-Location resource not collected: %+v", imported)
	}
	html, attachments, warnings := localizeImportedEmailHTMLWithFetcher(context.Background(),
		`<html><body><img src="images/logo.png"></body></html>`,
		imported, func(context.Context, string) (string, error) {
			t.Fatal("Content-Location resource must not use the network")
			return "", nil
		})
	if len(warnings) != 0 || len(attachments) != 1 || strings.Contains(html, "images/logo.png") || !strings.Contains(html, "cid:"+name) {
		t.Fatalf("Content-Location resource was not localized: html=%s warnings=%v", html, warnings)
	}
}

func TestEmailImportLeavesUnsafeExternalSchemesUnfetched(t *testing.T) {
	calls := 0
	html, attachments, warnings := localizeImportedEmailHTMLWithFetcher(context.Background(),
		`<html><body><img src="http://internal.example.test/a.png"><img src="//example.test/b.png"></body></html>`,
		importedEmailAssets{CIDNames: map[string]string{}},
		func(context.Context, string) (string, error) {
			calls++
			return "", errImagePreview
		})
	if calls != 0 || len(attachments) != 0 || len(warnings) == 0 {
		t.Fatalf("unsafe image scheme reached fetcher: calls=%d attachments=%d warnings=%v", calls, len(attachments), warnings)
	}
	if !strings.Contains(html, "http://internal.example.test/a.png") {
		t.Fatal("unsupported reference was silently rewritten")
	}
}

func TestEmailImportConvertsSafeDataImageToCID(t *testing.T) {
	_, preview := tinyPNG(t)
	html, attachments, warnings := localizeImportedEmailHTMLWithFetcher(context.Background(),
		`<html><body><img src="`+preview+`"></body></html>`,
		importedEmailAssets{CIDNames: map[string]string{}},
		func(context.Context, string) (string, error) {
			t.Fatal("data image must not use the network")
			return "", nil
		})
	if len(attachments) != 1 || len(warnings) != 0 || strings.Contains(html, "data:image/") || !strings.Contains(html, "cid:"+attachments[0].Name) {
		t.Fatalf("data image was not localized: html=%s attachments=%+v warnings=%v", html, attachments, warnings)
	}
}

func TestImportEmailReturnsEmbeddedAttachments(t *testing.T) {
	picture, _ := tinyPNG(t)
	raw := rawRelatedEmail(t, picture, `<html><body><img src="cid:logo@example.test"></body></html>`)
	payload, err := json.Marshal(map[string]any{"content": raw, "convert_links": false})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/import/email", bytes.NewReader(payload))
	response := httptest.NewRecorder()
	(&Server{}).ImportEmail(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("import failed: %d %s", response.Code, response.Body.String())
	}
	var result emailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Subject != "Imported resource test" || len(result.Attachments) != 1 || strings.Contains(result.HTML, "cid:logo@example.test") {
		t.Fatalf("import did not return localized MIME resources: %+v", result)
	}
	if !strings.Contains(result.HTML, "cid:"+result.Attachments[0].Name) {
		t.Fatal("localized HTML does not reference returned attachment")
	}
}

func TestImportEmailRejectsOversizedOrUnknownJSON(t *testing.T) {
	for _, body := range []string{
		`{"content":"x","convert_links":false,"extra":true}`,
		`{"content":"","convert_links":false}`,
		`{"content":"x","convert_links":false}{}`,
	} {
		response := httptest.NewRecorder()
		(&Server{}).ImportEmail(response, httptest.NewRequest(http.MethodPost, "/api/import/email", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid import request returned %d", response.Code)
		}
	}
}
