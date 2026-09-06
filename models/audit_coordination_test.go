package models

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"gopkg.in/check.v1"
	"gorm.io/gorm"
)

func TestAuditRetryClassification(t *testing.T) {
	for _, err := range []error{&pgconn.PgError{Code: "40001"}, &pgconn.PgError{Code: "40P01"}, &pgconn.PgError{Code: "55P03"}, &mysqlDriver.MySQLError{Number: 1213}, &mysqlDriver.MySQLError{Number: 1205}, sqlite3.Error{Code: sqlite3.ErrBusy}, sqlite3.Error{Code: sqlite3.ErrLocked}} {
		if !retryableAuditError(fmt.Errorf("wrapped: %w", err)) {
			t.Errorf("not retried: %T", err)
		}
	}
	for _, err := range []error{errors.New("deadlock"), &pgconn.PgError{Code: "23505"}, &mysqlDriver.MySQLError{Number: 1062}, sqlite3.Error{Code: sqlite3.ErrConstraint}, audit.ErrBrokenChain, audit.ErrInvalidSignature} {
		if retryableAuditError(err) {
			t.Errorf("unexpected retry: %v", err)
		}
	}
}

func (s *ModelsSuite) TestAuditFullRetentionKeepsHeadAndDeliveryReceipt(c *check.C) {
	event := audit.Event{OutboxID: 87123, Timestamp: time.Now().UTC().Add(-time.Hour), Actor: "test", Action: "retained", Metadata: "{}"}
	id, err := (databaseAuditStore{}).Append(event)
	c.Assert(err, check.IsNil)
	var original auditEventRow
	c.Assert(db.First(&original, id).Error, check.IsNil)
	count, err := (databaseAuditStore{}).DeleteBefore(time.Now().UTC())
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, int64(1))
	c.Assert(initializeAuditChain(), check.IsNil)
	idAgain, err := (databaseAuditStore{}).Append(event)
	c.Assert(err, check.IsNil)
	c.Assert(idAgain, check.Equals, id)
	c.Assert(audit.AppendEvent(audit.Event{Timestamp: time.Now().UTC(), Actor: "test", Action: "after.retention", Metadata: "{}"}), check.IsNil)
	var row auditEventRow
	c.Assert(db.First(&row).Error, check.IsNil)
	c.Assert(row.ChainSequence, check.Equals, original.ChainSequence+1)
	c.Assert(row.PreviousHash, check.Equals, original.EventHash)
	_, err = VerifyAuditChain()
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestAuditTailDeletionDetected(c *check.C) {
	rows := appendIntegrityEvents(c, 3)
	c.Assert(db.Delete(&rows[2]).Error, check.IsNil)
	_, err := VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestAuditCheckpointFailureRollsBackAppend(c *check.C) {
	previousSigner, previousInterval := auditSigner, auditCheckpointEvery
	defer func() { auditSigner, auditCheckpointEvery = previousSigner, previousInterval }()
	auditSigner, auditCheckpointEvery = nil, 1
	event := audit.Event{Timestamp: time.Now().UTC(), Action: "rollback", Actor: "test", Metadata: "{}", OutboxID: 90231}
	_, err := (databaseAuditStore{}).Append(event)
	c.Assert(err, check.NotNil)
	var head auditChainHead
	c.Assert(db.First(&head).Error, check.IsNil)
	c.Assert(head.Sequence, check.Equals, int64(0))
	var count int64
	c.Assert(db.Model(&auditEventRow{}).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	c.Assert(db.Model(&auditDeliveryReceipt{}).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	auditSigner = previousSigner
	_, err = (databaseAuditStore{}).Append(event)
	c.Assert(err, check.IsNil)
	report, err := VerifyAuditChain()
	c.Assert(err, check.IsNil)
	c.Assert(report.RecordCount, check.Equals, int64(1))
}

func (s *ModelsSuite) TestEphemeralCheckpointDoesNotPreventDevelopmentRestart(c *check.C) {
	rows := appendIntegrityEvents(c, 3)
	_, err := CreateAuditCheckpoint()
	c.Assert(err, check.IsNil)
	previousSigner, previousConfig := auditSigner, conf
	defer func() { auditSigner, conf = previousSigner, previousConfig }()
	// Exercise the actual startup signer generation with the same database.
	c.Assert(configureAuditStore(), check.IsNil)
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
	// First 0.6 initialization of a 0.5 development database has the same rule.
	c.Assert(db.Model(&auditChainHead{}).Where("chain_id=?", audit.DefaultChainID).Update("initialized", false).Error, check.IsNil)
	c.Assert(initializeAuditChain(), check.IsNil)
	c.Assert(audit.AppendEvent(audit.Event{Timestamp: time.Now().UTC(), Actor: "test", Action: "after.restart", Metadata: "{}"}), check.IsNil)
	// A configured persistent keyring may never use the development exception.
	persistent := *conf
	persistent.Audit.ActiveSigningKeyID = "persistent-test"
	conf = &persistent
	c.Assert(initializeAuditChain(), check.NotNil)
	conf = previousConfig
	// Even ephemeral development startup still fails on modified event hashes.
	c.Assert(db.Model(&auditEventRow{}).Where("id=?", rows[0].ID).Update("action", "tampered").Error, check.IsNil)
	c.Assert(errors.Is(initializeAuditChain(), audit.ErrBrokenChain), check.Equals, true)
}

func (s *ModelsSuite) TestEphemeralRestartAcknowledgesCommittedOutboxCheckpoint(c *check.C) {
	previousSigner, previousInterval := auditSigner, auditCheckpointEvery
	defer func() { auditSigner, auditCheckpointEvery = previousSigner, previousInterval }()
	auditCheckpointEvery = 1
	event := audit.Event{Timestamp: time.Now().UTC(), Actor: "test", Action: "outbox.before.crash", Metadata: "{}"}
	c.Assert(db.Transaction(func(tx *gorm.DB) error { return enqueueAuditEvent(tx, event) }), check.IsNil)
	var pending auditOutboxRow
	c.Assert(db.First(&pending).Error, check.IsNil)
	event.OutboxID = pending.ID
	_, err := (databaseAuditStore{}).Append(event)
	c.Assert(err, check.IsNil)
	// Crash window: the event/checkpoint committed, but dispatched_at is NULL.
	c.Assert(db.Transaction(func(tx *gorm.DB) error {
		return enqueueAuditEvent(tx, audit.Event{Timestamp: time.Now().UTC(), Actor: "test", Action: "outbox.after.crash", Metadata: "{}"})
	}), check.IsNil)
	c.Assert(configureAuditStore(), check.IsNil)
	c.Assert(FlushAuditOutbox(), check.IsNil)
	var count int64
	c.Assert(db.Model(&auditOutboxRow{}).Where("dispatched_at IS NULL").Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(0))
	c.Assert(db.Model(&auditEventRow{}).Count(&count).Error, check.IsNil)
	c.Assert(count, check.Equals, int64(2))
	// Acknowledgement does not turn a lost-key signature into a verified one.
	_, err = VerifyAuditChain()
	c.Assert(errors.Is(err, audit.ErrInvalidSignature), check.Equals, true)
}
