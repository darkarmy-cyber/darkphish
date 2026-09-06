package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type auditChainHead struct {
	ChainID               string `gorm:"primaryKey"`
	Sequence              int64  `gorm:"column:chain_sequence"`
	EventID               int64
	EventHash             string
	RetiredSequence       int64
	Initialized           bool
	SharedSigningRequired bool
}

func (auditChainHead) TableName() string { return "audit_chain_heads" }

// Delivery receipts intentionally outlive event retention. Retrying a delayed
// outbox worker must never resurrect an already retired event.
type auditDeliveryReceipt struct {
	OutboxID int64 `gorm:"primaryKey"`
	EventID  int64
	Sequence int64 `gorm:"column:chain_sequence"`
}

func (auditDeliveryReceipt) TableName() string { return "audit_delivery_receipts" }

type auditSigningIdentity struct {
	KeyID       string `gorm:"primaryKey"`
	Fingerprint string
}

func (auditSigningIdentity) TableName() string { return "audit_signing_identities" }

func checkAuditSigningIdentity(tx *gorm.DB, head *auditChainHead) error {
	multi := conf != nil && conf.Audit.MultiInstance
	if !multi && !head.SharedSigningRequired {
		return nil
	}
	if !multi || auditSigner == nil || conf.Audit.ActiveSigningKeyID == "" {
		return errors.New("shared audit chain requires audit.multi_instance and persistent shared signing keys on every instance")
	}
	identities := []auditSigningIdentity{}
	if err := tx.Find(&identities).Error; err != nil {
		return err
	}
	fingerprints := auditSigner.PublicKeyFingerprints()
	for _, identity := range identities {
		if fingerprints[identity.KeyID] != identity.Fingerprint {
			return errors.New("shared audit signing keyring does not match the database signing identities")
		}
		delete(fingerprints, identity.KeyID)
	}
	for id, fingerprint := range fingerprints {
		if err := tx.Create(&auditSigningIdentity{KeyID: id, Fingerprint: fingerprint}).Error; err != nil {
			return err
		}
	}
	if !head.SharedSigningRequired {
		return tx.Model(head).Update("shared_signing_required", true).Error
	}
	return nil
}

func retryableAuditError(err error) bool {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "40001" || pg.Code == "40P01" || pg.Code == "55P03"
	}
	var my *mysqlDriver.MySQLError
	if errors.As(err, &my) {
		return my.Number == 1213 || my.Number == 1205
	}
	var lite sqlite3.Error
	return errors.As(err, &lite) && (lite.Code == sqlite3.ErrBusy || lite.Code == sqlite3.ErrLocked)
}

// Every audit operation takes this lock before reading any chain state. Network
// backends use a current, row-locking read at READ COMMITTED; SQLite takes its
// writer lock before any reads. The callback must have no external side effects.
func withAuditChain(fn func(*gorm.DB, *auditChainHead) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for attempt := 0; attempt < 6; attempt++ {
		options := &sql.TxOptions{}
		if db.Dialector.Name() != "sqlite" {
			options.Isolation = sql.LevelReadCommitted
		}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if tx.Dialector.Name() == "sqlite" {
				if err := tx.Exec("UPDATE audit_chain_heads SET chain_sequence=chain_sequence WHERE chain_id=?", audit.DefaultChainID).Error; err != nil {
					return err
				}
			}
			var head auditChainHead
			query := tx.Where("chain_id=?", audit.DefaultChainID)
			if tx.Dialector.Name() != "sqlite" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := query.First(&head).Error; err != nil {
				return err
			}
			if err := checkAuditSigningIdentity(tx, &head); err != nil {
				return err
			}
			return fn(tx, &head)
		}, options)
		if err == nil || !retryableAuditError(err) || attempt == 5 {
			return err
		}
		timer := time.NewTimer(time.Duration(10*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("audit transaction retry limit exceeded")
}
