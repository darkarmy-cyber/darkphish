// Package traininghtml produces inert, visibly labelled training material.
// Imported content is never interpreted as a Go template or executed as script.
package traininghtml

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const Marker = "data-darkphish-training"
const Mode = "static-v1"
const Notice = "Training simulation — do not enter real credentials. Forms and scripts are disabled."
const maxBytes = 8 << 20

var ErrSize = errors.New("training HTML exceeds the 8 MiB limit")
var allowedTags = words("html head body title div span p br hr h1 h2 h3 h4 h5 h6 strong b em i u s small sub sup blockquote pre code ul ol li dl dt dd table thead tbody tfoot tr td th caption colgroup col section article header footer main nav aside figure figcaption details summary img")
var discardedTags = words("script style link meta base iframe frame frameset object embed svg math template noscript audio video source track canvas")
var controls = words("input textarea select button")
var styleProperties = words("color background-color font-size font-family font-weight font-style line-height text-align text-decoration letter-spacing white-space word-break overflow-wrap margin margin-top margin-right margin-bottom margin-left padding padding-top padding-right padding-bottom padding-left border border-top border-right border-bottom border-left border-color border-width border-style border-radius width max-width min-width height max-height min-height display vertical-align table-layout border-collapse border-spacing opacity")
var safeStyleValue = regexp.MustCompile("^[a-zA-Z0-9#.,% ()'\"+/-]+$")
var dimension = regexp.MustCompile("^[0-9]{1,4}%?$")
var pngData = regexp.MustCompile("^data:image/png;base64,[A-Za-z0-9+/=]+$")

func words(s string) map[string]bool {
	m := make(map[string]bool)
	for _, word := range strings.Fields(s) {
		m[word] = true
	}
	return m
}

// IsStatic reads an HTML marker, not executable/template source text.
func IsStatic(source string) bool {
	z := html.NewTokenizer(strings.NewReader(source))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken:
			t := z.Token()
			if t.Data != "body" {
				continue
			}
			for _, a := range t.Attr {
				if a.Key == Marker && a.Val == Mode {
					return true
				}
			}
		}
	}
}

func safeStyle(raw string) string {
	var declarations []string
	for _, item := range strings.Split(raw, ";") {
		pair := strings.SplitN(item, ":", 2)
		if len(pair) != 2 {
			continue
		}
		key, val := strings.ToLower(strings.TrimSpace(pair[0])), strings.TrimSpace(pair[1])
		lower := strings.ToLower(val)
		if !styleProperties[key] || len(val) > 256 || !safeStyleValue.MatchString(val) {
			continue
		}
		if strings.Contains(lower, "url") || strings.Contains(lower, "expression") || strings.Contains(lower, "var(") || strings.Contains(lower, "attr(") {
			continue
		}
		if strings.HasPrefix(key, "margin") && strings.Contains(val, "-") {
			continue
		}
		// Background images, imports, custom properties and positioning are
		// deliberately unavailable; imported CSS cannot fetch or cover the notice.
		declarations = append(declarations, key+":"+val)
	}
	return strings.Join(declarations, ";")
}

func imageURL(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 4096 || strings.ContainsAny(raw, "{}\\\r\n\t ") {
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

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func clean(n *html.Node, base *url.URL) *html.Node {
	switch n.Type {
	case html.TextNode:
		return &html.Node{Type: html.TextNode, Data: n.Data}
	case html.ElementNode:
		if n.Namespace != "" || discardedTags[n.Data] {
			return nil
		}
		if controls[n.Data] {
			// No editable element, field name, value, autofill or form owner survives.
			out := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{{Key: "aria-disabled", Val: "true"}, {Key: "style", Val: "display:inline-block;padding:12px;border:1px solid #aaa;border-radius:4px;background-color:#f5f5f5;color:#34495e"}}}
			label := "Training input (disabled)"
			if n.Data == "button" {
				label = "Training action (disabled)"
			}
			out.AppendChild(&html.Node{Type: html.TextNode, Data: label})
			return out
		}
		tag := n.Data
		if !allowedTags[tag] {
			tag = "div"
		} // includes forms and links
		out := &html.Node{Type: html.ElementNode, Data: tag}
		for _, a := range n.Attr {
			if a.Namespace != "" {
				continue
			}
			switch a.Key {
			case "style":
				if val := safeStyle(a.Val); val != "" {
					out.Attr = append(out.Attr, html.Attribute{Key: "style", Val: val})
				}
			case "title", "alt":
				if len(a.Val) <= 512 {
					out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
				}
			case "width", "height", "colspan", "rowspan":
				if dimension.MatchString(a.Val) {
					out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
				}
			case "lang", "dir":
				if len(a.Val) <= 32 {
					out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: a.Val})
				}
			}
		}
		if tag == "img" {
			src := attr(n, "src")
			if len(src) <= 2<<20 && pngData.MatchString(src) {
				out.Attr = append(out.Attr, html.Attribute{Key: "src", Val: src})
			}
			remote := attr(n, "data-training-image-url")
			if remote == "" {
				remote = src
			}
			if target := imageURL(remote, base); target != "" {
				// Never fetch during import/render. The explicit preview action
				// uses the existing authenticated, bounded raster-image verifier.
				out.Attr = append(out.Attr, html.Attribute{Key: "data-training-image-url", Val: target})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if child := clean(c, base); child != nil {
				out.AppendChild(child)
			}
		}
		return out
	}
	return nil
}

// Sanitize is a deliberately limited static projection, not a live site clone.
// It never makes a network request, retains field values or executes templates.
func Sanitize(source string, base *url.URL) (string, error) {
	if len(source) > maxBytes {
		return "", ErrSize
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var sourceBody *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			sourceBody = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	body := &html.Node{Type: html.ElementNode, Data: "body", Attr: []html.Attribute{{Key: Marker, Val: Mode}}}
	// This trusted layer is regenerated on every save/render. Imported content
	// cannot acquire positioning or a z-index, even via nested stacking contexts.
	notice := &html.Node{Type: html.ElementNode, Data: "p", Attr: []html.Attribute{{Key: "data-training-notice", Val: "true"}, {Key: "style", Val: "position:relative;z-index:2147483647;display:block;padding:12px;margin:0;background-color:#fff;color:#243746;font-size:16px;line-height:1.5;opacity:1"}}}
	notice.AppendChild(&html.Node{Type: html.TextNode, Data: Notice})
	body.AppendChild(notice)
	if sourceBody != nil {
		for c := sourceBody.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && attr(c, "data-training-notice") == "true" {
				continue
			}
			if child := clean(c, base); child != nil {
				body.AppendChild(child)
			}
		}
	}
	var out bytes.Buffer
	_, _ = io.WriteString(&out, "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Training simulation</title></head>")
	if err = html.Render(&out, body); err != nil {
		return "", err
	}
	_, _ = io.WriteString(&out, "</html>")
	if out.Len() > maxBytes {
		return "", ErrSize
	}
	return out.String(), nil
}
