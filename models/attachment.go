package models

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	MaxAttachmentEncodedSize   = 24 << 20
	MaxAttachmentDecodedSize   = 16 << 20
	MaxOfficeArchiveEntries    = 1024
	MaxOfficeArchiveMemberSize = 8 << 20
	MaxOfficeArchiveTotalSize  = 32 << 20
	MaxOfficeCompressionRatio  = 200
)

var (
	ErrAttachmentEncodedTooLarge = errors.New("attachment exceeds the maximum encoded size")
	ErrAttachmentDecodedTooLarge = errors.New("attachment exceeds the maximum decoded size")
	ErrOfficeTooManyEntries      = errors.New("Office attachment contains too many archive entries")
	ErrOfficeMemberTooLarge      = errors.New("Office attachment contains an oversized archive member")
	ErrOfficeArchiveTooLarge     = errors.New("Office attachment expands beyond the permitted total size")
	ErrOfficeCompressionRatio    = errors.New("Office attachment has a suspicious compression ratio")
	ErrInvalidOfficeArchive      = errors.New("Office attachment is not a valid ZIP archive")
)

// Attachment contains the fields and methods for
// an email attachment
type Attachment struct {
	Id          int64  `json:"-"`
	TemplateId  int64  `json:"-"`
	Content     string `json:"content"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	vanillaFile bool   // Vanilla file has no template variables
}

// Validate ensures that the provided attachment uses the supported template variables correctly.
func (a Attachment) Validate() error {
	vc := ValidationContext{
		FromAddress: "foo@bar.com",
		BaseURL:     "http://example.com",
	}
	td := Result{
		BaseRecipient: BaseRecipient{
			Email:     "foo@bar.com",
			FirstName: "Foo",
			LastName:  "Bar",
			Position:  "Test",
		},
		RId: "123456",
	}
	ptx, err := NewPhishingTemplateContext(vc, td.BaseRecipient, td.RId)
	if err != nil {
		return err
	}
	_, err = a.ApplyTemplate(ptx)
	return err
}

// ApplyTemplate parses different attachment files and applies the supplied phishing template.
func (a *Attachment) ApplyTemplate(ptx PhishingTemplateContext) (io.Reader, error) {
	decodedAttachment, err := decodeAttachment(a.Content)
	if err != nil {
		return nil, err
	}

	// If we've already determined there are no template variables in this attachment return it immediately
	if a.vanillaFile {
		return bytes.NewReader(decodedAttachment), nil
	}

	// Decided to use the file extension rather than the content type, as there seems to be quite
	//  a bit of variability with types. e.g sometimes a Word docx file would have:
	//   "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	fileExtension := filepath.Ext(a.Name)

	switch fileExtension {

	case ".docx", ".docm", ".pptx", ".xlsx", ".xlsm":
		// Most modern office formats are xml based and can be unarchived.
		// .docm and .xlsm files are comprised of xml, and a binary blob for the macro code

		// Zip archives require random access for reading, so it's hard to stream bytes. Solution seems to be to use a buffer.
		// See https://stackoverflow.com/questions/16946978/how-to-unzip-io-readcloser
		zipReader, err := zip.NewReader(bytes.NewReader(decodedAttachment), int64(len(decodedAttachment)))

		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidOfficeArchive, err)
		}
		if len(zipReader.File) > MaxOfficeArchiveEntries {
			return nil, fmt.Errorf("%w: got %d, limit %d", ErrOfficeTooManyEntries, len(zipReader.File), MaxOfficeArchiveEntries)
		}
		var totalUncompressed uint64
		for _, zipFile := range zipReader.File {
			if zipFile.UncompressedSize64 > MaxOfficeArchiveMemberSize {
				return nil, fmt.Errorf("%w: %q is %d bytes", ErrOfficeMemberTooLarge, zipFile.Name, zipFile.UncompressedSize64)
			}
			totalUncompressed += zipFile.UncompressedSize64
			if totalUncompressed > MaxOfficeArchiveTotalSize {
				return nil, fmt.Errorf("%w: expanded size exceeds %d bytes", ErrOfficeArchiveTooLarge, MaxOfficeArchiveTotalSize)
			}
			if zipFile.UncompressedSize64 >= 1<<20 {
				if zipFile.CompressedSize64 == 0 || zipFile.UncompressedSize64/zipFile.CompressedSize64 > MaxOfficeCompressionRatio {
					return nil, fmt.Errorf("%w: %q", ErrOfficeCompressionRatio, zipFile.Name)
				}
			}
		}

		newZipArchive := new(bytes.Buffer)
		zipWriter := zip.NewWriter(newZipArchive) // For writing the new archive

		// i. Read each file from the Word document archive
		// ii. Apply the template to it
		// iii. Add the templated content to a new zip Word archive
		a.vanillaFile = true
		for _, zipFile := range zipReader.File {
			ff, err := zipFile.Open()
			if err != nil {
				return nil, err
			}
			contents, readErr := io.ReadAll(io.LimitReader(ff, MaxOfficeArchiveMemberSize+1))
			closeErr := ff.Close()
			if readErr != nil {
				_ = zipWriter.Close()
				return nil, readErr
			}
			if closeErr != nil {
				_ = zipWriter.Close()
				return nil, closeErr
			}
			if len(contents) > MaxOfficeArchiveMemberSize {
				_ = zipWriter.Close()
				return nil, fmt.Errorf("%w: %q", ErrOfficeMemberTooLarge, zipFile.Name)
			}
			subFileExtension := filepath.Ext(zipFile.Name)
			var tFile string
			if subFileExtension == ".xml" || subFileExtension == ".rels" { // Ignore other files, e.g binary ones and images
				// First we look for instances where Word has URL escaped our template variables. This seems to happen when inserting a remote image, converting {{.Foo}} to %7b%7b.foo%7d%7d.
				// See https://stackoverflow.com/questions/68287630/disable-url-encoding-for-includepicture-in-microsoft-word
				rx, _ := regexp.Compile("%7b%7b.([a-zA-Z]+)%7d%7d")
				contents := rx.ReplaceAllFunc(contents, func(m []byte) []byte {
					d, err := url.QueryUnescape(string(m))
					if err != nil {
						return m
					}
					return []byte(d)
				})

				// For each file apply the template.
				tFile, err = ExecuteTemplate(string(contents), ptx)
				if err != nil {
					zipWriter.Close() // Don't use defer when writing files https://www.joeshaw.org/dont-defer-close-on-writable-files/
					return nil, err
				}
				// Check if the subfile changed. We only need this to be set once to know in the future to check the 'parent' file
				if tFile != string(contents) {
					a.vanillaFile = false
				}
			} else {
				tFile = string(contents) // Could move this to the declaration of tFile, but might be confusing to read
			}
			// Write new Word archive
			newZipFile, err := zipWriter.Create(zipFile.Name)
			if err != nil {
				zipWriter.Close() // Don't use defer when writing files https://www.joeshaw.org/dont-defer-close-on-writable-files/
				return nil, err
			}
			_, err = newZipFile.Write([]byte(tFile))
			if err != nil {
				zipWriter.Close()
				return nil, err
			}
		}
		if err = zipWriter.Close(); err != nil {
			return nil, err
		}
		if newZipArchive.Len() > MaxAttachmentDecodedSize {
			return nil, ErrAttachmentDecodedTooLarge
		}
		return bytes.NewReader(newZipArchive.Bytes()), nil

	case ".txt", ".html", ".ics":
		processedAttachment, err := ExecuteTemplate(string(decodedAttachment), ptx)
		if err != nil {
			return nil, err
		}
		if processedAttachment == string(decodedAttachment) {
			a.vanillaFile = true
		}
		return strings.NewReader(processedAttachment), nil
	default:
		return bytes.NewReader(decodedAttachment), nil // Default is to simply return the file
	}

}

func decodeAttachment(content string) ([]byte, error) {
	if len(content) > MaxAttachmentEncodedSize {
		return nil, ErrAttachmentEncodedTooLarge
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(content))
	decoded, err := io.ReadAll(io.LimitReader(decoder, MaxAttachmentDecodedSize+1))
	if err != nil {
		return nil, fmt.Errorf("invalid base64 attachment: %w", err)
	}
	if len(decoded) > MaxAttachmentDecodedSize {
		return nil, ErrAttachmentDecodedTooLarge
	}
	return decoded, nil
}
