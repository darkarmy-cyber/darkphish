package models

import "time"

// Modification is a real event whenever these records are saved. Model-level
// defaults also cover imports and callers that do not pass through HTTP handlers.
func ensureModifiedTime(value *time.Time) {
	if value.IsZero() {
		*value = time.Now().UTC()
	}
}

func (g *Group) BeforeSave() error {
	ensureModifiedTime(&g.ModifiedDate)
	return nil
}

func (t *Template) BeforeSave() error {
	ensureModifiedTime(&t.ModifiedDate)
	return nil
}

func (p *Page) BeforeSave() error {
	ensureModifiedTime(&p.ModifiedDate)
	return nil
}

func (s *SMTP) BeforeSave() error {
	ensureModifiedTime(&s.ModifiedDate)
	return nil
}
