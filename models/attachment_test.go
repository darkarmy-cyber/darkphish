package models

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/check.v1"
)

func (s *ModelsSuite) TestAttachment(c *check.C) {
	ptx := PhishingTemplateContext{
		BaseRecipient: BaseRecipient{
			FirstName: "Foo",
			LastName:  "Bar",
			Email:     "foo@bar.com",
			Position:  "Space Janitor",
		},
		BaseURL:     "http://testurl.com",
		URL:         "http://testurl.com/?rid=1234567",
		TrackingURL: "http://testurl.local/track?rid=1234567",
		Tracker:     "<img alt='' style='display: none' src='http://testurl.local/track?rid=1234567'/>",
		From:        "From Address",
		RId:         "1234567",
	}

	files, err := os.ReadDir("testdata")
	if err != nil {
		log.Fatalf("Failed to open attachment folder 'testdata': %v\n", err)
	}
	for _, ff := range files {
		if !ff.IsDir() && !strings.Contains(ff.Name(), "templated") {
			fname := ff.Name()
			fmt.Printf("Checking attachment file -> %s\n", fname)
			data := readFile("testdata/" + fname)
			if filepath.Ext(fname) == ".b64" {
				fname = fname[:len(fname)-4]
			}
			a := Attachment{
				Content: data,
				Name:    fname,
			}
			t, err := a.ApplyTemplate(ptx)
			c.Assert(err, check.Equals, nil)
			c.Assert(a.vanillaFile, check.Equals, strings.Contains(fname, "without-vars"))
			c.Assert(a.vanillaFile, check.Not(check.Equals), strings.Contains(fname, "with-vars"))

			// Verfify template was applied as expected
			tt, err := io.ReadAll(t)
			if err != nil {
				log.Fatalf("Failed to parse templated file '%s': %v\n", fname, err)
			}
			templatedFile := base64.StdEncoding.EncodeToString(tt)
			expectedOutput := readFile("testdata/" + strings.TrimSuffix(ff.Name(), filepath.Ext(ff.Name())) + ".templated" + filepath.Ext(ff.Name())) // e.g text-file-with-vars.templated.txt
			switch filepath.Ext(fname) {
			case ".docx", ".docm", ".pptx", ".xlsx", ".xlsm":
				expectedBytes, err := base64.StdEncoding.DecodeString(expectedOutput)
				c.Assert(err, check.IsNil)
				actualFiles, err := zipFileContents(tt)
				c.Assert(err, check.IsNil)
				expectedFiles, err := zipFileContents(expectedBytes)
				c.Assert(err, check.IsNil)
				c.Assert(actualFiles, check.DeepEquals, expectedFiles)
			default:
				c.Assert(templatedFile, check.Equals, expectedOutput)
			}
		}
	}
}

func zipFileContents(data []byte) (map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	contents := make(map[string][]byte, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		contents[file.Name] = body
	}
	return contents, nil
}

func readFile(fname string) string {
	content, err := os.ReadFile(fname)
	if err != nil {
		log.Fatalf("Failed to open file '%s': %v\n", fname, err)
	}
	data := ""
	if filepath.Ext(fname) == ".b64" {
		data = string(content)
	} else {
		data = base64.StdEncoding.EncodeToString(content)
	}
	return data
}
