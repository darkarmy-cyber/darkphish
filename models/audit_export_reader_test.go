package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing/iotest"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gopkg.in/check.v1"
)

func signedReaderManifest(c *check.C, content []byte, manifest audit.ExportManifest) []byte {
	manifest.SHA256 = audit.FileSHA256(content)
	c.Assert(auditSigner.SignManifest(&manifest), check.IsNil)
	encoded, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	return encoded
}

type auditTerminalErrorReader struct {
	io.Reader
	err error
}

func (r auditTerminalErrorReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		err = r.err
	}
	return n, err
}

func (s *ModelsSuite) TestAuditReaderCompatibleSignedExport(c *check.C) {
	appendIntegrityEvents(c, 3)
	content, manifest, err := BuildAuditExport("0.7.1", "reader-test")
	c.Assert(err, check.IsNil)
	encoded, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	for _, reader := range []io.Reader{bytes.NewReader(content), iotest.OneByteReader(bytes.NewReader(content)), iotest.HalfReader(bytes.NewReader(content)), iotest.DataErrReader(bytes.NewReader(content))} {
		got, err := VerifyAuditExportReader(reader, encoded)
		c.Assert(err, check.IsNil)
		c.Assert(got, check.DeepEquals, manifest)
	}
	got, err := VerifyAuditExport(content, encoded)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, manifest)
	// Whitespace is part of the signed original byte stream, even beyond the
	// decoder's normal read-ahead window.
	padded := append(append([]byte(nil), content...), []byte(strings.Repeat(" \r\n\t", 4096))...)
	_, err = VerifyAuditExportReader(bytes.NewReader(padded), encoded)
	c.Assert(err, check.NotNil)
	_, err = VerifyAuditExportReader(iotest.HalfReader(bytes.NewReader(padded)), signedReaderManifest(c, padded, manifest))
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestAuditReaderEmptyCompatibility(c *check.C) {
	for _, content := range []string{"[]\n", "null", " \r\nnull\t", "[ ]"} {
		manifest := audit.ExportManifest{FormatVersion: audit.ManifestFormatVersion, GeneratedAt: time.Now().UTC()}
		encoded := signedReaderManifest(c, []byte(content), manifest)
		_, err := VerifyAuditExportReader(iotest.OneByteReader(strings.NewReader(content)), encoded)
		c.Assert(err, check.IsNil)
	}
}

func (s *ModelsSuite) TestAuditReaderRejectsExcessRecordBeforeDecoding(c *check.C) {
	manifest := audit.ExportManifest{FormatVersion: audit.ManifestFormatVersion, GeneratedAt: time.Now().UTC()}
	encoded := signedReaderManifest(c, []byte("[]"), manifest)
	failure := errors.New("excess record must not be decoded")
	reader := auditTerminalErrorReader{Reader: strings.NewReader("[{"), err: failure}
	_, err := VerifyAuditExportReader(reader, encoded)
	c.Assert(err, check.NotNil)
	c.Assert(strings.Contains(err.Error(), "record count"), check.Equals, true)
	c.Assert(errors.Is(err, failure), check.Equals, false)
}

func (s *ModelsSuite) TestAuditReaderRejectsMalformedOrTrailingInput(c *check.C) {
	for _, content := range []string{"", "[", "[null]", "[{}]", "{}", "1", "true", `"[]"`, "[] []", "null null", "[]x", "[],", "[]\x00", "[]" + strings.Repeat(" ", 8192) + "false"} {
		manifest := audit.ExportManifest{FormatVersion: audit.ManifestFormatVersion, GeneratedAt: time.Now().UTC()}
		encoded := signedReaderManifest(c, []byte(content), manifest)
		_, err := VerifyAuditExportReader(iotest.OneByteReader(strings.NewReader(content)), encoded)
		c.Assert(err, check.NotNil, check.Commentf("input %q", content))
	}
}

