// Package importhtml sanitizes HTML downloaded by the site-import endpoint.
package importhtml

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const maxBytes = 8 << 20

// recipientParameter is reserved for DarkPhish result routing and must never
// be supplied by imported page controls.
const recipientParameter = "rid"
const literalOpenDelimiter = "{{`{{`}}"
const literalCloseDelimiter = "{{`}`}}{{`}`}}"

var ErrSize = errors.New("imported HTML exceeds the 8 MiB limit")

var allowedTags = words("html head body title link style noscript div span p br hr h1 h2 h3 h4 h5 h6 strong b em i u s small sub sup blockquote pre code ul ol li dl dt dd table thead tbody tfoot tr td th caption colgroup col section article header footer main nav aside figure figcaption details summary a picture source img form label fieldset legend input textarea select option optgroup button")
var discardedTags = words("script meta base iframe frame frameset object embed svg math template audio video track canvas")
var styleProperties = words("color background-color font-size font-family font-weight font-style line-height text-align text-decoration letter-spacing white-space word-break overflow-wrap margin margin-top margin-right margin-bottom margin-left padding padding-top padding-right padding-bottom padding-left border border-top border-right border-bottom border-left border-color border-width border-style border-radius width max-width min-width height max-height min-height display vertical-align table-layout border-collapse border-spacing opacity")
var safeStyleValue = regexp.MustCompile(`^[a-zA-Z0-9#.,% ()'"+/-]+$`)
var tokenValue = regexp.MustCompile(`^[a-zA-Z0-9_:. -]{1,512}$`)
var dimension = regexp.MustCompile(`^[0-9]{1,4}%?$`)
var rasterData = regexp.MustCompile(`^data:image/(?:png|gif|jpeg|webp);base64,[A-Za-z0-9+/=]+$`)

func words(s string) map[string]bool {
	m := make(map[string]bool)
	for _, word := range strings.Fields(s) {
		m[word] = true
	}
	return m
}

