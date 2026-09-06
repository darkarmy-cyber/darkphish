package models

import (
	"errors"
	"fmt"
	"sync"
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
	auditAppendMu        sync.Mutex
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
	auditAppendMu.Lock()
	defer auditAppendMu.Unlock()
	tx := db.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer tx.Rollback()
	if event.OutboxID != 0 {
		var existing auditEventRow
		if err := tx.Where("audit_outbox_id=?", event.OutboxID).First(&existing).Error; err == nil {
			_ = tx.Rollback().Error
			if existing.ChainSequence%int64(auditCheckpointEvery) == 0 {
				if _, checkpointErr := createAuditCheckpointLocked(existing.ChainSequence); checkpointErr != nil {
					return existing.ID, fmt.Errorf("audit event persisted but checkpoint retry failed: %w", checkpointErr)
				}
			}
			return existing.ID, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			_ = tx.Rollback().Error
			return 0, err
		}
	}
	var last auditEventRow
	previousHash := ""
	sequence := int64(1)
	lookup := tx.Where("chain_id=?", audit.DefaultChainID).Order("chain_sequence DESC").First(&last)
	if lookup.Error == nil {
		previousHash = last.EventHash
		sequence = last.ChainSequence + 1
	} else if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
		_ = tx.Rollback().Error
		return 0, lookup.Error
	}
	event.ChainID = audit.DefaultChainID
	event.Sequence = sequence
	event.PreviousHash = previousHash
	row := eventRow(event)
	if err := tx.Create(&row).Error; err != nil {
		_ = tx.Rollback().Error
		return 0, err
	}
	// Hash the persisted representation: MySQL/PostgreSQL round timestamps to
	// microseconds, unlike SQLite. Hashing the pre-insert nanoseconds would
	// make an untampered chain fail verification after it is read back.
	if err := tx.Where("id=?", row.ID).First(&row).Error; err != nil {
		_ = tx.Rollback().Error
		return 0, err
	}
	event = rowEvent(row)
	hash, err := audit.HashEvent(event, previousHash)
	if err != nil {
		_ = tx.Rollback().Error
		return 0, err
	}
	if err := tx.Model(&auditEventRow{}).Where("id=?", row.ID).
		Updates(map[string]interface{}{"event_hash": hash, "previous_hash": previousHash, "chain_sequence": sequence, "chain_id": audit.DefaultChainID}).Error; err != nil {
		_ = tx.Rollback().Error
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	if sequence%int64(auditCheckpointEvery) == 0 {
		if _, err := createAuditCheckpointLocked(sequence); err != nil {
			return row.ID, fmt.Errorf("audit event persisted but checkpoint failed: %w", err)
		}
	}
	return row.ID, nil
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
	auditAppendMu.Lock()
	defer auditAppendMu.Unlock()
	rows := []auditEventRow{}
	if err := db.Where("chain_id=?", audit.DefaultChainID).Order("chain_sequence ASC").Find(&rows).Error; err != nil {
		return 0, err
	}
	var cutoff *auditEventRow
	for i := range rows {
		if !rows[i].Timestamp.Before(before.UTC()) {
			break
		}
		cutoff = &rows[i]
	}
	if cutoff == nil {
		return 0, nil
	}
	if _, err := createAuditCheckpointLocked(cutoff.ChainSequence); err != nil {
		return 0, err
	}
	query := db.Where("chain_id=? AND chain_sequence<=?", audit.DefaultChainID, cutoff.ChainSequence).Delete(&auditEventRow{})
	return query.RowsAffected, query.Error
}

func configureAuditStore() error {
	var err error
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

func initializeAuditChain() error {
	auditAppendMu.Lock()
	defer auditAppendMu.Unlock()
	rows := []auditEventRow{}
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	previous := ""
	sequence := int64(0)
	if len(rows) > 0 && rows[0].EventHash != "" {
		sequence = rows[0].ChainSequence - 1
		previous = rows[0].PreviousHash
	}
	for _, row := range rows {
		sequence++
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
			if err := db.Model(&auditEventRow{}).Where("id=?", row.ID).Updates(map[string]interface{}{
				"chain_id": audit.DefaultChainID, "chain_sequence": sequence, "previous_hash": previous, "event_hash": expected,
			}).Error; err != nil {
				return err
			}
		}
		previous = expected
	}
	return nil
}

