package controllers

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"
	"testing"
)

func TestSendingPagesRenderLicenseWarningAndDisabledActions(t *testing.T) {
	for _, page := range []string{"dashboard", "campaigns", "sending_profiles"} {
		tmpl, err := template.ParseFiles("../templates/base.html", "../templates/nav.html", "../templates/flashes.html", "../templates/license_warning.html", "../templates/"+page+".html")
		if err != nil {
			t.Fatal(err)
		}
		for _, allowed := range []bool{false, true} {
			for _, admin := range []bool{false, true} {
				var output bytes.Buffer
				if err := tmpl.ExecuteTemplate(&output, "body", templateParams{SendingAllowed: allowed, ModifySystem: admin}); err != nil {
					t.Fatal(err)
				}
				html := output.String()
				if strings.Contains(html, "License activation required.") == allowed {
					t.Fatalf("%s incorrect warning", page)
				}
				if !allowed && strings.Contains(html, `href="/settings?tab=licensing"`) != admin {
					t.Fatal("incorrect role-specific activation guidance")
				}
				for _, id := range []string{"launchButton", "sendTestModalSubmit"} {
					button := regexp.MustCompile(`<button[^>]*id="` + id + `"[^>]*>`).FindString(html)
					if button != "" && strings.Contains(button, "disabled") == allowed {
						t.Fatalf("%s %s not fail-closed", page, id)
					}
				}
			}
		}
	}
}
