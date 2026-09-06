package models

import (
	"time"

	"gorm.io/gorm"
)

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func (u *User) BeforeSave(_ *gorm.DB) error {
	if u.LastLogin != nil {
		u.LastLogin = nullableTime(*u.LastLogin)
	}
	return nil
}

func (u *User) AfterFind(tx *gorm.DB) error { return u.BeforeSave(tx) }

func (im *IMAP) BeforeSave(tx *gorm.DB) error {
	ensureModifiedTime(&im.ModifiedDate)
	return im.AfterFind(tx)
}

func (im *IMAP) AfterFind(_ *gorm.DB) error {
	if im.LastLogin != nil {
		im.LastLogin = nullableTime(*im.LastLogin)
	}
	return nil
}

func (c *Campaign) AfterFind(_ *gorm.DB) error {
	if c.SendByDate != nil {
		c.SendByDate = nullableTime(*c.SendByDate)
	}
	if c.CompletedDate != nil {
		c.CompletedDate = nullableTime(*c.CompletedDate)
	}
	return nil
}

// Table(...).Scan does not invoke Gorm model hooks for summary projections.
func (c *CampaignSummary) normalizeOptionalDates() {
	if c.SendByDate != nil {
		c.SendByDate = nullableTime(*c.SendByDate)
	}
	if c.CompletedDate != nil {
		c.CompletedDate = nullableTime(*c.CompletedDate)
	}
}
