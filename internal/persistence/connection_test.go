package persistence

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

func TestConnectionDoesNotMigrateAndPreservesPool(t *testing.T) {
	for _, inMemory := range []bool{true, false} {
		dsn := filepath.Join(t.TempDir(), "connection.db")
		if inMemory {
			dsn = ":memory:"
		}
		database, err := Open(&config.Config{DBName: "sqlite3", DBPath: dsn})
		if err != nil {
			t.Fatal(err)
		}
		connection, err := database.DB()
		if err != nil {
			t.Fatal(err)
		}
		if connection.Stats().MaxOpenConnections != 1 {
			t.Fatal("SQLite default pool changed")
		}
		var count int64
		if err := database.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("opening ORM silently created tables")
		}
		if err := database.Exec("CREATE TABLE connection_fixture (id INTEGER PRIMARY KEY, enabled BOOLEAN)").Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Table("connection_fixture").Update("enabled", false).Error; !errors.Is(err, gorm.ErrMissingWhereClause) {
			t.Fatal("global write protection was disabled")
		}
		if err := connection.Ping(); err != nil {
			t.Fatal(err)
		}
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
		if err := connection.Ping(); err == nil {
			t.Fatal("database close did not release the connection")
		}
	}
	database, err := Open(&config.Config{DBName: "sqlite3", DBPath: filepath.Join(t.TempDir(), "pool.db"), DBMaxOpenConns: 3, DBMaxIdleConns: 2, DBConnMaxLifetimeMinutes: 1})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if connection.Stats().MaxOpenConnections != 3 {
		t.Fatal("explicit pool limit ignored")
	}
}

func TestConnectionErrorsDoNotExposeDSN(t *testing.T) {
	for _, test := range []struct {
		config config.Config
		want   error
	}{
		{config.Config{DBName: "unknown", DBPath: "synthetic-password-marker"}, ErrBackend},
		{config.Config{DBName: "postgres", DBPath: "postgres://synthetic:synthetic-password-marker@%invalid"}, ErrConnection},
		{config.Config{DBName: "mysql", DBPath: "synthetic-password-marker"}, ErrConnection},
		{config.Config{DBName: "mysql", DBPath: "synthetic-password-marker", DBSSLCaPath: filepath.Join(t.TempDir(), "missing-ca")}, ErrTrust},
	} {
		if _, err := Open(&test.config); !errors.Is(err, test.want) || err.Error() != test.want.Error() {
			t.Fatal("connection failure was not safely classified")
		}
	}
}

func TestPostgreSQLVerifiedTLSAndDriverIdentity(t *testing.T) {
	parsed, err := pgx.ParseConfig("postgres://fixture:synthetic@db.example.test:5432/fixture?sslmode=verify-full&connect_timeout=4")
	if err != nil {
		t.Fatal("parse verified PostgreSQL fixture")
	}
	if parsed.TLSConfig == nil || parsed.TLSConfig.InsecureSkipVerify || parsed.TLSConfig.ServerName != "db.example.test" || len(parsed.Fallbacks) != 0 {
		t.Fatal("verify-full can fall back to unverified/plaintext PostgreSQL")
	}
	if DriverName("postgres") != "pgx" || DriverName("mysql") != "mysql" || DriverName("sqlite3") != "sqlite3" {
		t.Fatal("database/sql driver mapping changed")
	}
}
