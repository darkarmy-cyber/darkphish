package models

import "time"

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func (u *User) BeforeSave() error {
	if u.LastLogin != nil {
		u.LastLogin = nullableTime(*u.LastLogin)
	}
	return nil
}

func (u *User) AfterFind() error { return u.BeforeSave() }

func (im *IMAP) BeforeSave() error {
	ensureModifiedTime(&im.ModifiedDate)
	return im.AfterFind()
}

func (im *IMAP) AfterFind() error {
	if im.LastLogin != nil {
		im.LastLogin = nullableTime(*im.LastLogin)
	}
	return nil
}

func (c *Campaign) AfterFind() error {
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
