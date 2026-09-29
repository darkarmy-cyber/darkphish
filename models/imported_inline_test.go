package models

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/gophish/gomail"
)

func TestImportedInlineAttachmentIsSentAsCIDResource(t *testing.T) {
	previousConfig := conf
	if conf == nil {
		conf = &config.Config{}
	}
	defer func() { conf = previousConfig }()

	const name = "inline-test-image.png"
	req := EmailRequest{
		Template: Template{
			Name:    "Imported template",
			Subject: "CID test",
			HTML:    `<html><body><img src="cid:` + name + `"></body></html>`,
			Attachments: []Attachment{{
				Name:    name,
				Type:    "image/png",
				Content: base64.StdEncoding.EncodeToString([]byte("synthetic-image-bytes")),
			}},
		},
		SMTP:        SMTP{FromAddress: "sender@example.com"},
		URL:         "https://example.test/",
		RId:         PreviewPrefix + "cid-resource-test",
		FromAddress: "sender@example.com",
		BaseRecipient: BaseRecipient{
			Email:     "recipient@example.com",
			FirstName: "Recipient",
			LastName:  "Example",
		},
	}

	message := gomail.NewMessage()
	if err := req.Generate(message); err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	if _, err := message.WriteTo(&raw); err != nil {
		t.Fatal(err)
	}
	body := raw.String()
	for _, required := range []string{
		`src="cid:` + name + `"`,
		"Content-Disposition: inline; filename=\"" + name + "\"",
		"Content-ID: <" + name + ">",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("generated email is missing %q", required)
		}
	}
}
