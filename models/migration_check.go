package models

import (
	"fmt"
	"os"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/persistence"
)

func LegacyAPIKeyRecordCount() (int64, error) {
	var count int64
	err := db.Model(&User{}).Where("api_key NOT LIKE ?", "disabled-%").Count(&count).Error
	return count, err
}

// InspectLegacyDatabase performs read-only compatibility counts without
// applying migrations, creating users, rotating secrets, or changing records.
func InspectLegacyDatabase(conf *config.Config) (int64, int64, error) {
	if conf.DBName == "sqlite3" && conf.DBPath != ":memory:" {
		if _, err := os.Stat(conf.DBPath); err != nil {
			if os.IsNotExist(err) {
				return 0, 0, nil
			}
			return 0, 0, fmt.Errorf("inspect database path: %w", err)
		}
	}
	inspection, err := persistence.Open(conf)
	if err != nil {
		return 0, 0, fmt.Errorf("open database for read-only migration inspection: %w", err)
	}
	connection, err := inspection.DB()
	if err != nil {
		return 0, 0, err
	}
	defer connection.Close()
	if conf.DBName == "sqlite3" {
		if err := inspection.Exec("PRAGMA query_only = ON").Error; err != nil {
			return 0, 0, fmt.Errorf("enable read-only SQLite inspection: %w", err)
		}
	}
	var legacyAPIKeys int64
	if inspection.Migrator().HasTable("users") {
		if err := inspection.Table("users").Where("api_key NOT LIKE ?", "disabled-%").Count(&legacyAPIKeys).Error; err != nil {
			return 0, 0, err
		}
	}
	var plaintext int64
	for _, target := range protectedTargets() {
		if !inspection.Migrator().HasTable(target.table) {
			continue
		}
		var count int64
		condition := target.column + " <> '' AND " + target.column + " NOT LIKE ?"
		if err := inspection.Table(target.table).Where(condition, "darkphish:secret:%").Count(&count).Error; err != nil {
			return 0, 0, err
		}
		plaintext += count
	}
	return legacyAPIKeys, plaintext, nil
}
