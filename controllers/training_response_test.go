package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func TestStaticTrainingResponseIsLiteralAndRejectsPosts(t *testing.T) {
	p := models.Page{TrainingStatic: true, HTML: `<body data-darkphish-training="static-v1"><p>{{\example}} Žluťoučký kôň</p><script>alert(1)</script><input name=password value=SECRET></body>`, CapturePasswords: true, RedirectURL: "https://example.test"}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		w := httptest.NewRecorder()
		renderPhishResponse(w, httptest.NewRequest(method, "/", nil), models.PhishingTemplateContext{}, p)
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action 'none'") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Fatal("missing sandbox")
		}
		if method == http.MethodPost {
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Location") != "" {
				t.Fatal("submission accepted")
			}
			continue
		}
		if method == http.MethodHead {
			if w.Code != http.StatusOK || w.Body.Len() != 0 {
				t.Fatal("HEAD retrieval must succeed without a response body")
			}
			continue
		}
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `{{\example}}`) {
			t.Fatal("static content parsed as Go template")
		}
		if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "<script") {
			t.Fatal("unsafe render")
		}
	}
}
