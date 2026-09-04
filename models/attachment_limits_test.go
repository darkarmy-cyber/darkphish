package models

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type zipTestEntry struct {
	name   string
	data   []byte
	method uint16
}

func encodedTestArchive(t *testing.T, entries []zipTestEntry) string {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: entry.method}
		member, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = member.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(archive.Bytes())
}

func archiveWithDeclaredSizes(t *testing.T, entryCount int, compressedSize, uncompressedSize uint32) string {
	t.Helper()
	entries := make([]zipTestEntry, entryCount)
	for i := range entries {
		entries[i] = zipTestEntry{name: fmt.Sprintf("word/member-%d.xml", i), method: zip.Store}
	}
	raw, err := base64.StdEncoding.DecodeString(encodedTestArchive(t, entries))
	if err != nil {
		t.Fatal(err)
	}
	signature := []byte{'P', 'K', 1, 2}
	patched := 0
	for offset := 0; offset+28 <= len(raw); offset++ {
		if !bytes.Equal(raw[offset:offset+4], signature) {
			continue
		}
		binary.LittleEndian.PutUint32(raw[offset+20:offset+24], compressedSize)
		binary.LittleEndian.PutUint32(raw[offset+24:offset+28], uncompressedSize)
		patched++
	}
	if patched != entryCount {
		t.Fatalf("patched %d central-directory records, expected %d", patched, entryCount)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func testTemplateContext() PhishingTemplateContext {
	return PhishingTemplateContext{BaseRecipient: BaseRecipient{FirstName: "Ada", Email: "ada@example.test"}}
}

func TestOfficeAttachmentLimits(t *testing.T) {
	t.Run("valid docx", func(t *testing.T) {
		attachment := Attachment{
			Name: "awareness.docx",
			Content: encodedTestArchive(t, []zipTestEntry{{
				name:   "word/document.xml",
				data:   []byte(`<w:t>Hello {{.FirstName}}</w:t>`),
				method: zip.Deflate,
			}}),
		}
		reader, err := attachment.ApplyTemplate(testTemplateContext())
		if err != nil {
			t.Fatal(err)
		}
		result, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
		if err != nil {
			t.Fatal(err)
		}
		member, err := zr.File[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer member.Close()
		contents, err := io.ReadAll(member)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "Hello Ada") {
			t.Fatalf("template was not applied: %s", contents)
		}
	})

	t.Run("oversized encoded file", func(t *testing.T) {
		attachment := Attachment{Name: "note.txt", Content: strings.Repeat("A", MaxAttachmentEncodedSize+1)}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrAttachmentEncodedTooLarge) {
			t.Fatalf("expected %v, got %v", ErrAttachmentEncodedTooLarge, err)
		}
	})

	t.Run("oversized decoded file", func(t *testing.T) {
		content := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, MaxAttachmentDecodedSize+1))
		attachment := Attachment{Name: "note.txt", Content: content}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrAttachmentDecodedTooLarge) {
			t.Fatalf("expected %v, got %v", ErrAttachmentDecodedTooLarge, err)
		}
	})

	t.Run("oversized zip member", func(t *testing.T) {
		content := encodedTestArchive(t, []zipTestEntry{{
			name:   "word/document.xml",
			data:   bytes.Repeat([]byte{'x'}, MaxOfficeArchiveMemberSize+1),
			method: zip.Store,
		}})
		attachment := Attachment{Name: "large.docx", Content: content}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrOfficeMemberTooLarge) {
			t.Fatalf("expected %v, got %v", ErrOfficeMemberTooLarge, err)
		}
	})

	t.Run("too many entries", func(t *testing.T) {
		entries := make([]zipTestEntry, MaxOfficeArchiveEntries+1)
		for i := range entries {
			entries[i] = zipTestEntry{name: fmt.Sprintf("word/member-%04d.xml", i), method: zip.Store}
		}
		attachment := Attachment{Name: "many.docx", Content: encodedTestArchive(t, entries)}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrOfficeTooManyEntries) {
			t.Fatalf("expected %v, got %v", ErrOfficeTooManyEntries, err)
		}
	})

	t.Run("excessive total expanded size", func(t *testing.T) {
		content := archiveWithDeclaredSizes(t, 5, 1<<20, 7<<20)
		attachment := Attachment{Name: "total.docx", Content: content}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrOfficeArchiveTooLarge) {
			t.Fatalf("expected %v, got %v", ErrOfficeArchiveTooLarge, err)
		}
	})

	t.Run("decompression expansion", func(t *testing.T) {
		content := encodedTestArchive(t, []zipTestEntry{{
			name:   "word/document.xml",
			data:   bytes.Repeat([]byte{'A'}, 2<<20),
			method: zip.Deflate,
		}})
		attachment := Attachment{Name: "bomb.docx", Content: content}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrOfficeCompressionRatio) {
			t.Fatalf("expected %v, got %v", ErrOfficeCompressionRatio, err)
		}
	})

	t.Run("malformed zip", func(t *testing.T) {
		attachment := Attachment{Name: "broken.docx", Content: base64.StdEncoding.EncodeToString([]byte("not a zip"))}
		_, err := attachment.ApplyTemplate(testTemplateContext())
		if !errors.Is(err, ErrInvalidOfficeArchive) {
			t.Fatalf("expected %v, got %v", ErrInvalidOfficeArchive, err)
		}
	})
}
