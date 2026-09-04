package models

import (
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"gopkg.in/check.v1"
)

func (s *ModelsSuite) TestPersistentAuditStoreFiltersAndRetention(c *check.C) {
	audit.RecordSystem("retention.cleanup", "credentials", "2", "success")
	events, total, err := audit.Query(audit.Filter{Action: "retention.cleanup", Actor: "darkphish", Page: 1, PerPage: 10})
	c.Assert(err, check.IsNil)
	c.Assert(total, check.Equals, int64(1))
	c.Assert(events, check.HasLen, 1)
	c.Assert(events[0].ActorType, check.Equals, "system")

	row := auditEventRow{Timestamp: time.Now().UTC().Add(-400 * 24 * time.Hour), Actor: "old", ActorType: "system", Action: "old", TargetType: "system", TargetID: "old", Result: "success", RequestID: "old", Metadata: "{}"}
	c.Assert(db.Create(&row).Error, check.IsNil)
	removed, err := audit.DeleteBefore(time.Now().UTC().Add(-365 * 24 * time.Hour))
	c.Assert(err, check.IsNil)
	c.Assert(removed, check.Equals, int64(1))
}
