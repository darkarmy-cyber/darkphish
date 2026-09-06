package databasetest

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

// The same tests exercise the actual v2 transaction implementation on every
// supported backend; no mock transaction can hide a driver-specific rollback.
func exerciseTransactionContract(t *testing.T, orm *gorm.DB) {
	t.Helper()
	injected := errors.New("synthetic transaction failure")
	create := func(tx *gorm.DB, name string) error {
		return tx.Create(&persistenceContractRow{ScopeID: 55, Name: name}).Error
	}
	err := orm.Transaction(func(tx *gorm.DB) error {
		if err := create(tx, "contract-outer-before"); err != nil {
			return err
		}
		inner := tx.Transaction(func(nested *gorm.DB) error {
			if err := create(nested, "contract-inner-rollback"); err != nil {
				return err
			}
			return injected
		})
		if !errors.Is(inner, injected) {
			return errors.New("nested transaction lost original error")
		}
		return create(tx, "contract-outer-after")
	})
	if err != nil {
		t.Fatal(err)
	}
	var outer int64
	if err := orm.Model(&persistenceContractRow{}).Where("scope_id=?", 55).Count(&outer).Error; err != nil || outer != 2 {
		t.Fatal("nested rollback did not preserve exactly the outer writes")
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != injected {
				t.Fatal("transaction changed or swallowed panic")
			}
		}()
		_ = orm.Transaction(func(tx *gorm.DB) error {
			if err := create(tx, "contract-panic-rollback"); err != nil {
				t.Fatal(err)
			}
			panic(injected)
		})
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := create(tx, "contract-cancel-rollback"); err != nil {
			return err
		}
		cancel() // Commit must fail and the already-written row must roll back.
		return nil
	})
	if err == nil {
		t.Fatal("canceled transaction reported successful commit")
	}
	for _, name := range []string{"contract-inner-rollback", "contract-panic-rollback", "contract-cancel-rollback"} {
		if err := orm.Where("name=?", name).Take(&persistenceContractRow{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("failed transaction persisted %s: %v", name, err)
		}
	}
}
