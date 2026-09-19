package traininghtml

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestStaticTrainingRemovesActiveContent(t *testing.T) {
	base, _ := url.Parse("https://assets.example.test/training/page")
	input := `<style>body{background:url(https://track.example.test)}</style><script>alert(1)</script><form action="https://collect.example.test"><input type=password name=password value=SECRET><textarea>SECRET</textarea><select><option>SECRET</option></select><button formaction="https://collect.example.test">Submit</button></form><iframe srcdoc="SECRET"></iframe><svg><script>SECRET</script></svg><div onclick="SECRET" contenteditable="true" style="color:#123456;background-image:url(https://track.example.test);position:fixed;padding:20px">Training {{\example}} Žluťoučký kôň</div><a href="javascript:SECRET">Link</a><img src="../mark.png" onerror="SECRET"><img src="http://private.test/pixel">`
	got, err := Sanitize(input, base)
	if err != nil {
		t.Fatal(err)
	}
	if !IsStatic(got) {
		t.Fatal("training mode missing")
	}
	for _, blocked := range []string{"SECRET", "<form", "<input", "<textarea", "<select", "<button", "<script", "<iframe", "<svg", "<style", "onclick", "contenteditable", "background-image", "position:", "javascript:"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("active content survived: %s", blocked)
		}
	}
	for _, kept := range []string{Notice, "color:#123456", "padding:20px", `{{\example}}`, "Žluťoučký kôň", `data-training-image-url="https://assets.example.test/mark.png"`} {
		if !strings.Contains(got, kept) {
			t.Fatalf("missing safe content %q: %s", kept, got)
		}
	}
	if strings.Contains(got, `src="https:`) {
		t.Fatal("import must not initiate image fetches")
	}
	again, err := Sanitize(got, nil)
	if err != nil || !IsStatic(again) || strings.Count(again, Notice) != 1 {
		t.Fatal("round trip lost mode or duplicated notice")
	}
	if strings.Contains(again, "<input") {
		t.Fatal("round trip restored input")
	}
}

func TestStaticTrainingMarkerAndLimits(t *testing.T) {
	for _, s := range []string{"", `<!-- <body data-darkphish-training="static-v1"> -->`, `<script>"<body data-darkphish-training='static-v1'>"</script>`} {
		if IsStatic(s) {
			t.Fatal("marker in inert text accepted")
		}
	}
	if !IsStatic(`<BODY data-darkphish-training='static-v1'>safe</BODY>`) {
		t.Fatal("valid marker rejected")
	}
	if _, err := Sanitize(strings.Repeat("x", maxBytes+1), nil); err != ErrSize {
		t.Fatal("missing input limit")
	}
}

func TestStaticTrainingAttributeAllowlist(t *testing.T) {
	got, err := Sanitize(`<body><div style="color:expression(alert(1));font-family:url(x);width:var(--secret);height:attr(x);color:red" onfocus=x data-x=x id=x class=x><img src="data:image/svg+xml,evil" srcset="https://track.test 1x" /><math><mi>x</mi></math></div></body>`, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "on") || a.Key == "srcset" || a.Key == "id" || a.Key == "class" {
				t.Fatalf("unsafe attribute %s", a.Key)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if strings.Contains(got, "expression") || strings.Contains(got, "url(") || strings.Contains(got, "svg+xml") || strings.Contains(got, "<math") {
		t.Fatal(got)
	}
}
