package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gopkg.in/check.v1"
)

func appendIntegrityEvents(c *check.C, count int) []auditEventRow {
	for i := 0; i < count; i++ {
		c.Assert(audit.AppendEvent(audit.Event{
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Millisecond), Actor: "integrity-test", ActorType: "system",
			Action: fmt.Sprintf("test.action.%d", i), TargetType: "test", TargetID: fmt.Sprintf("%d", i),
			Result: "success", RequestID: fmt.Sprintf("request-%d", i), AuthMethod: "system", Metadata: "{}",
		}), check.IsNil)
	}
	rows := []auditEventRow{}
	c.Assert(db.Order("chain_sequence ASC").Find(&rows).Error, check.IsNil)
	c.Assert(rows, check.HasLen, count)
	return rows
}

func (s *ModelsSuite) TestConcurrentAuditWritesRemainOrdered(c *check.C) {
	const count = 12
	errorsFound := make(chan error, count)
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		wait.Add(1)
		go func(id int) {
			defer wait.Done()
			errorsFound <- audit.AppendEvent(audit.Event{
				Timestamp: time.Now().UTC(), Actor: "concurrent-test", ActorType: "system", Action: "concurrent.write",
				TargetType: "test", TargetID: fmt.Sprintf("%d", id), Result: "success", RequestID: fmt.Sprintf("concurrent-%d", id), Metadata: "{}",
			})
		}(i)
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		c.Assert(err, check.IsNil)
	}
	report, err := VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(count))
}

func (s *ModelsSuite) TestAuditChainValidAndModifiedEventDetected(c *check.C) {
	rows := appendIntegrityEvents(c, 4)
	report, err := VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(4))
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[1].ID).Update("action", "tampered").Error, check.IsNil)
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestAuditChainDeletedMiddleEventDetected(c *check.C) {
	rows := appendIntegrityEvents(c, 4)
	c.Assert(db.Where("id=?", rows[1].ID).Delete(&auditEventRow{}).Error, check.IsNil)
	_, err := VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestAuditChainChangedPreviousHashDetected(c *check.C) {
	rows := appendIntegrityEvents(c, 3)
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[1].ID).Update("previous_hash", "tampered").Error, check.IsNil)
	_, err := VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestAuditChainReorderedEventDetected(c *check.C) {
	rows := appendIntegrityEvents(c, 3)
	// Sequence zero is intentionally outside the partial uniqueness constraint
	// so the test can simulate a malicious two-row swap.
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[0].ID).Update("chain_sequence", int64(0)).Error, check.IsNil)
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[1].ID).Update("chain_sequence", int64(1)).Error, check.IsNil)
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[0].ID).Update("chain_sequence", int64(2)).Error, check.IsNil)
	_, err := VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestAuditCheckpointCorruptionAndInvalidSignatureDetected(c *check.C) {
	appendIntegrityEvents(c, 3)
	checkpoint, err := CreateAuditCheckpoint()
	c.Assert(err, check.IsNil)
	c.Assert(db.Model(&auditCheckpointRow{}).Where("id=?", checkpoint.ID).Update("final_hash", "tampered").Error, check.IsNil)
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
	c.Assert(db.Model(&auditCheckpointRow{}).Where("id=?", checkpoint.ID).Updates(map[string]interface{}{"final_hash": checkpoint.FinalHash, "signature": "invalid"}).Error, check.IsNil)
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
}

func (s *ModelsSuite) TestSignedAuditExportVerification(c *check.C) {
	appendIntegrityEvents(c, 3)
	content, manifest, err := BuildAuditExport("0.3.0", "test-commit")
	c.Assert(err, check.IsNil)
	manifestContent, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	verified, err := VerifyAuditExport(content, manifestContent)
	c.Assert(err, check.IsNil)
	c.Assert(verified.RecordCount, check.Equals, 3)

	tampered := append([]byte(nil), content...)
	tampered[len(tampered)-2] ^= 1
	_, err = VerifyAuditExport(tampered, manifestContent)
	c.Assert(err, check.NotNil)

	manifest.Signature = "invalid"
	manifestContent, err = json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	_, err = VerifyAuditExport(content, manifestContent)
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
}

func (s *ModelsSuite) TestAuditOutboxDeliveryIsIdempotent(c *check.C) {
	event := audit.Event{
		OutboxID: 4242, Timestamp: time.Now().UTC(), Actor: "outbox-test", ActorType: "system",
		Action: "outbox.retry", TargetType: "test", TargetID: "4242", Result: "success", RequestID: "outbox-retry", Metadata: "{}",
	}
	c.Assert(audit.AppendEvent(event), check.IsNil)
	c.Assert(audit.AppendEvent(event), check.IsNil)
	_, total, err := audit.Query(audit.Filter{Action: "outbox.retry", Page: 1, PerPage: 10})
	c.Assert(err, check.IsNil)
	c.Assert(total, check.Equals, int64(1))
}

func (s *ModelsSuite) TestAuditOutboxRetryRestoresRequiredCheckpoint(c *check.C) {
	previousInterval := auditCheckpointEvery
	auditCheckpointEvery = 1
	defer func() { auditCheckpointEvery = previousInterval }()
	event := audit.Event{
		OutboxID: 4343, Timestamp: time.Now().UTC(), Actor: "outbox-test", ActorType: "system",
		Action: "outbox.checkpoint-retry", TargetType: "test", TargetID: "4343", Result: "success",
		RequestID: "outbox-checkpoint-retry", Metadata: "{}",
	}
	c.Assert(audit.AppendEvent(event), check.IsNil)
	c.Assert(db.Where("chain_id=?", audit.DefaultChainID).Delete(&auditCheckpointRow{}).Error, check.IsNil)
	c.Assert(audit.AppendEvent(event), check.IsNil)
	var eventCount, checkpointCount int64
	c.Assert(db.Model(&auditEventRow{}).Where("audit_outbox_id=?", event.OutboxID).Count(&eventCount).Error, check.IsNil)
	c.Assert(db.Model(&auditCheckpointRow{}).Count(&checkpointCount).Error, check.IsNil)
	c.Assert(eventCount, check.Equals, int64(1))
	c.Assert(checkpointCount, check.Equals, int64(1))
}
