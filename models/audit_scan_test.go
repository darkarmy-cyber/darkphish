package models

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/mattn/go-sqlite3"
	"gopkg.in/check.v1"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A deterministic query-shape guard is more reliable than a GC/RSS threshold:
// every runtime event/checkpoint slice must have a bounded SQL LIMIT. Install
// only after fixture construction, which intentionally reads comparison data.
func guardAuditScanQueries(c *check.C) func() {
	const name = "test:bounded-audit-queries"
	c.Assert(db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		switch tx.Statement.Dest.(type) {
		case *[]auditEventRow, *[]auditCheckpointRow:
			limit, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit)
			c.Assert(ok && limit.Limit != nil && *limit.Limit > 0 && *limit.Limit <= auditScanBatchSize, check.Equals, true)
			c.Assert(limit.Offset, check.Equals, 0)
			c.Assert(tx.RowsAffected <= auditScanBatchSize, check.Equals, true)
		}
	}), check.IsNil)
	return func() { c.Assert(db.Callback().Query().Remove(name), check.IsNil) }
}

func (s *ModelsSuite) TestAuditBoundedHistoryOperations(c *check.C) {
	oldInterval := auditCheckpointEvery
	auditCheckpointEvery = 1
	defer func() { auditCheckpointEvery = oldInterval }()
	rows := appendIntegrityEvents(c, 2*auditScanBatchSize+1)
	events := make([]audit.Event, len(rows))
	for i := range rows {
		events[i] = rowEvent(rows[i])
	}
	want, err := json.MarshalIndent(events, "", "  ")
	c.Assert(err, check.IsNil)
	want = append(want, '\n')
	defer guardAuditScanQueries(c)()
	c.Assert(initializeAuditChain(), check.IsNil)
	report, err := VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(len(rows)))
	_, err = CreateAuditCheckpoint()
	c.Assert(err, check.IsNil)
	content, manifest, err := BuildAuditExport("0.7.0", "bounded-test")
	c.Assert(err, check.IsNil)
	c.Assert(string(content), check.Equals, string(want))
	encoded, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	_, err = VerifyAuditExport(content, encoded)
	c.Assert(err, check.IsNil)
	count, err := (databaseAuditStore{}).DeleteBefore(rows[auditScanBatchSize].Timestamp)
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, int64(auditScanBatchSize))
	report, err = VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(len(rows)-auditScanBatchSize))
	c.Assert(report.FirstEventID, check.Equals, rows[auditScanBatchSize].ID)
	count, err = (databaseAuditStore{}).DeleteBefore(rows[len(rows)-1].Timestamp.Add(time.Second))
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, int64(len(rows)-auditScanBatchSize))
	content, manifest, err = BuildAuditExport("0.7.0", "empty-test")
	c.Assert(err, check.IsNil)
	c.Assert(string(content), check.Equals, "[]\n")
	c.Assert(manifest.RecordCount, check.Equals, 0)
	// Even after every event has been retired, an older checkpoint beyond
	// the first page must still be authenticated, not just the latest anchor.
	c.Assert(db.Model(&auditCheckpointRow{}).Where("last_sequence=?", auditScanBatchSize+1).Update("signature", "corrupted").Error, check.IsNil)
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
}

func (s *ModelsSuite) TestAuditScanBoundaryCorruptionFailsClosed(c *check.C) {
	rows := appendIntegrityEvents(c, 2*auditScanBatchSize+1)
	defer guardAuditScanQueries(c)()
	rollback := errors.New("rollback test mutation")
	for _, index := range []int{0, auditScanBatchSize - 1, auditScanBatchSize, auditScanBatchSize + 1, len(rows) - 1} {
		for _, mutation := range []string{"action", "previous_hash", "delete"} {
			err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
				query := tx.Model(&auditEventRow{}).Where("id=?", rows[index].ID)
				if mutation == "delete" {
					c.Assert(query.Delete(&auditEventRow{}).Error, check.IsNil)
				} else {
					c.Assert(query.Update(mutation, "tampered").Error, check.IsNil)
				}
				_, err := verifyAuditChainTx(tx, head)
				c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true, check.Commentf("index=%d mutation=%s", index, mutation))
				return rollback
			})
			c.Assert(errors.Is(err, rollback), check.Equals, true)
		}
	}
	for _, sequence := range []int64{-1, 0} {
		err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
			c.Assert(tx.Model(&auditEventRow{}).Where("id=?", rows[len(rows)-1].ID).Update("chain_sequence", sequence).Error, check.IsNil)
			_, err := verifyAuditChainTx(tx, head)
			c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
			return rollback
		})
		c.Assert(errors.Is(err, rollback), check.Equals, true)
	}
}

