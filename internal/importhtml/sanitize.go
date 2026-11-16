// Package importhtml sanitizes HTML downloaded by the site-import endpoint.
package importhtml

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
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

var allowedTags = words("html head body title link style div span p br hr h1 h2 h3 h4 h5 h6 strong b em i u s small sub sup blockquote pre code ul ol li dl dt dd table thead tbody tfoot tr td th caption colgroup col section article header footer main nav aside figure figcaption details summary a picture source img form label fieldset legend input textarea select option optgroup button svg g path circle ellipse rect line polyline polygon text tspan defs linearGradient radialGradient stop clipPath title desc symbol")
var discardedTags = words("script noscript meta base iframe frame frameset object embed math template audio video track canvas foreignObject use animate set animateMotion animateTransform image")
var dimension = regexp.MustCompile(`^[0-9]{1,4}%?$`)
var rasterData = regexp.MustCompile(`^data:image/(?:png|gif|jpeg|webp);base64,[A-Za-z0-9+/=]+$`)

// Class and id values are inert identifiers: they only need to be bounded and
// free of markup/control characters. Real-world framework class names include
// characters such as "[", "]", "=" and "#" which older token filtering broke.
var safeToken = regexp.MustCompile(`^[^\x00-\x1f\x7f<>]{1,1000}$`)

// Stylesheet sanitization runs lexically on top of a comment/import strip and
// resolves every url() reference through the same HTTPS-only policy as
// attribute URLs. Styles are passive presentation; active CSS (expression,
// behavior, -moz-binding) fails the whole block closed.
const maxStylesheetBytes = 1 << 20

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
var cssImport = regexp.MustCompile(`(?is)@import[^;{}]*;?`)
var cssURL = regexp.MustCompile(`(?is)url\(\s*('[^']*'|"[^"]*"|[^'")]+)\s*\)`)
var cssEscape = regexp.MustCompile(`\\([0-9a-fA-F]{1,6})\s?|\\(.)`)

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

func cssUnescape(css string) string {
	return cssEscape.ReplaceAllStringFunc(css, func(m string) string {
		sub := cssEscape.FindStringSubmatch(m)
		if sub[1] != "" {
			value, err := strconv.ParseUint(sub[1], 16, 32)
			if err != nil || value > 0x10FFFF || (value >= 0xD800 && value <= 0xDFFF) {
				return ""
			}
			return string(rune(value))
		}
		return sub[2]
	})
}

// hostileCSS reports active-content constructs that have no place in imported
// presentation styles.
func hostileCSS(css string) bool {
	lower := strings.ToLower(css)
	return strings.Contains(lower, "expression(") || strings.Contains(lower, "behavior:") ||
		strings.Contains(lower, "-moz-binding") || strings.Contains(lower, "-o-link") ||
		strings.Contains(lower, "@import")
}

