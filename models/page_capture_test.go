package models

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func parsedPageDocument(t *testing.T, page *Page) *goquery.Document {
	t.Helper()
	if err := page.Validate(); err != nil {
		t.Fatal(err)
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(page.HTML))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestPageValidationRemovesAllControlNamesWhenCaptureDisabled(t *testing.T) {
	page := Page{Name: "No capture", HTML: `<form action="https://collect.example.test"><input name="input"><textarea name="textarea"></textarea><select name="select"><option>one</option></select><button name="button">go</button></form>`}
	document := parsedPageDocument(t, &page)
	if document.Find("form input[name], form textarea[name], form select[name], form button[name]").Length() != 0 {
		t.Fatal("a successful control retained its name while capture was disabled")
	}
	if action, _ := document.Find("form").Attr("action"); action != "" {
		t.Fatal("form action was not routed back to DarkPhish")
	}
}

func TestPageValidationRemovesPasswordLikeNamesOnly(t *testing.T) {
	page := Page{CaptureCredentials: true, CapturePasswords: false, Name: "Credential capture", HTML: `<form><input name="username"><input type="password" name="password"><textarea autocomplete="new-password" name="recovery"></textarea><select name="tenant"><option>one</option></select><button name="submit">go</button></form>`}
	document := parsedPageDocument(t, &page)
	if document.Find(`input[type="password"][name], textarea[autocomplete="new-password"][name]`).Length() != 0 {
		t.Fatal("a password-like field retained its name")
	}
	for _, selector := range []string{`input[name="username"]`, `select[name="tenant"]`, `button[name="submit"]`} {
		if document.Find(selector).Length() != 1 {
			t.Fatalf("ordinary credential control was removed: %s", selector)
		}
	}
}

func TestPageValidationKeepsPasswordNameWhenEnabled(t *testing.T) {
	page := Page{CaptureCredentials: true, CapturePasswords: true, Name: "Password capture", HTML: `<form><input type="password" name="password"></form>`}
	document := parsedPageDocument(t, &page)
	if document.Find(`input[type="password"][name="password"]`).Length() != 1 {
		t.Fatal("enabled password capture removed the password field name")
	}
}
