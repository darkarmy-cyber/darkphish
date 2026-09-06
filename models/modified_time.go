package models

import (
	"time"

	"gorm.io/gorm"
)

// Modification is a real event whenever these records are saved. Model-level
// defaults also cover imports and callers that do not pass through HTTP handlers.
func ensureModifiedTime(value *time.Time) {
	if value.IsZero() {
		*value = time.Now().UTC()
	}
}

func (g *Group) BeforeSave(_ *gorm.DB) error {
	ensureModifiedTime(&g.ModifiedDate)
	return nil
}

func (t *Template) BeforeSave(_ *gorm.DB) error {
	ensureModifiedTime(&t.ModifiedDate)
	return nil
}

func (p *Page) BeforeSave(_ *gorm.DB) error {
	ensureModifiedTime(&p.ModifiedDate)
	return nil
}

func (s *SMTP) BeforeSave(_ *gorm.DB) error {
	ensureModifiedTime(&s.ModifiedDate)
	return nil
}
