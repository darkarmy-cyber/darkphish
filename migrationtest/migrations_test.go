package migrationtest

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

func migrationDirectory(t *testing.T, database string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve migration test path")
	}
	return filepath.Join(filepath.Dir(file), "..", "db", "db_"+database, "migrations")
}

func exerciseLatestMigration(t *testing.T, driver, dialect, dsn, migrations string) {
	t.Helper()
	database, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if driver == "sqlite3" {
		database.SetMaxOpenConns(1)
	}
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(database, migrations); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	assertSecuritySchema(t, database)
	if err := goose.Down(database, migrations); err != nil {
		t.Fatalf("migrate latest down: %v", err)
	}
	if _, err := database.Exec("SELECT COUNT(*) FROM audit_chain_heads"); err == nil {
		t.Fatal("audit_chain_heads still exists after rolling back the 0.6 migration")
	}
	if err := goose.Up(database, migrations); err != nil {
		t.Fatalf("migrate up after rollback: %v", err)
	}
	assertSecuritySchema(t, database)
}

func assertSecuritySchema(t *testing.T, database *sql.DB) {
	t.Helper()
	for _, table := range []string{"campaign_credential_policies", "credential_policy_results", "encrypted_credentials", "personal_access_tokens", "audit_events", "privileged_sessions", "campaign_reviewers", "audit_outbox", "audit_checkpoints", "audit_chain_heads", "audit_delivery_receipts"} {
		if _, err := database.Exec("SELECT COUNT(*) FROM " + table); err != nil {
			t.Fatalf("security table %s is unavailable: %v", table, err)
		}
	}
	if _, err := database.Exec("SELECT credential_capture_mode, credential_retention_hours FROM campaigns LIMIT 0"); err != nil {
		t.Fatalf("campaign credential controls are unavailable: %v", err)
	}
	if _, err := database.Exec("SELECT min_uppercase, min_lowercase, min_digits, min_symbols FROM campaign_credential_policies LIMIT 0"); err != nil {
		t.Fatalf("campaign credential policy counters are unavailable: %v", err)
	}
	if _, err := database.Exec("SELECT chain_id, chain_sequence, previous_hash, event_hash, audit_outbox_id FROM audit_events LIMIT 0"); err != nil {
		t.Fatalf("audit chain columns are unavailable: %v", err)
	}
	var permissions int
	if err := database.QueryRow("SELECT COUNT(*) FROM permissions WHERE slug='credentials:view'").Scan(&permissions); err != nil || permissions != 1 {
		t.Fatalf("credentials:view permission missing: count=%d err=%v", permissions, err)
	}
}

func TestPostgreSQLMigrationsUpDownUp(t *testing.T) {
	dsn := os.Getenv("DARKPHISH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DARKPHISH_TEST_POSTGRES_DSN is not configured")
	}
	exerciseLatestMigration(t, "pgx", "postgres", dsn, migrationDirectory(t, "postgres"))
}

func TestSQLiteMigrationsUpDownUp(t *testing.T) {
	exerciseLatestMigration(t, "sqlite3", "sqlite3", filepath.Join(t.TempDir(), "migration.db"), migrationDirectory(t, "sqlite3"))
}

func TestMySQLMigrationsUpDownUp(t *testing.T) {
	dsn := os.Getenv("DARKPHISH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("DARKPHISH_TEST_MYSQL_DSN is not configured")
	}
	exerciseLatestMigration(t, "mysql", "mysql", dsn, migrationDirectory(t, "mysql"))
}
