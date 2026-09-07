package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"gorm.io/gorm"
)

type auditEventRow struct {
	ID            int64 `gorm:"primaryKey"`
	AuditOutboxID *int64
	Timestamp     time.Time
	Actor         string
	ActorID       int64
	ActorType     string
	Action        string
	TargetType    string
	TargetID      string
	Result        string
	RequestID     string
	SourceIP      string
	UserAgent     string
	AuthMethod    string
	Metadata      string
	ChainID       string
	ChainSequence int64 `gorm:"column:chain_sequence"`
	PreviousHash  string
	EventHash     string
}

func (auditEventRow) TableName() string { return "audit_events" }

type auditCheckpointRow struct {
	ID            int64 `gorm:"primaryKey"`
	FormatVersion int
	ChainID       string
	FirstEventID  int64
	LastEventID   int64
	FirstSequence int64
	LastSequence  int64
	FinalHash     string
	CreatedAt     time.Time
	KeyID         string
	Signature     string
}

func (auditCheckpointRow) TableName() string { return "audit_checkpoints" }

var (
	auditSigner          *audit.SigningKeyring
	auditCheckpointEvery = 1000
)

func eventRow(event audit.Event) auditEventRow {
	row := auditEventRow{
		ID: event.ID, Timestamp: event.Timestamp, Actor: event.Actor, ActorID: event.ActorID,
		ActorType: event.ActorType, Action: event.Action, TargetType: event.TargetType,
		TargetID: event.TargetID, Result: event.Result, RequestID: event.RequestID,
		SourceIP: event.SourceIP, UserAgent: event.UserAgent, AuthMethod: event.AuthMethod,
		Metadata: event.Metadata, ChainID: event.ChainID, ChainSequence: event.Sequence,
		PreviousHash: event.PreviousHash, EventHash: event.EventHash,
	}
	if event.OutboxID != 0 {
		value := event.OutboxID
		row.AuditOutboxID = &value
	}
	return row
}

func rowEvent(row auditEventRow) audit.Event {
	event := audit.Event{
		ID: row.ID, Timestamp: row.Timestamp, Actor: row.Actor, ActorID: row.ActorID,
		ActorType: row.ActorType, Action: row.Action, TargetType: row.TargetType,
		TargetID: row.TargetID, Result: row.Result, RequestID: row.RequestID,
		SourceIP: row.SourceIP, UserAgent: row.UserAgent, AuthMethod: row.AuthMethod,
		Metadata: row.Metadata, ChainID: row.ChainID, Sequence: row.ChainSequence,
		PreviousHash: row.PreviousHash, EventHash: row.EventHash,
	}
	if row.AuditOutboxID != nil {
		event.OutboxID = *row.AuditOutboxID
	}
	return event
}

func checkpointRow(value audit.Checkpoint) auditCheckpointRow {
	return auditCheckpointRow{
		ID: value.ID, FormatVersion: value.FormatVersion, ChainID: value.ChainID,
		FirstEventID: value.FirstEventID, LastEventID: value.LastEventID,
		FirstSequence: value.FirstSequence, LastSequence: value.LastSequence,
		FinalHash: value.FinalHash, CreatedAt: value.CreatedAt, KeyID: value.KeyID, Signature: value.Signature,
	}
}

func rowCheckpoint(row auditCheckpointRow) audit.Checkpoint {
	return audit.Checkpoint{
		ID: row.ID, FormatVersion: row.FormatVersion, ChainID: row.ChainID,
		FirstEventID: row.FirstEventID, LastEventID: row.LastEventID,
		FirstSequence: row.FirstSequence, LastSequence: row.LastSequence,
		FinalHash: row.FinalHash, CreatedAt: row.CreatedAt, KeyID: row.KeyID, Signature: row.Signature,
	}
}

type databaseAuditStore struct{}

