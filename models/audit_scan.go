package models

import (
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gorm.io/gorm"
)

// Bound database materialization independently of retained history size. These
// iterators must stay inside the transaction holding the audit chain lock. No
// cursor remains open while callbacks execute queries on the same connection.
const auditScanBatchSize = 256

type auditEventIterator struct {
	tx         *gorm.DB
	legacyByID bool
	batch      []auditEventRow
	position   int
	started    bool
	done       bool
	sequence   int64
	id         int64
}

func (it *auditEventIterator) next() (auditEventRow, bool, error) {
	if it.position == len(it.batch) {
		if it.done {
			return auditEventRow{}, false, nil
		}
		query := it.tx.Limit(auditScanBatchSize)
		if it.legacyByID {
			query = query.Order("id ASC")
			if it.started {
				query = query.Where("id > ?", it.id)
			}
		} else {
			query = query.Where("chain_id=?", audit.DefaultChainID).Order("chain_sequence ASC, id ASC")
			if it.started {
				query = query.Where("(chain_sequence > ?) OR (chain_sequence = ? AND id > ?)", it.sequence, it.sequence, it.id)
			}
		}
		// Do not use a zero-valued initial cursor: verification must also see
		// corrupt negative/zero sequences, including duplicates at a boundary.
		it.batch = make([]auditEventRow, 0, auditScanBatchSize)
		if err := query.Find(&it.batch).Error; err != nil {
			return auditEventRow{}, false, err
		}
		it.position, it.started = 0, true
		it.done = len(it.batch) < auditScanBatchSize
		if len(it.batch) == 0 {
			return auditEventRow{}, false, nil
		}
	}
	row := it.batch[it.position]
	it.position++
	it.sequence, it.id = row.ChainSequence, row.ID
	return row, true, nil
}

type auditCheckpointIterator struct {
	tx       *gorm.DB
	batch    []auditCheckpointRow
	position int
	started  bool
	done     bool
	sequence int64
	id       int64
}

func (it *auditCheckpointIterator) next() (auditCheckpointRow, bool, error) {
	if it.position == len(it.batch) {
		if it.done {
			return auditCheckpointRow{}, false, nil
		}
		query := it.tx.Where("chain_id=?", audit.DefaultChainID).Order("last_sequence ASC, id ASC").Limit(auditScanBatchSize)
		if it.started {
			query = query.Where("(last_sequence > ?) OR (last_sequence = ? AND id > ?)", it.sequence, it.sequence, it.id)
		}
		it.batch = make([]auditCheckpointRow, 0, auditScanBatchSize)
		if err := query.Find(&it.batch).Error; err != nil {
			return auditCheckpointRow{}, false, err
		}
		it.position, it.started = 0, true
		it.done = len(it.batch) < auditScanBatchSize
		if len(it.batch) == 0 {
			return auditCheckpointRow{}, false, nil
		}
	}
	row := it.batch[it.position]
	it.position++
	it.sequence, it.id = row.LastSequence, row.ID
	return row, true, nil
}
