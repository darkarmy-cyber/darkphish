package models

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gorm.io/gorm"
)

func buildAuditExportTx(tx *gorm.DB, head *auditChainHead, applicationVersion, commit string) ([]byte, audit.ExportManifest, error) {
	var manifest audit.ExportManifest
	if _, err := verifyAuditChainTx(tx, head); err != nil {
		return nil, manifest, err
	}
	// Keep the established byte-for-byte JSON representation without retaining
	// a second full history as database rows and decoded events. This legacy
	// []byte API still buffers the final payload; it is not a streaming API.
	var content bytes.Buffer
	content.WriteByte('[')
	rows := auditEventIterator{tx: tx}
	for {
		row, ok, err := rows.next()
		if err != nil {
			return nil, manifest, err
		}
		if !ok {
			break
		}
		encoded, err := json.MarshalIndent(rowEvent(row), "  ", "  ")
		if err != nil {
			return nil, manifest, err
		}
		if manifest.RecordCount == 0 {
			manifest.FirstEventID = row.ID
		} else {
			content.WriteByte(',')
		}
		content.WriteString("\n  ")
		content.Write(encoded)
		manifest.RecordCount++
		manifest.LastEventID = row.ID
	}
	if manifest.RecordCount > 0 {
		content.WriteByte('\n')
	}
	content.WriteString("]\n")
	count, first, last := manifest.RecordCount, manifest.FirstEventID, manifest.LastEventID
	manifest = audit.ExportManifest{
		FormatVersion: audit.ManifestFormatVersion, GeneratedAt: time.Now().UTC(), RecordCount: count,
		FirstEventID: first, LastEventID: last,
		SHA256: audit.FileSHA256(content.Bytes()), ApplicationVersion: applicationVersion, Commit: commit,
	}
	if count > 0 {
		checkpoint, checkpointErr := createAuditCheckpointTx(tx, head.Sequence)
		if checkpointErr != nil {
			return nil, manifest, checkpointErr
		}
		manifest.CheckpointReference = checkpoint.ID
	}
	if auditSigner == nil {
		return nil, manifest, errors.New("audit signing key is unavailable")
	}
	if err := auditSigner.SignManifest(&manifest); err != nil {
		return nil, manifest, err
	}
	return content.Bytes(), manifest, nil
}

func VerifyAuditExport(content, manifestContent []byte) (audit.ExportManifest, error) {
	return VerifyAuditExportReader(bytes.NewReader(content), manifestContent)
}

// VerifyAuditExportReader verifies the signed manifest and the complete original
// input, retaining only one decoded event rather than the full event history.
// It does not close content. Callers own reader deadlines and cancellation.
// Memory still depends on the largest individual JSON value and manifest size;
// the legacy byte-slice wrapper necessarily retains its caller-owned input.
func VerifyAuditExportReader(content io.Reader, manifestContent []byte) (audit.ExportManifest, error) {
	var manifest audit.ExportManifest
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		return manifest, fmt.Errorf("decode audit manifest: %w", err)
	}
	if auditSigner == nil {
		return manifest, errors.New("audit signing key is unavailable")
	}
	if err := auditSigner.VerifyManifest(manifest); err != nil {
		return manifest, err
	}
	if content == nil {
		return manifest, errors.New("audit export reader is unavailable")
	}
	digest := sha256.New()
	decoder := json.NewDecoder(io.TeeReader(content, digest))
	opening, err := decoder.Token()
	if err != nil {
		return manifest, fmt.Errorf("decode audit export: %w", err)
	}
	// json.Unmarshal into []audit.Event historically accepts null as an empty
	// export. Preserve this compatibility; its signed count/hash still apply.
	if opening != nil && opening != json.Delim('[') {
		return manifest, errors.New("decode audit export: expected an array or null")
	}
	count := 0
	var firstID, lastID, previousSequence int64
	var previous string
	if opening != nil {
		for decoder.More() {
			// Refuse excess records before decoding their potentially large value,
			// and bound the counter by the signed count before incrementing it.
			if count >= manifest.RecordCount {
				return manifest, errors.New("audit export record count does not match manifest")
			}
			var event audit.Event
			if err := decoder.Decode(&event); err != nil {
				return manifest, fmt.Errorf("decode audit export: %w", err)
			}
			if count == 0 {
				firstID, previous = event.ID, event.PreviousHash
			} else if previousSequence == math.MaxInt64 || event.Sequence != previousSequence+1 {
				return manifest, audit.ErrBrokenChain
			}
			if event.PreviousHash != previous {
				return manifest, audit.ErrBrokenChain
			}
			hash, err := audit.HashEvent(event, previous)
			if err != nil || hash != event.EventHash {
				return manifest, audit.ErrBrokenChain
			}
			lastID, previousSequence, previous = event.ID, event.Sequence, event.EventHash
			count++
		}
		closing, err := decoder.Token()
		if err != nil {
			return manifest, fmt.Errorf("decode audit export: %w", err)
		}
		if closing != json.Delim(']') {
			return manifest, errors.New("decode audit export: missing array end")
		}
	}
	// Token consumes trailing whitespace and forces the underlying reader to EOF,
	// including buffered lookahead. No hash or checkpoint success before this.
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("unexpected trailing JSON value")
		}
		return manifest, fmt.Errorf("decode audit export trailing input: %w", err)
	}
	if hex.EncodeToString(digest.Sum(nil)) != manifest.SHA256 {
		return manifest, errors.New("audit export hash does not match manifest")
	}
	if count != manifest.RecordCount {
		return manifest, errors.New("audit export record count does not match manifest")
	}
	if count > 0 {
		if firstID != manifest.FirstEventID || lastID != manifest.LastEventID {
			return manifest, errors.New("audit export event range does not match manifest")
		}
		if manifest.CheckpointReference != 0 {
			var row auditCheckpointRow
			if err := db.Where("id=?", manifest.CheckpointReference).First(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return manifest, errors.New("referenced audit checkpoint is missing")
				}
				return manifest, err
			}
			if err := auditSigner.VerifyCheckpoint(rowCheckpoint(row)); err != nil || row.FinalHash != previous {
				return manifest, errors.New("audit export checkpoint linkage is invalid")
			}
		}
	}
	return manifest, nil
}

func BuildAuditExport(applicationVersion, commit string) ([]byte, audit.ExportManifest, error) {
	var content []byte
	var manifest audit.ExportManifest
	err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
		var err error
		content, manifest, err = buildAuditExportTx(tx, head, applicationVersion, commit)
		return err
	})
	if err != nil {
		return nil, audit.ExportManifest{}, err
	}
	return content, manifest, nil
}