func createAuditCheckpointLocked(lastSequence int64) (audit.Checkpoint, error) {
	var value audit.Checkpoint
	if auditSigner == nil {
		return value, errors.New("audit signing key is not configured")
	}
	var existing auditCheckpointRow
	if err := db.Where("chain_id=? AND last_sequence=?", audit.DefaultChainID, lastSequence).First(&existing).Error; err == nil {
		return rowCheckpoint(existing), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return value, err
	}
	var first, last auditEventRow
	if err := db.Where("chain_id=? AND chain_sequence<=?", audit.DefaultChainID, lastSequence).Order("chain_sequence ASC").First(&first).Error; err != nil {
		return value, err
	}
	if err := db.Where("chain_id=? AND chain_sequence=?", audit.DefaultChainID, lastSequence).First(&last).Error; err != nil {
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
	if err := db.Create(&row).Error; err != nil {
		return value, err
	}
	value.ID = row.ID
	return value, nil
}

func CreateAuditCheckpoint() (audit.Checkpoint, error) {
	auditAppendMu.Lock()
	defer auditAppendMu.Unlock()
	var last auditEventRow
	if err := db.Where("chain_id=?", audit.DefaultChainID).Order("chain_sequence DESC").First(&last).Error; err != nil {
		return audit.Checkpoint{}, err
	}
	return createAuditCheckpointLocked(last.ChainSequence)
}

type AuditVerification struct {
	FirstEventID int64
	LastEventID  int64
	RecordCount  int64
	FinalHash    string
	CheckpointID int64
}

func VerifyAuditChain() (AuditVerification, error) {
	auditAppendMu.Lock()
	defer auditAppendMu.Unlock()
	var report AuditVerification
	rows := []auditEventRow{}
	if err := db.Where("chain_id=?", audit.DefaultChainID).Order("chain_sequence ASC").Find(&rows).Error; err != nil {
		return report, err
	}
	if len(rows) == 0 {
		return report, nil
	}
	expectedSequence := rows[0].ChainSequence
	previous := rows[0].PreviousHash
	if expectedSequence == 1 {
		if previous != "" {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, rows[0].ID)
		}
	} else {
		var anchor auditCheckpointRow
		if err := db.Where("chain_id=? AND last_sequence=? AND final_hash=?", audit.DefaultChainID, expectedSequence-1, previous).First(&anchor).Error; err != nil {
			return report, fmt.Errorf("%w: missing signed retention anchor before event %d", audit.ErrBrokenChain, rows[0].ID)
		}
		if err := auditSigner.VerifyCheckpoint(rowCheckpoint(anchor)); err != nil {
			return report, err
		}
		report.CheckpointID = anchor.ID
	}
	for i, row := range rows {
		if row.ChainSequence != expectedSequence || row.PreviousHash != previous {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, row.ID)
		}
		event := rowEvent(row)
		expectedHash, err := audit.HashEvent(event, previous)
		if err != nil || expectedHash != row.EventHash {
			return report, fmt.Errorf("%w at event %d", audit.ErrBrokenChain, row.ID)
		}
		if i == 0 {
			report.FirstEventID = row.ID
		}
		report.LastEventID = row.ID
		report.RecordCount++
		report.FinalHash = row.EventHash
		previous = row.EventHash
		expectedSequence++
	}
	checkpoints := []auditCheckpointRow{}
	if err := db.Where("chain_id=?", audit.DefaultChainID).Order("last_sequence ASC").Find(&checkpoints).Error; err != nil {
		return report, err
	}
	for _, row := range checkpoints {
		if err := auditSigner.VerifyCheckpoint(rowCheckpoint(row)); err != nil {
			return report, fmt.Errorf("checkpoint %d: %w", row.ID, err)
		}
		if row.LastSequence >= rows[0].ChainSequence {
			var event auditEventRow
			if err := db.Where("chain_id=? AND chain_sequence=?", row.ChainID, row.LastSequence).First(&event).Error; err != nil || event.EventHash != row.FinalHash {
				return report, fmt.Errorf("checkpoint %d: %w", row.ID, audit.ErrBrokenChain)
			}
		}
		report.CheckpointID = row.ID
	}
	return report, nil
}

func ActiveAuditSigner() *audit.SigningKeyring { return auditSigner }
