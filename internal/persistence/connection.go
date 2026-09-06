// Package persistence owns the ORM dialect and database/sql connection policy.
// It never creates or migrates application schema; production schema is Goose's
// responsibility. No driver or ORM logger receives credentials or query values.
package persistence

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrConnection = errors.New("database connection unavailable")
	ErrBackend    = errors.New("unsupported database backend")
	ErrTrust      = errors.New("database certificate configuration is invalid")
)

// Open applies the existing pool defaults and opens a verified connection.
// Driver errors can contain a DSN/password, so connection failures deliberately
// return stable, secret-free errors. Query errors remain distinct at call sites.
func Open(c *config.Config) (*gorm.DB, error) {
	if c == nil {
		return nil, ErrBackend
	}
	var dialect gorm.Dialector
	switch c.DBName {
	case "sqlite3":
		dialect = sqlite.Open(c.DBPath)
	case "mysql":
		if c.DBSSLCaPath != "" {
			roots, err := certificateRoots(c.DBSSLCaPath)
			if err != nil {
				return nil, err
			}
			if err := mysqldriver.RegisterTLSConfig("ssl_ca", &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}); err != nil {
				return nil, ErrTrust
			}
		}
		dialect = mysql.Open(c.DBPath)
	case "postgres":
		// Structured PostgreSQL settings encode this file as sslrootcert.
		// Reject invalid configured trust before pgx opens a connection so a
		// permanent CA error cannot enter the transient connection retry loop.
		if c.DBSSLCaPath != "" {
			if _, err := certificateRoots(c.DBSSLCaPath); err != nil {
				return nil, err
			}
		}
		// pgx retains its supported parameterized extended-query protocol. TLS,
		// verify-full and CA settings arrive from the validated configuration.
		dialect = postgres.Open(c.DBPath)
	default:
		return nil, ErrBackend
	}
	database, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		if database != nil {
			if connection, e := database.DB(); e == nil {
				_ = connection.Close()
			}
		}
		return nil, ErrConnection
	}
	connection, err := database.DB()
	if err != nil {
		return nil, ErrConnection
	}
	maxOpen := c.DBMaxOpenConns
	if maxOpen == 0 {
		if c.DBName == "sqlite3" {
			maxOpen = 1
		} else {
			maxOpen = 10
		}
	}
	maxIdle := c.DBMaxIdleConns
	if maxIdle == 0 {
		maxIdle = maxOpen
	}
	connection.SetMaxOpenConns(maxOpen)
	connection.SetMaxIdleConns(maxIdle)
	if c.DBConnMaxLifetimeMinutes > 0 {
		connection.SetConnMaxLifetime(time.Duration(c.DBConnMaxLifetimeMinutes) * time.Minute)
	}
	return database, nil
}

func certificateRoots(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrTrust
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, ErrTrust
	}
	return roots, nil
}

// DriverName keeps public backend names stable while PostgreSQL uses pgx.
func DriverName(backend string) string {
	if backend == "postgres" {
		return "pgx"
	}
	return backend
}
