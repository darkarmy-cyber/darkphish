package models

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"gorm.io/gorm"
)

const legacyLandingPageMarker = "data-darkphish-training"
const legacyLandingPageNotice = "data-training-notice"
const legacyLandingPageImage = "data-training-image-url"
const legacyLandingPageNoticeText = "Training simulation — do not enter real credentials. Forms and scripts are disabled."
const legacyLiteralOpenDelimiter = "{{`{{`}}"
const legacyLiteralCloseDelimiter = "{{`}`}}{{`}`}}"

func removeAttr(n *html.Node, key string) (string, bool) {
	for i, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return a.Val, true
		}
	}
	return "", false
}

func hasAttr(n *html.Node, key, value string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key && a.Val == value {
			return true
		}
	}
	return false
}

func textContent(n *html.Node) string {
	var value strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			value.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(value.String())
}

func legacyNodeContains(n *html.Node, value string) bool {
	if strings.Contains(n.Data, value) {
		return true
	}
	for _, attribute := range n.Attr {
		if strings.Contains(attribute.Val, value) {
			return true
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if legacyNodeContains(child, value) {
			return true
		}
	}
	return false
}

func legacyDelimiterTokens(doc *html.Node) (string, string) {
	open := "DARKPHISHLEGACYOPENDELIMITER"
	close := "DARKPHISHLEGACYCLOSEDELIMITER"
	for legacyNodeContains(doc, open) || legacyNodeContains(doc, close) {
		open += "X"
		close += "X"
	}
	return open, close
}

func replaceLegacyDelimiters(n *html.Node, open, close string) {
	if n.Type == html.TextNode {
		n.Data = strings.ReplaceAll(strings.ReplaceAll(n.Data, "{{", open), "}}", close)
	}
	if n.Type == html.ElementNode {
		for i := range n.Attr {
			n.Attr[i].Val = strings.ReplaceAll(strings.ReplaceAll(n.Attr[i].Val, "{{", open), "}}", close)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		replaceLegacyDelimiters(child, open, close)
	}
}

func stripLegacyLandingPageMarkup(source string) (string, bool, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", false, err
	}
	var legacyBody *html.Node
	var findBody func(*html.Node)
	findBody = func(n *html.Node) {
		if legacyBody == nil && n.Type == html.ElementNode && n.Data == "body" && hasAttr(n, legacyLandingPageMarker, "static-v1") {
			legacyBody = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			findBody(child)
		}
	}
	findBody(doc)
	if legacyBody == nil {
		return source, false, nil
	}
	openDelimiter, closeDelimiter := legacyDelimiterTokens(doc)
	_, _ = removeAttr(legacyBody, legacyLandingPageMarker)
	changed := true
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode && hasAttr(child, legacyLandingPageNotice, "true") && textContent(child) == legacyLandingPageNoticeText {
				n.RemoveChild(child)
				changed = true
				child = next
				continue
			}
			walk(child)
			child = next
		}
		if n.Type != html.ElementNode {
			return
		}
		if _, ok := removeAttr(n, legacyLandingPageImage); ok {
			changed = true
		}
	}
	walk(legacyBody)
	var removeLegacyTitle func(*html.Node)
	removeLegacyTitle = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode && child.Data == "title" && textContent(child) == "Training simulation" {
				n.RemoveChild(child)
				changed = true
			} else {
				removeLegacyTitle(child)
			}
			child = next
		}
	}
	removeLegacyTitle(doc)
	// These pages were previously rendered literally. Preserve that contract
	// after retiring the special response mode by quoting delimiters only in
	// parsed text and attribute values. Placeholders prevent replacement from
	// recursively rewriting the template expressions introduced below.
	replaceLegacyDelimiters(doc, openDelimiter, closeDelimiter)
	if !changed {
		return source, false, nil
	}
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		return "", false, err
	}
	cleaned := strings.ReplaceAll(out.String(), openDelimiter, legacyLiteralOpenDelimiter)
	cleaned = strings.ReplaceAll(cleaned, closeDelimiter, legacyLiteralCloseDelimiter)
	return cleaned, true, nil
}

// cleanupLegacyLandingPages is an idempotent compatibility migration for
// landing pages created before v0.23.0. It runs after schema migrations and
// removes the retired mode metadata without deleting the imported page body.
func cleanupLegacyLandingPages(database *gorm.DB) error {
	var pages []Page
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("html LIKE ?", "%data-darkphish-training%").Find(&pages).Error; err != nil {
			return err
		}
		for _, page := range pages {
			cleaned, changed, err := stripLegacyLandingPageMarkup(page.HTML)
			if err != nil {
				return err
			}
			if changed {
				if err := tx.Model(&Page{}).Where("id = ?", page.Id).Update("html", cleaned).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
