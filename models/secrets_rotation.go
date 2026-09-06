package models

import (
	"fmt"
	"strconv"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	"gorm.io/gorm"
)

type protectedColumn struct {
	ID    int64  `gorm:"column:id"`
	Value string `gorm:"column:value"`
}

type SecretStatus struct {
	LegacyKeyRecords    int64            `json:"legacy_key_records"`
	CurrentLocalRecords int64            `json:"current_local_records"`
	ExternalRecords     int64            `json:"external_provider_records"`
	PlaintextRecords    int64            `json:"plaintext_records"`
	RequiringMigration  int64            `json:"requiring_migration"`
	ByProviderAndKey    map[string]int64 `json:"by_provider_and_key"`
}

func protectedTargets() []struct {
	table  string
	id     string
	column string
} {
	return []struct {
		table  string
		id     string
		column string
	}{
		{table: "smtp", id: "id", column: "password"},
		{table: "imap", id: "user_id", column: "password"},
		{table: "webhooks", id: "id", column: "secret"},
		{table: "encrypted_credentials", id: "id", column: "encrypted_value"},
	}
}

func SecretsStatus() (SecretStatus, error) {
	status := SecretStatus{ByProviderAndKey: make(map[string]int64)}
	store, versioned := secretStore.(secretpkg.VersionedStore)
	for _, target := range protectedTargets() {
		rows := []protectedColumn{}
		selectSQL := fmt.Sprintf("%s AS id, %s AS value", target.id, target.column)
		if err := db.Table(target.table).Select(selectSQL).Where(target.column + " <> ''").Scan(&rows).Error; err != nil {
			return status, err
		}
		for _, row := range rows {
			metadata, err := secretpkg.InspectEnvelope(row.Value)
			if err != nil {
				status.RequiringMigration++
				continue
			}
			key := metadata.Provider + ":" + metadata.KeyID
			status.ByProviderAndKey[key]++
			switch {
			case !metadata.Encrypted:
				status.PlaintextRecords++
			case metadata.Version == 1 || metadata.KeyID == "legacy":
				status.LegacyKeyRecords++
			case metadata.Provider == "local":
				status.CurrentLocalRecords++
			default:
				status.ExternalRecords++
			}
			if !versioned || store.NeedsRewrap(row.Value) {
				status.RequiringMigration++
			}
		}
	}
	return status, nil
}

// RotateSecrets re-encrypts integration and retained-credential ciphertexts
// with the active key. Old keys must remain configured until this completes.
func RotateSecrets() (int64, error) {
	store, ok := secretStore.(secretpkg.VersionedStore)
	if !ok {
		return 0, ErrCredentialEncryption
	}
	var rotated int64
	for _, target := range protectedTargets() {
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
			event := audit.Event{
				Timestamp: time.Now().UTC(), Actor: "darkphish", ActorType: "system", Action: "secret.migrate",
				TargetType: target.table, TargetID: strconv.FormatInt(row.ID, 10), Result: "success",
				RequestID: audit.NewRequestID(), AuthMethod: "system", Metadata: "{}",
			}
			if err := withSecurityTransaction(func(tx *gorm.DB) error {
				if err := tx.Table(target.table).Where(target.id+"=?", row.ID).Update(target.column, protected).Error; err != nil {
					return err
				}
				return (gormAuditRepository{db: tx}).Enqueue(event)
			}); err != nil {
				return rotated, err
			}
			flushAuditOutboxAfterCommit()
			rotated++
		}
	}
	return rotated, nil
}