// sanitizeCSSReference keeps embedded raster data and absolute HTTPS URLs,
// resolving relative references against the imported page.
func sanitizeCSSReference(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if rasterData.MatchString(raw) {
		return raw
	}
	if raw == "" || len(raw) > 4096 || strings.ContainsAny(raw, "\"'\\ \t\r\n{}") {
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

// splitDeclarations splits an inline style attribute on semicolons that are
// not nested inside parentheses, brackets or quoted strings, so values such as
// url(data:image/png;base64,...) survive intact.
func splitDeclarations(raw string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	var quote byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if quote != 0 {
			current.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}
		current.WriteByte(c)
	}
	parts = append(parts, current.String())
	return parts
}

// sanitizeStylesheet returns presentation-safe CSS for a <style> block, or an
// empty string when the block must be dropped entirely. CSS escapes are decoded
// first so no construct can hide behind them.
func sanitizeStylesheet(css string, base *url.URL) string {
	if len(css) > maxStylesheetBytes {
		return ""
	}
	css = cssUnescape(css)
	css = cssComment.ReplaceAllString(css, " ")
	css = cssImport.ReplaceAllString(css, " ")
	css = strings.TrimSpace(css)
	if css == "" {
		return ""
	}
	if hostileCSS(css) {
		return ""
	}
	css = cssURL.ReplaceAllStringFunc(css, func(m string) string {
		inner := cssURL.FindStringSubmatch(m)
		reference := strings.TrimSpace(inner[1])
		quote := ""
		if len(reference) >= 2 && (reference[0] == '\'' || reference[0] == '"') && reference[len(reference)-1] == reference[0] {
			quote = string(reference[0])
			reference = reference[1 : len(reference)-1]
		}
		if replacement := sanitizeCSSReference(reference, base); replacement != "" {
			return "url(" + quote + replacement + quote + ")"
		}
		return "url(none)"
	})
	return css
}

// safeStyle filters an inline style attribute. Unlike stylesheet blocks,
// inline url() references are limited to embedded raster data: remote URLs in
// arbitrary inline attributes are a classic tracking vector.
func safeStyle(raw string) string {
	if len(raw) > 2048 {
		return ""
	}
	raw = cssComment.ReplaceAllString(raw, " ")
	var declarations []string
	for _, item := range splitDeclarations(raw) {
		pair := strings.SplitN(item, ":", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		if key == "" {
			continue
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "expression(") || strings.Contains(lower, "behavior:") || strings.Contains(lower, "-moz-binding") {
			continue
		}
		if strings.Contains(lower, "url(") || strings.Contains(lower, "src(") {
			onlyRasters := true
			for _, match := range cssURL.FindAllStringSubmatch(value, -1) {
				reference := strings.TrimSpace(match[1])
				if len(reference) >= 2 && (reference[0] == '\'' || reference[0] == '"') && reference[len(reference)-1] == reference[0] {
					reference = reference[1 : len(reference)-1]
				}
				if !rasterData.MatchString(reference) {
					onlyRasters = false
					break
				}
			}
			if strings.Contains(value, "\\") && strings.Contains(cssUnescape(value), "url(") {
				onlyRasters = false
			}
			if !onlyRasters {
				continue
			}
		}
		declarations = append(declarations, key+":"+value)
	}
	return strings.Join(declarations, ";")
}

// safeSrcset validates every image candidate of a srcset attribute and keeps
// only HTTPS or embedded-raster references.
func safeSrcset(raw string, base *url.URL) string {
	if len(raw) > 8192 {
		return ""
	}
	var kept []string
	for _, candidate := range strings.Split(raw, ",") {
		fields := strings.Fields(candidate)
		if len(fields) == 0 {
			continue
		}
		resolved := safeURL(fields[0], base, true)
		if resolved == "" {
			continue
		}
		kept = append(kept, strings.Join(append([]string{resolved}, fields[1:]...), " "))
	}
	return strings.Join(kept, ", ")
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
		if a.Namespace != "" || strings.HasPrefix(a.Key, "on") || a.Key == "formaction" {
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
			if safeToken.MatchString(a.Val) {
				out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
			}
		case "viewBox", "preserveAspectRatio", "gradientUnits", "gradientTransform", "transform", "clip-path", "clip-rule", "fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity", "stroke-linecap", "stroke-linejoin", "stroke-dasharray", "stroke-dashoffset", "stroke-miterlimit", "stop-color", "stop-opacity", "offset", "text-anchor", "dominant-baseline", "dx", "dy", "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry", "points":
			if isSVGElement(out.Data) {
				appendBounded(out, a.Key, a.Val, 512)
			}
		case "d":
			if isSVGElement(out.Data) {
				appendBounded(out, a.Key, a.Val, 8192)
			}
		case "font-size", "font-family", "font-weight", "letter-spacing", "version", "xmlns":
			if isSVGElement(out.Data) {
				appendBounded(out, a.Key, a.Val, 256)
			}
		case "srcset":
			if out.Data == "img" || out.Data == "source" {
				if value := safeSrcset(a.Val, base); value != "" {
					out.Attr = append(out.Attr, html.Attribute{Key: "srcset", Val: value})
				}
			}
		case "width", "height", "colspan", "rowspan", "size", "maxlength", "minlength":
			if dimension.MatchString(a.Val) {
				out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
			}
		case "lang", "dir", "autocomplete", "media", "as":
			appendBounded(out, a.Key, a.Val, 64)
		case "rel":
			if out.Data == "link" {
				rel := strings.ToLower(strings.TrimSpace(a.Val))
				as := strings.ToLower(strings.TrimSpace(attr(source, "as")))
				if rel == "stylesheet" || (rel == "preload" && as == "style") {
					out.Attr = append(out.Attr, html.Attribute{Key: "rel", Val: "stylesheet"})
				}
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

func isSVGElement(tag string) bool {
	switch tag {
	case "svg", "g", "path", "circle", "ellipse", "rect", "line", "polyline", "polygon", "text", "tspan", "defs", "linearGradient", "radialGradient", "stop", "clipPath", "title", "desc", "symbol":
		return true
	}
	return false
}

func styleText(n *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return builder.String()
}

func clean(n *html.Node, base *url.URL) *html.Node {
	switch n.Type {
	case html.TextNode:
		return &html.Node{Type: html.TextNode, Data: n.Data}
	case html.ElementNode:
		// The HTML parser assigns the svg namespace to inline SVG subtrees;
		// allow it for the curated passive vector-art subset.
		if n.Namespace != "" && !(n.Namespace == "svg" && isSVGElement(n.Data)) {
			return nil
		}
		if discardedTags[n.Data] {
			return nil
		}
		tag := n.Data
		if tag == "link" {
			rel := strings.ToLower(strings.TrimSpace(attr(n, "rel")))
			as := strings.ToLower(strings.TrimSpace(attr(n, "as")))
			if rel != "stylesheet" && !(rel == "preload" && as == "style") {
				return nil
			}
		}
		if tag == "style" {
			css := sanitizeStylesheet(styleText(n), base)
			if css == "" {
				return nil
			}
			out := &html.Node{Type: html.ElementNode, Data: "style"}
			out.AppendChild(&html.Node{Type: html.TextNode, Data: css})
			return out
		}
		if !allowedTags[tag] {
			tag = "div"
		}
		out := &html.Node{Type: html.ElementNode, Data: tag, Namespace: n.Namespace}
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
			if child.Type == html.ElementNode && (child.Data == "link" || child.Data == "style") {
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