func nodeContains(n *html.Node, value string) bool {
	if strings.Contains(n.Data, value) {
		return true
	}
	for _, attribute := range n.Attr {
		if strings.Contains(attribute.Val, value) {
			return true
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if nodeContains(child, value) {
			return true
		}
	}
	return false
}

func delimiterTokens(doc *html.Node) (string, string) {
	open := "DARKPHISHIMPORTOPENDELIMITER"
	close := "DARKPHISHIMPORTCLOSEDELIMITER"
	for nodeContains(doc, open) || nodeContains(doc, close) {
		open += "X"
		close += "X"
	}
	return open, close
}

func neutralizeDelimiters(n *html.Node, open, close string) {
	if n.Type == html.TextNode {
		n.Data = strings.ReplaceAll(strings.ReplaceAll(n.Data, "{{", open), "}}", close)
	}
	if n.Type == html.ElementNode {
		for i := range n.Attr {
			n.Attr[i].Val = strings.ReplaceAll(strings.ReplaceAll(n.Attr[i].Val, "{{", open), "}}", close)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		neutralizeDelimiters(child, open, close)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func safeStyle(raw string) string {
	var declarations []string
	for _, item := range strings.Split(raw, ";") {
		pair := strings.SplitN(item, ":", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(pair[0])), strings.TrimSpace(pair[1])
		lower := strings.ToLower(value)
		if !styleProperties[key] || len(value) > 256 || !safeStyleValue.MatchString(value) {
			continue
		}
		if strings.Contains(lower, "url") || strings.Contains(lower, "expression") || strings.Contains(lower, "var(") || strings.Contains(lower, "attr(") {
			continue
		}
		declarations = append(declarations, key+":"+value)
	}
	return strings.Join(declarations, ";")
}

func safeURL(raw string, base *url.URL, image bool) string {
	raw = strings.TrimSpace(raw)
	if image && len(raw) <= 2<<20 && rasterData.MatchString(raw) {
		return raw
	}
	if strings.HasPrefix(raw, "#") && len(raw) <= 512 && !image {
		return raw
	}
	if raw == "" || len(raw) > 4096 || strings.ContainsAny(raw, "{}\\\r\n\t ") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func appendBounded(out *html.Node, key, value string, limit int) {
	if len(value) <= limit {
		out.Attr = append(out.Attr, html.Attribute{Key: key, Val: value})
	}
}

func copyAttributes(out, source *html.Node, base *url.URL) {
	for _, a := range source.Attr {
		if a.Namespace != "" || strings.HasPrefix(a.Key, "on") || a.Key == "formaction" || a.Key == "srcset" {
			continue
		}
		switch a.Key {
		case "style":
			if value := safeStyle(a.Val); value != "" {
				out.Attr = append(out.Attr, html.Attribute{Key: "style", Val: value})
			}
		case "title", "alt", "placeholder", "aria-label":
			appendBounded(out, a.Key, a.Val, 512)
		case "id", "class", "for", "role":
			if tokenValue.MatchString(a.Val) {
				out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
			}
		case "width", "height", "colspan", "rowspan", "size", "maxlength", "minlength":
			if dimension.MatchString(a.Val) {
				out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
			}
		case "lang", "dir", "autocomplete", "media":
			appendBounded(out, a.Key, a.Val, 64)
		case "rel":
			if out.Data == "link" && strings.EqualFold(strings.TrimSpace(a.Val), "stylesheet") {
				out.Attr = append(out.Attr, html.Attribute{Key: "rel", Val: "stylesheet"})
			}
		case "name", "value", "type", "min", "max", "step", "pattern":
			if out.Data == "input" || out.Data == "textarea" || out.Data == "select" || out.Data == "option" || out.Data == "optgroup" || out.Data == "button" {
				if a.Key == "name" && strings.EqualFold(strings.TrimSpace(a.Val), recipientParameter) {
					continue
				}
				appendBounded(out, a.Key, a.Val, 1024)
			}
		case "checked", "selected", "disabled", "readonly", "required", "multiple":
			out.Attr = append(out.Attr, html.Attribute{Key: a.Key})
		case "href":
			if out.Data == "a" || out.Data == "link" {
				if value := safeURL(a.Val, base, false); value != "" {
					out.Attr = append(out.Attr, html.Attribute{Key: "href", Val: value})
				}
			}
		case "src":
			if out.Data == "img" {
				if value := safeURL(a.Val, base, true); value != "" {
					out.Attr = append(out.Attr, html.Attribute{Key: "src", Val: value})
				}
			}
		}
	}
	if out.Data == "form" {
		out.Attr = append(out.Attr, html.Attribute{Key: "action", Val: ""}, html.Attribute{Key: "method", Val: "post"})
	}
	if out.Data == "input" && strings.EqualFold(attr(out, "type"), "file") {
		for i := range out.Attr {
			if out.Attr[i].Key == "type" {
				out.Attr[i].Val = "text"
			}
		}
	}
}

func clean(n *html.Node, base *url.URL) *html.Node {
	switch n.Type {
	case html.TextNode:
		return &html.Node{Type: html.TextNode, Data: n.Data}
	case html.ElementNode:
		if n.Namespace != "" || discardedTags[n.Data] {
			return nil
		}
		tag := n.Data
		if tag == "link" && !strings.EqualFold(strings.TrimSpace(attr(n, "rel")), "stylesheet") {
			return nil
		}
		if !allowedTags[tag] {
			tag = "div"
		}
		out := &html.Node{Type: html.ElementNode, Data: tag}
		copyAttributes(out, n, base)
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if copied := clean(child, base); copied != nil {
				out.AppendChild(copied)
			}
		}
		return out
	}
	return nil
}

// Sanitize returns a bounded HTML document with active content and dangerous
// URLs removed. Forms and ordinary controls remain usable; their submissions
// are forced back to the DarkPhish landing-page endpoint when the page is saved.
func Sanitize(source string, base *url.URL) (string, error) {
	if len(source) > maxBytes {
		return "", ErrSize
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var sourceHead, sourceBody *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if sourceHead == nil && n.Data == "head" {
				sourceHead = n
			}
			if sourceBody == nil && n.Data == "body" {
				sourceBody = n
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(doc)
	openDelimiter, closeDelimiter := delimiterTokens(doc)
	var headResources []*html.Node
	if sourceHead != nil {
		for child := sourceHead.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && (child.Data == "link" || child.Data == "style" || child.Data == "noscript") {
				if copied := clean(child, base); copied != nil {
					headResources = append(headResources, copied)
				}
			}
		}
	}
	body := &html.Node{Type: html.ElementNode, Data: "body"}
	if sourceBody != nil {
		copyAttributes(body, sourceBody, base)
		for child := sourceBody.FirstChild; child != nil; child = child.NextSibling {
			if copied := clean(child, base); copied != nil {
				body.AppendChild(copied)
			}
		}
	}
	neutralizeDelimiters(body, openDelimiter, closeDelimiter)
	for _, resource := range headResources {
		neutralizeDelimiters(resource, openDelimiter, closeDelimiter)
	}
	var out bytes.Buffer
	_, _ = io.WriteString(&out, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Imported landing page</title>`)
	for _, resource := range headResources {
		if err := html.Render(&out, resource); err != nil {
			return "", err
		}
	}
	_, _ = io.WriteString(&out, "</head>")
	if err := html.Render(&out, body); err != nil {
		return "", err
	}
	_, _ = io.WriteString(&out, "</html>")
	cleaned := strings.ReplaceAll(out.String(), openDelimiter, literalOpenDelimiter)
	cleaned = strings.ReplaceAll(cleaned, closeDelimiter, literalCloseDelimiter)
	if len(cleaned) > maxBytes {
		return "", ErrSize
	}
	return cleaned, nil
}
