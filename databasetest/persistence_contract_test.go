package databasetest

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/jinzhu/gorm"
)

// Test-only table: deliberately explicit DDL, never GORM AutoMigrate. The same
// contract runs within each existing hosted security-model fixture.
type persistenceContractRow struct {
	ID         int64
	ScopeID    int64
	Name       string
	Label      string
	Enabled    bool
	Amount     int64
	OptionalAt *time.Time
	Payload    []byte
}

func (persistenceContractRow) TableName() string { return "persistence_contract_rows" }

func exercisePersistenceContract(t *testing.T, backend, dsn string, ownerID int64) {
	t.Helper()
	orm, err := gorm.Open(backend, dsn)
	if err != nil {
		t.Fatal("open persistence contract connection")
	}
	defer orm.Close()
	orm.LogMode(false)
	id, boolean, timestamp, blob := "INTEGER PRIMARY KEY AUTOINCREMENT", "BOOLEAN", "DATETIME", "BLOB"
	switch backend {
	case "mysql":
		id, timestamp, blob = "BIGINT PRIMARY KEY AUTO_INCREMENT", "DATETIME(6)", "LONGBLOB"
	case "postgres":
		id, timestamp, blob = "BIGSERIAL PRIMARY KEY", "TIMESTAMPTZ", "BYTEA"
	}
	ddl := "CREATE TABLE persistence_contract_rows (id " + id + ", scope_id BIGINT NOT NULL, name VARCHAR(100) NOT NULL UNIQUE, label TEXT NOT NULL, enabled " + boolean + " NOT NULL, amount BIGINT NOT NULL, optional_at " + timestamp + " NULL, payload " + blob + ")"
	if err := orm.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := orm.Exec("DROP TABLE persistence_contract_rows").Error; err != nil {
			t.Error(err)
		}
	}()
	stamp := time.Date(2026, 9, 6, 10, 11, 12, 123456000, time.UTC)
	wantBytes := []byte{0, 1, 127, 128, 255, '\n'}
	first := persistenceContractRow{ScopeID: 7, Name: "contract-first", Label: "nonempty", Enabled: true, Amount: 9, OptionalAt: &stamp, Payload: wantBytes}
	created := orm.Create(&first)
	if created.Error != nil || created.RowsAffected != 1 || first.ID == 0 {
		t.Fatalf("generated identity/create failed: %v", created.Error)
	}
	var stored persistenceContractRow
	if err := orm.Where("id=? AND scope_id=?", first.ID, 7).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.OptionalAt == nil || !stored.OptionalAt.Equal(stamp) || !bytes.Equal(stored.Payload, wantBytes) {
		t.Fatal("timestamp precision or binary round trip changed")
	}
	missing := orm.Where("id=? AND scope_id=?", first.ID, 8).First(&persistenceContractRow{})
	if !errors.Is(missing.Error, gorm.ErrRecordNotFound) || missing.RowsAffected != 0 {
		t.Fatal("ownership/missing-row contract changed")
	}
	var absent []persistenceContractRow
	if err := orm.Where("scope_id=?", 999).Find(&absent).Error; err != nil || len(absent) != 0 {
		t.Fatal("empty collection is not successful")
	}
	bad := orm.Table("contract_missing_table").First(&persistenceContractRow{}).Error
	if bad == nil || errors.Is(bad, gorm.ErrRecordNotFound) {
		t.Fatal("database failure confused with missing record")
	}
	duplicate := persistenceContractRow{ScopeID: 7, Name: first.Name}
	if err := orm.Create(&duplicate).Error; err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("unique conflict was not distinguished")
	}
	updated := orm.Model(&persistenceContractRow{}).Where("id=? AND scope_id=?", first.ID, 7).Updates(map[string]interface{}{"label": "", "enabled": false, "amount": 0, "optional_at": nil})
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("zero-value map update failed: %v", updated.Error)
	}
	stored = persistenceContractRow{}
	if err := orm.Where("id=?", first.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Label != "" || stored.Enabled || stored.Amount != 0 || stored.OptionalAt != nil || !bytes.Equal(stored.Payload, wantBytes) {
		t.Fatal("zero values or NULL did not persist")
	}
	stored.Enabled, stored.Label, stored.Amount, stored.OptionalAt = true, "again", 2, &stamp
	if err := orm.Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	stored.Enabled, stored.Label, stored.Amount, stored.OptionalAt = false, "", 0, nil
	if err := orm.Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	stored = persistenceContractRow{}
	if err := orm.Where("id=?", first.ID).First(&stored).Error; err != nil || stored.Enabled || stored.Label != "" || stored.Amount != 0 || stored.OptionalAt != nil {
		t.Fatal("Save skipped explicit zero values")
	}
	explicit := persistenceContractRow{ID: 900001, ScopeID: 7, Name: "contract-explicit"}
	if err := orm.Create(&explicit).Error; err != nil || explicit.ID != 900001 {
		t.Fatal("explicit identity changed")
	}
	for i, name := range []string{"contract-b", "contract-a", "contract-c"} {
		row := persistenceContractRow{ScopeID: 8, Name: name, Amount: int64(i)}
		if err := orm.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	var page []persistenceContractRow
	if err := orm.Where("scope_id=?", 8).Order("name ASC").Limit(1).Offset(1).Find(&page).Error; err != nil || len(page) != 1 || page[0].Name != "contract-b" {
		t.Fatal("explicit ordering/pagination changed")
	}
	base := orm.Model(&persistenceContractRow{}).Where("scope_id=?", 8)
	var countA, countB int64
	if err := base.Where("name=?", "contract-a").Count(&countA).Error; err != nil {
		t.Fatal(err)
	}
	if err := base.Where("name=?", "contract-b").Count(&countB).Error; err != nil || countA != 1 || countB != 1 {
		t.Fatal("reused query accumulated previous predicates")
	}
	transaction := orm.Begin()
	if transaction.Error != nil {
		t.Fatal(transaction.Error)
	}
	rolledBack := persistenceContractRow{ScopeID: 7, Name: "contract-rollback"}
	if err := transaction.Create(&rolledBack).Error; err != nil {
		transaction.Rollback()
		t.Fatal(err)
	}
	if err := transaction.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Where("name=?", rolledBack.Name).First(&persistenceContractRow{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("rolled-back mutation persisted")
	}
	transaction = orm.Begin()
	committed := persistenceContractRow{ScopeID: 7, Name: "contract-commit"}
	if err := transaction.Create(&committed).Error; err != nil {
		transaction.Rollback()
		t.Fatal(err)
	}
	if err := transaction.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Where("name=?", committed.Name).First(&persistenceContractRow{}).Error; err != nil {
		t.Fatal("committed mutation missing")
	}
	if err := transaction.Create(&persistenceContractRow{ScopeID: 7, Name: "contract-after-commit"}).Error; err == nil {
		t.Fatal("finished transaction remained writable")
	}
	deleted := orm.Where("id=? AND scope_id=?", first.ID, 8).Delete(&persistenceContractRow{})
	if deleted.Error != nil || deleted.RowsAffected != 0 {
		t.Fatal("delete crossed scope")
	}
	deleted = orm.Where("id=? AND scope_id=?", first.ID, 7).Delete(&persistenceContractRow{})
	if deleted.Error != nil || deleted.RowsAffected != 1 {
		t.Fatal("delete result changed")
	}
	if err := orm.Where("id=?", first.ID).First(&persistenceContractRow{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("deleted row remained readable")
	}
	exerciseModelHookContract(t, ownerID)
}

func exerciseModelHookContract(t *testing.T, ownerID int64) {
	t.Helper()
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	originalRoleName := role.Name
	role.Name = "must not overwrite the role association"
	zero := time.Time{}
	user := models.User{Username: "contract-hook-user", ApiKey: "disabled-contract-hook", Role: role, RoleID: role.ID, LastLogin: &zero, AccountLocked: true, PasswordChangeRequired: true}
	if err := models.PutUser(&user); err != nil {
		t.Fatal(err)
	}
	loaded, err := models.GetUser(user.Id)
	if err != nil || loaded.LastLogin != nil || loaded.Role.Name != originalRoleName {
		t.Fatal("nullable hook or read-only role association changed")
	}
	loaded.AccountLocked, loaded.PasswordChangeRequired, loaded.Hash = false, false, ""
	if err := models.PutUser(&loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = models.GetUser(user.Id)
	if err != nil || loaded.AccountLocked || loaded.PasswordChangeRequired || loaded.Hash != "" {
		t.Fatal("security zero-value Save failed")
	}
	page := models.Page{UserId: ownerID, Name: "contract-hook-page", HTML: "<p>synthetic</p>", CaptureCredentials: true, CapturePasswords: true}
	if err := models.PostPage(&page); err != nil || page.ModifiedDate.IsZero() {
		t.Fatal("BeforeSave modification hook did not run")
	}
	page.CaptureCredentials, page.CapturePasswords = false, false
	if err := models.PutPage(&page); err != nil {
		t.Fatal(err)
	}
	reloaded, err := models.GetPage(page.Id, ownerID)
	if err != nil || reloaded.CaptureCredentials || reloaded.CapturePasswords {
		t.Fatal("page boolean transition changed")
	}
	if _, err := models.GetPage(page.Id, ownerID+10000); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("page ownership/missing-row distinction changed")
	}
}
