package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	messagemail "github.com/emersion/go-message/mail"

	"github.com/darkarmy-cyber/darkphish/models"
)

const (
	maxImportedEmailBytes       = 48 << 20
	maxImportedEmailParts       = 256
	maxImportedEmailAssets      = 48
	maxImportedEmailAssetTotal  = 32 << 20
	importedImageFetchTimeout   = 30 * time.Second
)

var emailCSSURL = regexp.MustCompile(`(?i)url\(\s*['"]?([^'")]+)['"]?\s*\)`)

type importedEmailAssets struct {
	Attachments []models.Attachment
	CIDNames    map[string]string
	Warnings    []string
}

type emailAssetLocalizer struct {
	ctx         context.Context
	attachments []models.Attachment
	cidNames    map[string]string
	cached      map[string]string
	usedNames   map[string]bool
	totalBytes  int
	warnings    map[string]bool
	fetchImage  func(context.Context, string) (string, error)
}

func normalizeContentID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	return strings.TrimSpace(value)
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

func sanitizeImportedAttachmentName(name, mediaType string, content []byte, used map[string]bool) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "/" || name == "" {
		sum := sha256.Sum256(content)
		name = "imported-" + hex.EncodeToString(sum[:8]) + extensionForMediaType(mediaType)
	}
	var cleaned strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			cleaned.WriteRune('_')
			continue
		}
		cleaned.WriteRune(r)
	}
	name = strings.TrimSpace(cleaned.String())
	if name == "" {
		name = "imported-file"
	}
	base, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	used[candidate] = true
	return candidate
}

func generatedInlineName(prefix, mediaType string, content []byte, used map[string]bool) string {
	sum := sha256.Sum256(content)
	ext := extensionForMediaType(mediaType)
	if ext == "" {
		ext = ".bin"
	}
	base := prefix + "-" + hex.EncodeToString(sum[:8]) + ext
	if !used[base] {
		used[base] = true
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s-%d%s", strings.TrimSuffix(base, ext), i, ext)
		if !used[name] {
			used[name] = true
			return name
		}
	}
}

func readImportedPart(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, models.MaxAttachmentDecodedSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > models.MaxAttachmentDecodedSize {
		return nil, models.ErrAttachmentDecodedTooLarge
	}
	return data, nil
}

func collectImportedEmailAssets(raw string) importedEmailAssets {
	result := importedEmailAssets{CIDNames: map[string]string{}}
	reader, err := messagemail.CreateReader(strings.NewReader(raw))
	if reader == nil {
		result.Warnings = append(result.Warnings, "Embedded MIME resources could not be parsed.")
		return result
	}
	defer reader.Close()
	if err != nil {
		result.Warnings = append(result.Warnings, "Some MIME character encodings could not be decoded.")
	}

	used := map[string]bool{}
	total := 0
	for partCount := 0; partCount < maxImportedEmailParts; partCount++ {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if part == nil {
			result.Warnings = append(result.Warnings, "Some MIME parts could not be imported.")
			break
		}
		contentType, _, parseErr := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if parseErr != nil || contentType == "" {
			contentType = "application/octet-stream"
		}
		disposition, dispositionParams, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		filename := dispositionParams["filename"]
		if filename == "" {
			_, params, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			filename = params["name"]
		}
		cid := normalizeContentID(part.Header.Get("Content-ID"))

		isBody := (contentType == "text/plain" || contentType == "text/html") && disposition != "attachment" && cid == ""
		if isBody {
			// The legacy email parser remains authoritative for body selection.
			// Drain this part so the streaming MIME reader can advance safely.
			_, _ = io.Copy(io.Discard, io.LimitReader(part.Body, models.MaxAttachmentDecodedSize+1))
			continue
		}

		data, readErr := readImportedPart(part.Body)
		if readErr != nil {
			result.Warnings = append(result.Warnings, "One or more oversized MIME resources were skipped.")
			continue
		}
		if len(data) == 0 {
			continue
		}
		if total+len(data) > maxImportedEmailAssetTotal || len(result.Attachments) >= maxImportedEmailAssets {
			result.Warnings = append(result.Warnings, "The email contains more embedded resources than can be imported safely.")
			break
		}

		inlineImage := cid != "" && strings.HasPrefix(strings.ToLower(contentType), "image/")
		if inlineImage {
			preview, previewErr := rasterPreview(data)
			if previewErr != nil {
				result.Warnings = append(result.Warnings, "Some CID images were unsupported or oversized and were skipped.")
				continue
			}
			normalized, ok := decodePreviewData(preview)
			if !ok {
				result.Warnings = append(result.Warnings, "Some CID images could not be normalized safely.")
				continue
			}
			data = normalized
			contentType = "image/png"
		}
		var name string
		if inlineImage {
			name = generatedInlineName("inline", contentType, data, used)
			result.CIDNames[cid] = name
			result.CIDNames[strings.ToLower(cid)] = name
		} else {
			if filename == "" {
				// Ignore unnamed non-body MIME parts that cannot be represented
				// safely in the template attachment model.
				continue
			}
			name = sanitizeImportedAttachmentName(filename, contentType, data, used)
		}
		result.Attachments = append(result.Attachments, models.Attachment{
			Name:    name,
			Type:    contentType,
			Content: base64.StdEncoding.EncodeToString(data),
		})
		total += len(data)
	}
	return result
}

func newEmailAssetLocalizer(ctx context.Context, imported importedEmailAssets, fetchImage func(context.Context, string) (string, error)) *emailAssetLocalizer {
	used := make(map[string]bool)
	total := 0
	for _, attachment := range imported.Attachments {
		used[attachment.Name] = true
		if decoded, err := base64.StdEncoding.DecodeString(attachment.Content); err == nil {
			total += len(decoded)
		}
	}
	return &emailAssetLocalizer{
		ctx:         ctx,
		attachments: append([]models.Attachment(nil), imported.Attachments...),
		cidNames:    imported.CIDNames,
		cached:      map[string]string{},
		usedNames:   used,
		totalBytes:  total,
		warnings:    map[string]bool{},
		fetchImage:  fetchImage,
	}
}

