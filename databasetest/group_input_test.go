package databasetest

import (
	"fmt"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

// This helper runs inside the same SQLite, strict MySQL and PostgreSQL fixtures
// as credential, reviewer and audit persistence. Values are synthetic data.
func exerciseGroupInputIsolation(t *testing.T, userID int64) {
	t.Helper()
	recipients := []models.BaseRecipient{
		{Email: "query-boundary@example.test", FirstName: "quoted ' OR '1'='1", LastName: "semi;--", Position: "100%_literal"},
		{Email: "query-boundary@example.test"},
		{Email: "other-query@example.test", FirstName: "ordinary"},
	}
	var ids []int64
	for i, recipient := range recipients {
		group := models.Group{Name: fmt.Sprintf("query isolation %d", i), UserId: userID, Targets: []models.Target{{BaseRecipient: recipient}}}
		if err := models.PostGroup(&group); err != nil {
			t.Fatal(err)
		}
		stored, err := models.GetGroup(group.Id, userID)
		if err != nil || len(stored.Targets) != 1 {
			t.Fatalf("recipient round trip failed: %v", err)
		}
		if stored.Targets[0].BaseRecipient != recipient {
			t.Fatal("recipient predicate reused or altered different data")
		}
		ids = append(ids, stored.Targets[0].Id)
		if _, err := models.GetGroup(group.Id, userID+10000); err == nil {
			t.Fatal("group crossed owner boundary")
		}
	}
	if ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatal("distinct recipient identities merged")
	}
	duplicate := models.Group{Name: "query exact reuse", UserId: userID, Targets: []models.Target{{BaseRecipient: recipients[0]}}}
	if err := models.PostGroup(&duplicate); err != nil {
		t.Fatal(err)
	}
	stored, err := models.GetGroup(duplicate.Id, userID)
	if err != nil || len(stored.Targets) != 1 || stored.Targets[0].Id != ids[0] {
		t.Fatal("exact recipient reuse changed")
	}
}