func (s *ModelsSuite) TestAuditReaderErrorsBeforeAndAfterValidJSON(c *check.C) {
	appendIntegrityEvents(c, 2)
	content, manifest, err := BuildAuditExport("0.7.1", "reader-error-test")
	c.Assert(err, check.IsNil)
	encoded, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	failure := errors.New("synthetic read failure")
	for _, length := range []int{0, 1, len(content) / 2, len(content) - 1, len(content)} {
		reader := auditTerminalErrorReader{Reader: bytes.NewReader(content[:length]), err: failure}
		_, err := VerifyAuditExportReader(reader, encoded)
		c.Assert(errors.Is(err, failure), check.Equals, true, check.Commentf("length %d, error %v", length, err))
	}
	_, err = VerifyAuditExportReader(nil, encoded)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestAuditReaderRejectsSignedCountRangeAndCheckpointChanges(c *check.C) {
	appendIntegrityEvents(c, 3)
	content, manifest, err := BuildAuditExport("0.7.1", "reader-integrity-test")
	c.Assert(err, check.IsNil)
	for _, mutate := range []func(*audit.ExportManifest){
		func(m *audit.ExportManifest) { m.RecordCount = -1 },
		func(m *audit.ExportManifest) { m.RecordCount-- },
		func(m *audit.ExportManifest) { m.RecordCount++ },
		func(m *audit.ExportManifest) { m.FirstEventID++ },
		func(m *audit.ExportManifest) { m.LastEventID++ },
		func(m *audit.ExportManifest) { m.CheckpointReference = math.MaxInt64 },
	} {
		changed := manifest
		mutate(&changed)
		_, err := VerifyAuditExportReader(bytes.NewReader(content), signedReaderManifest(c, content, changed))
		c.Assert(err, check.NotNil)
	}
	encoded := signedReaderManifest(c, content, manifest)
	c.Assert(db.Model(&auditCheckpointRow{}).Where("id=?", manifest.CheckpointReference).Update("final_hash", "changed").Error, check.IsNil)
	_, err = VerifyAuditExportReader(bytes.NewReader(content), encoded)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestAuditReaderRejectsBoundaryChainCorruption(c *check.C) {
	appendIntegrityEvents(c, auditScanBatchSize+2)
	content, manifest, err := BuildAuditExport("0.7.1", "reader-boundary-test")
	c.Assert(err, check.IsNil)
	encoded := signedReaderManifest(c, content, manifest)
	_, err = VerifyAuditExportReader(iotest.HalfReader(bytes.NewReader(content)), encoded)
	c.Assert(err, check.IsNil)
	for _, index := range []int{0, auditScanBatchSize - 1, auditScanBatchSize, auditScanBatchSize + 1} {
		var events []audit.Event
		c.Assert(json.Unmarshal(content, &events), check.IsNil)
		events[index].EventHash = "changed"
		changed, err := json.Marshal(events)
		c.Assert(err, check.IsNil)
		_, err = VerifyAuditExportReader(bytes.NewReader(changed), signedReaderManifest(c, changed, manifest))
		c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
	}
}

func (s *ModelsSuite) TestAuditReaderRejectsSequenceOverflow(c *check.C) {
	events := []audit.Event{{ID: 1, Sequence: math.MaxInt64, Timestamp: time.Now().UTC()}, {ID: 2, Sequence: math.MinInt64, Timestamp: time.Now().UTC()}}
	for i := range events {
		if i > 0 {
			events[i].PreviousHash = events[i-1].EventHash
		}
		var err error
		events[i].EventHash, err = audit.HashEvent(events[i], events[i].PreviousHash)
		c.Assert(err, check.IsNil)
	}
	content, err := json.Marshal(events)
	c.Assert(err, check.IsNil)
	manifest := audit.ExportManifest{FormatVersion: audit.ManifestFormatVersion, GeneratedAt: time.Now().UTC(), RecordCount: 2, FirstEventID: 1, LastEventID: 2}
	_, err = VerifyAuditExportReader(bytes.NewReader(content), signedReaderManifest(c, content, manifest))
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}
