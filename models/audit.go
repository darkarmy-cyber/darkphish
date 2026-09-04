package models

import (
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/jinzhu/gorm"
)

type auditEventRow struct {
	ID         int64 `gorm:"primary_key"`
	Timestamp  time.Time
	Actor      string
	ActorID    int64
	ActorType  string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	RequestID  string
	SourceIP   string
	UserAgent  string
	AuthMethod string
	Metadata   string
}

func (auditEventRow) TableName() string { return "audit_events" }

type databaseAuditStore struct{}

func eventRow(event audit.Event) auditEventRow {
	return auditEventRow{
		ID: event.ID, Timestamp: event.Timestamp, Actor: event.Actor, ActorID: event.ActorID,
		ActorType: event.ActorType, Action: event.Action, TargetType: event.TargetType,
		TargetID: event.TargetID, Result: event.Result, RequestID: event.RequestID,
		SourceIP: event.SourceIP, UserAgent: event.UserAgent, AuthMethod: event.AuthMethod,
		Metadata: event.Metadata,
	}
}

func rowEvent(row auditEventRow) audit.Event {
	return audit.Event{
		ID: row.ID, Timestamp: row.Timestamp, Actor: row.Actor, ActorID: row.ActorID,
		ActorType: row.ActorType, Action: row.Action, TargetType: row.TargetType,
		TargetID: row.TargetID, Result: row.Result, RequestID: row.RequestID,
		SourceIP: row.SourceIP, UserAgent: row.UserAgent, AuthMethod: row.AuthMethod,
		Metadata: row.Metadata,
	}
}

func (databaseAuditStore) Append(event audit.Event) (int64, error) {
	row := eventRow(event)
	err := db.Create(&row).Error
	return row.ID, err
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
	query := applyAuditFilter(db.Model(&auditEventRow{}), filter)
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
	query := db.Where("timestamp < ?", before.UTC()).Delete(&auditEventRow{})
	return query.RowsAffected, query.Error
}

func configureAuditStore() { audit.SetStore(databaseAuditStore{}) }