func (s *ModelsSuite) TestAuditScanFailureAndRetryDoNotPublishPartialExport(c *check.C) {
	appendIntegrityEvents(c, auditScanBatchSize+1)
	const name = "test:audit-page-failure"
	injected := errors.New("injected second audit page failure")
	failure := injected
	remaining := 1
	c.Assert(db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*[]auditEventRow); ok && strings.Contains(tx.Statement.SQL.String(), "chain_sequence >") && remaining > 0 {
			remaining--
			tx.AddError(failure)
		}
	}), check.IsNil)
	defer func() { c.Assert(db.Callback().Query().Remove(name), check.IsNil) }()
	content, manifest, err := BuildAuditExport("0.7.0", "failure-test")
	c.Assert(errors.Is(err, injected), check.Equals, true)
	c.Assert(content, check.IsNil)
	c.Assert(manifest, check.DeepEquals, audit.ExportManifest{})
	// The first transaction may have checked an entire page. Typed retry must
	// start traversal and report state afresh, not resume the discarded cursor.
	failure, remaining = sqlite3.Error{Code: sqlite3.ErrBusy}, 1
	content, manifest, err = BuildAuditExport("0.7.0", "retry-test")
	c.Assert(err, check.IsNil)
	c.Assert(remaining, check.Equals, 0)
	c.Assert(manifest.RecordCount, check.Equals, auditScanBatchSize+1)
	encoded, err := json.Marshal(manifest)
	c.Assert(err, check.IsNil)
	_, err = VerifyAuditExport(content, encoded)
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestAuditLegacyScanRollbackAndReceipts(c *check.C) {
	rows := appendIntegrityEvents(c, auditScanBatchSize+1)
	for i := range rows {
		c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[i].ID).Update("audit_outbox_id", int64(700000+i)).Error, check.IsNil)
	}
	// Model pre-chain history, including receipt backfill after a full page.
	c.Assert(db.Model(&auditEventRow{}).Where("chain_id=?", audit.DefaultChainID).Updates(map[string]interface{}{"chain_id": "", "chain_sequence": 0, "previous_hash": "", "event_hash": ""}).Error, check.IsNil)
	c.Assert(db.Model(&auditChainHead{}).Where("chain_id=?", audit.DefaultChainID).Update("initialized", false).Error, check.IsNil)
	const name = "test:legacy-page-failure"
	injected := errors.New("injected legacy page failure")
	c.Assert(db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*[]auditEventRow); ok && strings.Contains(tx.Statement.SQL.String(), "id >") {
			tx.AddError(injected)
		}
	}), check.IsNil)
	err := initializeAuditChain()
	c.Assert(db.Callback().Query().Remove(name), check.IsNil)
	c.Assert(errors.Is(err, injected), check.Equals, true)
	var count int64
	c.Assert(db.Model(&auditDeliveryReceipt{}).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	c.Assert(db.Model(&auditEventRow{}).Where("event_hash<>?", "").Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	defer guardAuditScanQueries(c)()
	c.Assert(initializeAuditChain(), check.IsNil)
	c.Assert(initializeAuditChain(), check.IsNil)
	c.Assert(db.Model(&auditDeliveryReceipt{}).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(len(rows)))
	report, err := VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(len(rows)))
}
