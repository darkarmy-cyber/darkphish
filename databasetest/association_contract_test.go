package databasetest

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

// Fixed read-only queries cover all manually maintained campaign dependencies.
// Only a hash is compared/reported; protected configuration values stay local.
func associationFingerprint(t *testing.T, connection *sql.DB, backend string) [32]byte {
	t.Helper()
	hash := sha256.New()
	groupsQuery := "SELECT * FROM \"groups\" ORDER BY id"
	if backend == "mysql" {
		groupsQuery = "SELECT * FROM `groups` ORDER BY id"
	}
	for _, query := range []string{
		groupsQuery,
		"SELECT * FROM targets ORDER BY id",
		"SELECT * FROM group_targets ORDER BY group_id, target_id",
		"SELECT * FROM templates ORDER BY id",
		"SELECT * FROM attachments ORDER BY id",
		"SELECT * FROM pages ORDER BY id",
		"SELECT * FROM smtp ORDER BY id",
		"SELECT * FROM headers ORDER BY id",
	} {
		rows, err := connection.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			pointers := make([]interface{}, len(columns))
			for i := range pointers {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if err := json.NewEncoder(hash).Encode(values); err != nil {
				rows.Close()
				t.Fatal(err)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func exerciseSummaryOwnership(t *testing.T, campaign models.Campaign, group models.Group) {
	t.Helper()
	if summary, err := models.GetCampaignSummary(campaign.Id, campaign.UserId); err != nil || summary.Id != campaign.Id || summary.Stats.Total != int64(len(campaign.Results)) {
		t.Fatal("owned campaign summary failed")
	}
	if summary, err := models.GetCampaignSummary(campaign.Id, campaign.UserId+10000); !errors.Is(err, models.ErrRecordNotFound) || summary.Id != 0 || summary.Stats.Total != 0 {
		t.Fatal("campaign summary exposed another owner's data")
	}
	if summary, err := models.GetGroupSummary(group.Id, group.UserId+10000); !errors.Is(err, models.ErrRecordNotFound) || summary.Id != 0 || summary.NumTargets != 0 {
		t.Fatal("group summary exposed another owner's recipient count")
	}
}