func (l *emailAssetLocalizer) warn(message string) {
	if message != "" {
		l.warnings[message] = true
	}
}

func decodePreviewData(data string) ([]byte, bool) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(data, prefix) {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, prefix))
	return decoded, err == nil
}

func (l *emailAssetLocalizer) addPNG(key string, content []byte) string {
	if name := l.cached[key]; name != "" {
		return "cid:" + name
	}
	if len(l.attachments) >= maxImportedEmailAssets || l.totalBytes+len(content) > maxImportedEmailAssetTotal {
		l.warn("Some email images were skipped because the safe import limit was reached.")
		return key
	}
	name := generatedInlineName("remote", "image/png", content, l.usedNames)
	l.attachments = append(l.attachments, models.Attachment{
		Name:    name,
		Type:    "image/png",
		Content: base64.StdEncoding.EncodeToString(content),
	})
	l.totalBytes += len(content)
	l.cached[key] = name
	return "cid:" + name
}

func (l *emailAssetLocalizer) localize(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "{{") {
		return raw
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "cid:") {
		cid := normalizeContentID(value[4:])
		name := l.cidNames[cid]
		if name == "" {
			name = l.cidNames[strings.ToLower(cid)]
		}
		if name == "" {
			l.warn("Some CID images were missing from the raw email source.")
			return raw
		}
		return "cid:" + name
	}
	if strings.HasPrefix(lower, "data:image/") {
		preview, err := rasterPreviewDataURI(value)
		if err != nil {
			l.warn("Some embedded data images were unsupported or oversized.")
			return raw
		}
		if content, ok := decodePreviewData(preview); ok {
			return l.addPNG(value, content)
		}
		return raw
	}
	if !strings.HasPrefix(lower, "https://") {
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "//") {
			l.warn("Some non-HTTPS email images were left unchanged.")
		}
		return raw
	}
	if name := l.cached[value]; name != "" {
		return "cid:" + name
	}
	preview, err := l.fetchImage(l.ctx, value)
	if err != nil {
		l.warn("Some external email images were unavailable, private, unsupported, or oversized.")
		return raw
	}
	content, ok := decodePreviewData(preview)
	if !ok {
		l.warn("Some external email images could not be normalized safely.")
		return raw
	}
	return l.addPNG(value, content)
}

func (l *emailAssetLocalizer) localizeCSS(css string) string {
	return emailCSSURL.ReplaceAllStringFunc(css, func(match string) string {
		groups := emailCSSURL.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		replacement := l.localize(strings.TrimSpace(groups[1]))
		if replacement == groups[1] {
			return match
		}
		return `url("` + replacement + `")`
	})
}

func (l *emailAssetLocalizer) localizeSrcset(value string) string {
	// Data URLs contain commas and are left to the normal src handler.
	if strings.Contains(strings.ToLower(value), "data:") {
		return value
	}
	parts := strings.Split(value, ",")
	for i, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		fields[0] = l.localize(fields[0])
		parts[i] = strings.Join(fields, " ")
	}
	return strings.Join(parts, ", ")
}

func localizeImportedEmailHTML(ctx context.Context, html string, imported importedEmailAssets) (string, []models.Attachment, []string) {
	return localizeImportedEmailHTMLWithFetcher(ctx, html, imported, fetchImagePreview)
}

func localizeImportedEmailHTMLWithFetcher(ctx context.Context, html string, imported importedEmailAssets, fetchImage func(context.Context, string) (string, error)) (string, []models.Attachment, []string) {
	if strings.TrimSpace(html) == "" {
		return html, imported.Attachments, imported.Warnings
	}
	ctx, cancel := context.WithTimeout(ctx, importedImageFetchTimeout)
	defer cancel()
	localizer := newEmailAssetLocalizer(ctx, imported, fetchImage)
	document, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		warnings := append([]string(nil), imported.Warnings...)
		warnings = append(warnings, "Email HTML could not be analyzed for embedded images.")
		return html, imported.Attachments, warnings
	}
	document.Find("img[src], input[type='image'][src], source[src]").Each(func(_ int, selection *goquery.Selection) {
		if value, ok := selection.Attr("src"); ok {
			selection.SetAttr("src", localizer.localize(value))
		}
	})
	document.Find("[srcset]").Each(func(_ int, selection *goquery.Selection) {
		if value, ok := selection.Attr("srcset"); ok {
			selection.SetAttr("srcset", localizer.localizeSrcset(value))
		}
	})
	document.Find("[style]").Each(func(_ int, selection *goquery.Selection) {
		if value, ok := selection.Attr("style"); ok {
			selection.SetAttr("style", localizer.localizeCSS(value))
		}
	})
	document.Find("style").Each(func(_ int, selection *goquery.Selection) {
		selection.SetText(localizer.localizeCSS(selection.Text()))
	})
	output, err := document.Html()
	if err != nil {
		output = html
		localizer.warn("Email HTML image references could not be rewritten.")
	}
	warnings := append([]string(nil), imported.Warnings...)
	for warning := range localizer.warnings {
		warnings = append(warnings, warning)
	}
	return output, localizer.attachments, warnings
}

func rasterPreviewDataURI(value string) (string, error) {
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return "", errImagePreview
	}
	header, payload := value[:comma], value[comma+1:]
	if !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return "", errImagePreview
	}
	content, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", errImagePreview
	}
	return rasterPreview(content)
}
