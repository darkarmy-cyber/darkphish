package models

import (
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

// All writers take the same database row lock before reading usage and retain
// it through commit. READ COMMITTED prevents a waiter from using an old MySQL
// snapshot. SQLite takes its writer lock with the UPDATE before any reads.
func beginLicenseTransaction() *gorm.DB {
	options := &sql.TxOptions{}
	if db.Dialector.Name() != "sqlite" {
		options.Isolation = sql.LevelReadCommitted
	}
	return db.Begin(options)
}

func lockLicenseUsage(tx *gorm.DB) error {
	if currentLicenseManager() == nil {
		return nil
	}
	if err := tx.Exec("UPDATE license_coordination SET id=id WHERE id=1").Error; err != nil {
		return err
	}
	var row struct{ ID int }
	if err := tx.Table("license_coordination").Where("id=1").Take(&row).Error; err != nil {
		return err
	}
	if row.ID != 1 {
		return errors.New("license coordination row is unavailable")
	}
	return nil
}
