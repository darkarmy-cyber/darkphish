package models

import (
	"strings"
	"testing"

	"gopkg.in/check.v1"
)

func TestStripLegacyLandingPageMarkup(t *testing.T) {
	source := `<!DOCTYPE html><html><head><title>Training simulation</title></head><body data-darkphish-training="static-v1"><p data-training-notice="true" style="color:red">Training simulation — do not enter real credentials. Forms and scripts are disabled.</p><p data-training-notice="true">Custom notice that must remain</p><form><input name="email" value="{{.Email}}"></form><p>Keep {{\example}} and {{.Email}} literal</p><img data-training-image-url="https://assets.example.test/logo.png"></body></html>`
	cleaned, changed, err := stripLegacyLandingPageMarkup(source)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("legacy document was not detected")
	}
	for _, removed := range []string{"data-darkphish-training=\"static-v1\"", "data-training-image-url", legacyLandingPageNoticeText, "<title>Training simulation</title>"} {
		if strings.Contains(cleaned, removed) {
			t.Fatalf("legacy markup survived: %s\n%s", removed, cleaned)
		}
	}
	for _, kept := range []string{"<form>", `name="email"`, "Custom notice that must remain", `data-training-notice="true"`, "<img"} {
		if !strings.Contains(cleaned, kept) {
			t.Fatalf("page content was lost: %s\n%s", kept, cleaned)
		}
	}
	if err := ValidateTemplate(cleaned); err != nil {
		t.Fatalf("cleaned legacy page is not a valid template: %v\n%s", err, cleaned)
	}
	rendered, err := ExecuteTemplate(cleaned, map[string]string{"Email": "activated@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{`{{\example}}`, `{{.Email}}`, `value="{{.Email}}"`} {
		if !strings.Contains(rendered, literal) {
			t.Fatalf("legacy template text was not preserved literally: %s\n%s", literal, rendered)
		}
	}
	if strings.Contains(rendered, "activated@example.test") {
		t.Fatal("legacy template expression became active")
	}
	page := Page{Name: "Legacy round trip", HTML: cleaned, CaptureCredentials: true}
	if err := page.Validate(); err != nil {
		t.Fatalf("legacy page failed normal save validation: %v", err)
	}
	rendered, err = ExecuteTemplate(page.HTML, map[string]string{"Email": "activated@example.test"})
	if err != nil || !strings.Contains(rendered, `value="{{.Email}}"`) || strings.Contains(rendered, "activated@example.test") {
		t.Fatalf("normal page save activated or damaged a legacy attribute: %v\n%s", err, rendered)
	}
	again, changed, err := stripLegacyLandingPageMarkup(cleaned)
	if err != nil || changed || again != cleaned {
		t.Fatal("cleanup must be idempotent")
	}
}

func (s *ModelsSuite) TestCleanupLegacyLandingPagesPersistsContent(c *check.C) {
	page := Page{UserId: 1, Name: "Legacy page", HTML: `<body data-darkphish-training="static-v1"><p data-training-notice="true">Training simulation — do not enter real credentials. Forms and scripts are disabled.</p><form><input name="email"></form><p>Keep me</p></body>`}
	c.Assert(PostPage(&page), check.IsNil)
	c.Assert(cleanupLegacyLandingPages(db), check.IsNil)
	fetched, err := GetPage(page.Id, page.UserId)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(fetched.HTML, "data-darkphish-training"), check.Equals, false)
	c.Assert(strings.Contains(fetched.HTML, "data-training-notice"), check.Equals, false)
	c.Assert(strings.Contains(fetched.HTML, "Keep me"), check.Equals, true)
	c.Assert(strings.Contains(fetched.HTML, "<form"), check.Equals, true)
	c.Assert(cleanupLegacyLandingPages(db), check.IsNil)
}

func TestStripLegacyLandingPageMarkupLeavesOrdinaryHTMLUnchanged(t *testing.T) {
	source := `<html><head><title>Ordinary page</title></head><body data-darkphish-training="custom"><p>Keep exact source</p></body></html>`
	cleaned, changed, err := stripLegacyLandingPageMarkup(source)
	if err != nil || changed || cleaned != source {
		t.Fatal("ordinary HTML changed")
	}
}
