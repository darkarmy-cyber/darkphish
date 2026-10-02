package importhtml

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSanitizeKeepsFormsAndRemovesActiveContent(t *testing.T) {
	base, _ := url.Parse("https://assets.example.test/account/page")
	input := `<body data-darkphish-training="static-v1"><p data-training-notice="true">obsolete</p><script>alert(1)</script><form action="https://collect.example.test" onsubmit="steal()"><label for="login">Login</label><input id="login" name="username" value="alice" onclick="steal()"><input type="password" name="password"><textarea name="note">hello</textarea><select name="kind"><option value="a" selected>A</option></select><button formaction="https://collect.example.test">Continue</button></form><a href="javascript:alert(1)">bad</a><img src="../logo.png" onerror="steal()"><div style="color:red;background-image:url(https://track.example.test)">content</div></body>`
	got, err := Sanitize(input, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []string{"data-darkphish-training", "data-training-notice", "<script", "onsubmit", "onclick", "onerror", "formaction", "javascript:", "background-image", "https://collect.example.test"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("unsafe content survived: %s\n%s", blocked, got)
		}
	}
	for _, kept := range []string{`<form action="" method="post">`, `name="username"`, `value="alice"`, `type="password"`, `<textarea name="note">hello</textarea>`, `<select name="kind">`, `<button>Continue</button>`, `style="color:red"`} {
		if !strings.Contains(got, kept) {
			t.Fatalf("expected content missing: %s\n%s", kept, got)
		}
	}
	if strings.Contains(got, "assets.example.test/logo.png") {
		t.Fatal("remote image URL survived import")
	}
}

func TestSanitizeRejectsDangerousAttributesAndBoundsInput(t *testing.T) {
	got, err := Sanitize(`<body><iframe srcdoc="x"></iframe><svg><script>x</script></svg><input type="file" name="upload"><img src="data:image/svg+xml,evil"><img src="data:image/png;base64,aGVsbG8="></body>`, nil)
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
			if strings.HasPrefix(a.Key, "on") || a.Key == "srcdoc" || a.Key == "formaction" || a.Key == "srcset" {
				t.Fatalf("unsafe attribute survived: %s", a.Key)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if strings.Contains(got, "<iframe") || strings.Contains(got, "<svg") || strings.Contains(got, "svg+xml") || !strings.Contains(got, `type="text"`) || !strings.Contains(got, `data:image/png;base64,aGVsbG8=`) {
		t.Fatal(got)
	}
	if _, err := Sanitize(strings.Repeat("x", maxBytes+1), nil); err != ErrSize {
		t.Fatal("missing input limit")
	}
}
