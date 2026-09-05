package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/jinzhu/gorm"
)

type auditOutboxRow struct {
	ID           int64      `gorm:"primary_key"`
	EventJSON    string     `gorm:"column:event_json"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	DispatchedAt *time.Time `gorm:"column:dispatched_at"`
	Attempts     int        `gorm:"column:attempts"`
	LastError    string     `gorm:"column:last_error"`
}

func flushAuditOutboxAfterCommit() {
	if err := FlushAuditOutbox(); err != nil {
		log.Errorf("audit outbox delivery failed; event retained for retry: %v", err)
	}
}

func (auditOutboxRow) TableName() string { return "audit_outbox" }

func enqueueAuditEvent(database *gorm.DB, event audit.Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Metadata == "" {
		event.Metadata = "{}"
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	row := auditOutboxRow{EventJSON: string(encoded), CreatedAt: time.Now().UTC()}
	if err := database.Create(&row).Error; err != nil {
		return err
	}
	event.OutboxID = row.ID
	encoded, err = json.Marshal(event)
	if err != nil {
		return err
	}
	return database.Model(&auditOutboxRow{}).Where("id=?", row.ID).Update("event_json", string(encoded)).Error
}

// FlushAuditOutbox retries durable security events after the business
// transaction commits. A failed append remains pending and is retried on the
// next sensitive operation and at startup.
func FlushAuditOutbox() error {
	rows := []auditOutboxRow{}
	if err := db.Where("dispatched_at IS NULL").Order("id ASC").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var event audit.Event
		if err := json.Unmarshal([]byte(row.EventJSON), &event); err != nil {
			message := strings.ToValidUTF8(err.Error(), "")
			if len(message) > 512 {
				message = message[:512]
			}
			_ = db.Model(&auditOutboxRow{}).Where("id=?", row.ID).Updates(map[string]interface{}{"attempts": row.Attempts + 1, "last_error": message}).Error
			continue
		}
		if err := audit.AppendEvent(event); err != nil {
			message := strings.ToValidUTF8(err.Error(), "")
			if len(message) > 512 {
				message = message[:512]
			}
			_ = db.Model(&auditOutboxRow{}).Where("id=?", row.ID).Updates(map[string]interface{}{"attempts": row.Attempts + 1, "last_error": message}).Error
			return err
		}
		now := time.Now().UTC()
		if err := db.Model(&auditOutboxRow{}).Where("id=? AND dispatched_at IS NULL", row.ID).
			Updates(map[string]interface{}{"dispatched_at": now, "attempts": row.Attempts + 1, "last_error": ""}).Error; err != nil {
			return err
		}
	}
	return nil
}
