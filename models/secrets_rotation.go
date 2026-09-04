package models

import (
	"fmt"

	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
)

type protectedColumn struct {
	ID    int64  `gorm:"column:id"`
	Value string `gorm:"column:value"`
}

// RotateSecrets re-encrypts integration and retained-credential ciphertexts
// with the active key. Old keys must remain configured until this completes.
func RotateSecrets() (int64, error) {
	store, ok := secretStore.(secretpkg.VersionedStore)
	if !ok {
		return 0, ErrCredentialEncryption
	}
	targets := []struct {
		table  string
		id     string
		column string
	}{
		{table: "smtp", id: "id", column: "password"},
		{table: "imap", id: "user_id", column: "password"},
		{table: "webhooks", id: "id", column: "secret"},
		{table: "encrypted_credentials", id: "id", column: "encrypted_value"},
	}
	var rotated int64
	for _, target := range targets {
		rows := []protectedColumn{}
		selectSQL := fmt.Sprintf("%s AS id, %s AS value", target.id, target.column)
		if err := db.Table(target.table).Select(selectSQL).Where(target.column + " <> ''").Scan(&rows).Error; err != nil {
			return rotated, err
		}
		for _, row := range rows {
			if !store.NeedsRewrap(row.Value) {
				continue
			}
			plaintext, err := store.Open(row.Value)
			if err != nil {
				return rotated, fmt.Errorf("open %s row %d: %w", target.table, row.ID, err)
			}
			protected, err := store.Seal(plaintext)
			if err != nil {
				return rotated, fmt.Errorf("seal %s row %d: %w", target.table, row.ID, err)
			}
			if err := db.Table(target.table).Where(target.id+"=?", row.ID).Update(target.column, protected).Error; err != nil {
				return rotated, err
			}
			rotated++
		}
	}
	return rotated, nil
}
