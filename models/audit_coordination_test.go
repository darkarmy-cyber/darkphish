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
