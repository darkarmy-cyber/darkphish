package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	if audit.FileSHA256(content) != manifest.SHA256 {
		return manifest, errors.New("audit export hash does not match manifest")
	}
	var events []audit.Event
	if err := json.Unmarshal(content, &events); err != nil {
		return manifest, fmt.Errorf("decode audit export: %w", err)
	}
	if len(events) != manifest.RecordCount {
		return manifest, errors.New("audit export record count does not match manifest")
	}
	if len(events) > 0 {
		if events[0].ID != manifest.FirstEventID || events[len(events)-1].ID != manifest.LastEventID {
			return manifest, errors.New("audit export event range does not match manifest")
		}
		previous := events[0].PreviousHash
		expectedSequence := events[0].Sequence
		for _, event := range events {
			if event.Sequence != expectedSequence || event.PreviousHash != previous {
				return manifest, audit.ErrBrokenChain
			}
			hash, err := audit.HashEvent(event, previous)
			if err != nil || hash != event.EventHash {
				return manifest, audit.ErrBrokenChain
			}
			previous = event.EventHash
			expectedSequence++
		}
		if manifest.CheckpointReference != 0 {
			var row auditCheckpointRow
			if err := db.Where("id=?", manifest.CheckpointReference).First(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return manifest, errors.New("referenced audit checkpoint is missing")
				}
				return manifest, err
			}
			if err := auditSigner.VerifyCheckpoint(rowCheckpoint(row)); err != nil || row.FinalHash != events[len(events)-1].EventHash {
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