func (databaseAuditStore) Append(event audit.Event) (int64, error) {
	var id int64
	err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
		id = 0
		if !head.Initialized {
			return errors.New("audit chain is not initialized")
		}
		if event.OutboxID != 0 {
			var receipt auditDeliveryReceipt
			if err := tx.Where("outbox_id=?", event.OutboxID).First(&receipt).Error; err == nil {
				id = receipt.EventID
				if receipt.Sequence > head.RetiredSequence && receipt.Sequence%int64(auditCheckpointEvery) == 0 {
					// The event, receipt and checkpoint already committed together.
					// Legacy development may have lost its ephemeral signing key;
					// acknowledging that delivery must not wedge the pending outbox.
					if !persistentAuditSigning() {
						var checkpoint auditCheckpointRow
						if err := tx.Where("chain_id=? AND last_sequence=?", audit.DefaultChainID, receipt.Sequence).First(&checkpoint).Error; err == nil {
							return nil
						} else if !errors.Is(err, gorm.ErrRecordNotFound) {
							return err
						}
					}
					_, err = createAuditCheckpointTx(tx, receipt.Sequence)
					return err
				}
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		candidate := event
		candidate.ChainID = audit.DefaultChainID
		candidate.Sequence = head.Sequence + 1
		candidate.PreviousHash = head.EventHash
		row := eventRow(candidate)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// Hash the persisted timestamp representation, preserving format v1.
		if err := tx.Where("id=?", row.ID).First(&row).Error; err != nil {
			return err
		}
		hash, err := audit.HashEvent(rowEvent(row), head.EventHash)
		if err != nil {
			return err
		}
		if err := tx.Model(&auditEventRow{}).Where("id=?", row.ID).Update("event_hash", hash).Error; err != nil {
			return err
		}
		if err := tx.Model(head).Updates(map[string]interface{}{"chain_sequence": candidate.Sequence, "event_id": row.ID, "event_hash": hash}).Error; err != nil {
			return err
		}
		if event.OutboxID != 0 {
			if err := tx.Create(&auditDeliveryReceipt{OutboxID: event.OutboxID, EventID: row.ID, Sequence: candidate.Sequence}).Error; err != nil {
				return err
			}
		}
		if candidate.Sequence%int64(auditCheckpointEvery) == 0 {
			if _, err := createAuditCheckpointTx(tx, candidate.Sequence); err != nil {
				return err
			}
		}
		id = row.ID
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func applyAuditFilter(query *gorm.DB, filter audit.Filter) *gorm.DB {
	if filter.Action != "" {
		query = query.Where("action=?", filter.Action)
	}
	if filter.ActorID != 0 {
		query = query.Where("actor_id=?", filter.ActorID)
	}
	if filter.Actor != "" {
		query = query.Where("actor=?", filter.Actor)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type=?", filter.TargetType)
	}
	if filter.TargetID != "" {
		query = query.Where("target_id=?", filter.TargetID)
	}
	if filter.Result != "" {
		query = query.Where("result=?", filter.Result)
	}
	if filter.From != nil {
		query = query.Where("timestamp>=?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("timestamp<=?", filter.To.UTC())
	}
	return query
}

func (databaseAuditStore) Query(filter audit.Filter) ([]audit.Event, int64, error) {
	query := applyAuditFilter(db.Model(&auditEventRow{}), filter).Session(&gorm.Session{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PerPage < 1 || filter.PerPage > 500 {
		filter.PerPage = 50
	}
	rows := []auditEventRow{}
	if err := query.Order("timestamp DESC, id DESC").Limit(filter.PerPage).Offset((filter.Page - 1) * filter.PerPage).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	events := make([]audit.Event, len(rows))
	for i := range rows {
		events[i] = rowEvent(rows[i])
	}
	return events, total, nil
}

func (databaseAuditStore) DeleteBefore(before time.Time) (int64, error) {
	var count int64
	err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
		count = 0
		if _, err := verifyAuditChainTx(tx, head); err != nil {
			return err
		}
		rows := auditEventIterator{tx: tx}
		var cutoff int64
		for {
			row, ok, err := rows.next()
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			if !row.Timestamp.Before(before.UTC()) {
				break
			}
			cutoff = row.ChainSequence
		}
		if cutoff == 0 {
			return nil
		}
		if _, err := createAuditCheckpointTx(tx, cutoff); err != nil {
			return err
		}
		deleted := tx.Where("chain_id=? AND chain_sequence<=?", audit.DefaultChainID, cutoff).Delete(&auditEventRow{})
		if deleted.Error != nil {
			return deleted.Error
		}
		count = deleted.RowsAffected
		return tx.Model(head).Update("retired_sequence", cutoff).Error
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func configureAuditStore() error {
	var err error
	if conf != nil && conf.Audit.AllowLegacyEphemeralRecovery && (conf.ProductionMode || conf.Audit.MultiInstance || conf.Audit.ActiveSigningKeyID != "") {
		return errors.New("legacy ephemeral recovery is only available for unkeyed single-instance development")
	}
	if conf != nil && conf.Audit.MultiInstance {
		if conf.DBName != "mysql" && conf.DBName != "postgres" {
			return errors.New("audit.multi_instance requires MySQL or PostgreSQL; SQLite is single-instance only")
		}
		if conf.Audit.ActiveSigningKeyID == "" {
			return errors.New("audit.multi_instance requires a persistent shared audit signing keyring")
		}
	}
	if conf != nil && conf.Audit.ActiveSigningKeyID != "" {
		auditSigner, err = audit.NewSigningKeyring(conf.Audit.ActiveSigningKeyID, conf.Audit.SigningKeys)
	} else {
		auditSigner, err = audit.NewEphemeralSigningKeyring()
		if err == nil {
			log.Warn("audit checkpoints use an ephemeral development signing key; configure a persistent key before relying on verification across restarts")
		}
	}
	if err != nil {
		return err
	}
	if conf != nil && conf.Audit.CheckpointInterval > 0 {
		auditCheckpointEvery = conf.Audit.CheckpointInterval
	}
	if err := initializeAuditChain(); err != nil {
		return err
	}
	audit.SetStore(databaseAuditStore{})
	return nil
}

func initializeAuditChainTx(tx *gorm.DB, head *auditChainHead) error {
	if head.Initialized {
		_, err := verifyAuditChainStateTx(tx, head, persistentAuditSigning())
		return err
	}
	rows := auditEventIterator{tx: tx, legacyByID: true}
	previous := ""
	sequence := int64(0)
	var firstSequence, lastID int64
	found := false
	for {
		row, ok, err := rows.next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		if !found && row.EventHash != "" {
			sequence = row.ChainSequence - 1
			previous = row.PreviousHash
		}
		sequence++
		if !found {
			firstSequence = sequence
			found = true
		}
		event := rowEvent(row)
		event.ChainID = audit.DefaultChainID
		event.Sequence = sequence
		event.PreviousHash = previous
		expected, err := audit.HashEvent(event, previous)
		if err != nil {
			return err
		}
		if row.EventHash != "" && (row.EventHash != expected || row.PreviousHash != previous || row.ChainSequence != sequence) {
			return fmt.Errorf("%w at event %d", audit.ErrBrokenChain, row.ID)
		}
		if row.EventHash == "" {
			if err := tx.Model(&auditEventRow{}).Where("id=?", row.ID).Updates(map[string]interface{}{
				"chain_id": audit.DefaultChainID, "chain_sequence": sequence, "previous_hash": previous, "event_hash": expected,
			}).Error; err != nil {
				return err
			}
		}
		if row.AuditOutboxID != nil {
			if err := tx.Create(&auditDeliveryReceipt{OutboxID: *row.AuditOutboxID, EventID: row.ID, Sequence: sequence}).Error; err != nil {
				return err
			}
		}
		previous = expected
		lastID = row.ID
	}
	if found {
		head.Sequence = sequence
		head.EventID = lastID
		head.EventHash = previous
		head.RetiredSequence = firstSequence - 1
		if head.RetiredSequence < 0 {
			head.RetiredSequence = 0
		}
	} else {
		var last auditCheckpointRow
		if err := tx.Where("chain_id=?", audit.DefaultChainID).Order("last_sequence DESC").First(&last).Error; err == nil {
			if persistentAuditSigning() {
				if err := auditSigner.VerifyCheckpoint(rowCheckpoint(last)); err != nil {
					return err
				}
			}
			head.Sequence, head.EventID, head.EventHash, head.RetiredSequence = last.LastSequence, last.LastEventID, last.FinalHash, last.LastSequence
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	head.Initialized = true
	if err := tx.Save(head).Error; err != nil {
		return err
	}
	_, err := verifyAuditChainStateTx(tx, head, persistentAuditSigning())
	return err
}

func persistentAuditSigning() bool {
	return conf != nil && conf.Audit.ActiveSigningKeyID != ""
}

func initializeAuditChain() error {
	return withAuditChainInitialization(initializeAuditChainTx)
}

func createAuditCheckpointTx(tx *gorm.DB, lastSequence int64) (audit.Checkpoint, error) {
	var value audit.Checkpoint
	if auditSigner == nil {
		return value, errors.New("audit signing key is not configured")
	}
	var existing auditCheckpointRow
	if err := tx.Where("chain_id=? AND last_sequence=?", audit.DefaultChainID, lastSequence).First(&existing).Error; err == nil {
		value := rowCheckpoint(existing)
		return value, auditSigner.VerifyCheckpoint(value)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return value, err
	}
	var first, last auditEventRow
	if err := tx.Where("chain_id=? AND chain_sequence<=?", audit.DefaultChainID, lastSequence).Order("chain_sequence ASC").First(&first).Error; err != nil {
		return value, err
	}
	if err := tx.Where("chain_id=? AND chain_sequence=?", audit.DefaultChainID, lastSequence).First(&last).Error; err != nil {
		return value, err
	}
	value = audit.Checkpoint{
		FormatVersion: audit.CheckpointFormatVersion, ChainID: audit.DefaultChainID,
		FirstEventID: first.ID, LastEventID: last.ID, FirstSequence: first.ChainSequence, LastSequence: last.ChainSequence,
		FinalHash: last.EventHash, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := auditSigner.SignCheckpoint(&value); err != nil {
		return value, err
	}
	row := checkpointRow(value)
	if err := tx.Create(&row).Error; err != nil {
		return value, err
	}
	value.ID = row.ID
	return value, nil
}

func CreateAuditCheckpoint() (audit.Checkpoint, error) {
	var value audit.Checkpoint
	err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
		if _, err := verifyAuditChainTx(tx, head); err != nil {
			return err
		}
		var err error
		value, err = createAuditCheckpointTx(tx, head.Sequence)
		return err
	})
	return value, err
}

type AuditVerification struct {
	FirstEventID int64
	LastEventID  int64
	RecordCount  int64
	FinalHash    string
	CheckpointID int64
}

func verifyAuditChainTx(tx *gorm.DB, head *auditChainHead) (AuditVerification, error) {
	return verifyAuditChainStateTx(tx, head, true)
}

// Legacy development databases may contain checkpoints whose ephemeral private
// key was intentionally never persisted. Startup still checks every available
// event hash, sequence and checkpoint linkage, but cannot authenticate those
// signatures. Explicit verification/export/retention always require signatures.
// Configured persistent keyrings (including all multi-instance writers) do too.
func verifyAuditChainStateTx(tx *gorm.DB, head *auditChainHead, verifySignatures bool) (AuditVerification, error) {
	var report AuditVerification
	if !head.Initialized {
		return report, errors.New("audit chain is not initialized")
	}
	rows := auditEventIterator{tx: tx}
	first, ok, err := rows.next()
	if err != nil {
		return report, err
	}
	if !ok {
		checkpoints := auditCheckpointIterator{tx: tx}
		for {
			checkpoint, ok, err := checkpoints.next()
			if err != nil {
				return report, err
			}
			if !ok {
				break
			}
			if checkpoint.LastSequence > head.Sequence {
				return report, audit.ErrBrokenChain
			}
			if verifySignatures {
				if err := auditSigner.VerifyCheckpoint(rowCheckpoint(checkpoint)); err != nil {
					return report, err
				}
			}
		}
		if head.Sequence == 0 && head.RetiredSequence == 0 && head.EventHash == "" {
			return report, nil
		}
		var anchor auditCheckpointRow
		if head.Sequence != head.RetiredSequence {
			return report, audit.ErrBrokenChain
		}
		if err := tx.Where("chain_id=? AND last_sequence=? AND final_hash=?", audit.DefaultChainID, head.Sequence, head.EventHash).First(&anchor).Error; err != nil {
			return report, audit.ErrBrokenChain
		}
		report.CheckpointID, report.FinalHash = anchor.ID, anchor.FinalHash
		if verifySignatures {
			return report, auditSigner.VerifyCheckpoint(rowCheckpoint(anchor))
		}
		return report, nil
	}
	if first.ChainSequence != head.RetiredSequence+1 {
		return report, audit.ErrBrokenChain
	}
	expectedSequence := first.ChainSequence
	previous := first.PreviousHash
	if expectedSequence == 1 {
		if previous != "" {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, first.ID)
		}
	} else {
		var anchor auditCheckpointRow
		if err := tx.Where("chain_id=? AND last_sequence=? AND final_hash=?", audit.DefaultChainID, expectedSequence-1, previous).First(&anchor).Error; err != nil {
			return report, fmt.Errorf("%w: missing signed retention anchor before event %d", audit.ErrBrokenChain, first.ID)
		}
		if verifySignatures {
			if err := auditSigner.VerifyCheckpoint(rowCheckpoint(anchor)); err != nil {
				return report, err
			}
		}
		report.CheckpointID = anchor.ID
	}
	report.FirstEventID = first.ID
	lastSequence := int64(0)
	for row := first; ; {
		if row.ChainSequence != expectedSequence || row.PreviousHash != previous {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, row.ID)
		}
		event := rowEvent(row)
		expectedHash, err := audit.HashEvent(event, previous)
		if err != nil || expectedHash != row.EventHash {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, row.ID)
		}
		report.LastEventID = row.ID
		report.RecordCount++
		report.FinalHash = row.EventHash
		previous = row.EventHash
		lastSequence = row.ChainSequence
		expectedSequence++
		var ok bool
		row, ok, err = rows.next()
		if err != nil {
			return report, err
		}
		if !ok {
			break
		}
	}
	if lastSequence != head.Sequence || report.FinalHash != head.EventHash || report.LastEventID != head.EventID {
		return report, audit.ErrBrokenChain
	}
	checkpoints := auditCheckpointIterator{tx: tx}
	for {
		row, ok, err := checkpoints.next()
		if err != nil {
			return report, err
		}
		if !ok {
			break
		}
		if verifySignatures {
			if err := auditSigner.VerifyCheckpoint(rowCheckpoint(row)); err != nil {
				return report, fmt.Errorf("checkpoint %d: %w", row.ID, err)
			}
		}
		if row.LastSequence >= first.ChainSequence {
			var event auditEventRow
			if err := tx.Where("chain_id=? AND chain_sequence=?", row.ChainID, row.LastSequence).First(&event).Error; err != nil || event.EventHash != row.FinalHash {
				return report, fmt.Errorf("checkpoint %d: %w", row.ID, audit.ErrBrokenChain)
			}
		}
		report.CheckpointID = row.ID
	}
	return report, nil
}

func VerifyAuditChain() (AuditVerification, error) {
	var report AuditVerification
	err := withAuditChain(func(tx *gorm.DB, head *auditChainHead) error {
		var err error
		report, err = verifyAuditChainTx(tx, head)
		return err
	})
	return report, err
}

func ActiveAuditSigner() *audit.SigningKeyring { return auditSigner }
