package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

var validConfig = []byte(`{
	"admin_server": {
		"listen_url": "127.0.0.1:3333",
		"use_tls": true,
		"cert_path": "darkphish_admin.crt",
		"key_path": "darkphish_admin.key"
	},
	"phish_server": {
		"listen_url": "0.0.0.0:8080",
		"use_tls": false,
		"cert_path": "example.crt",
		"key_path": "example.key"
	},
	"db_name": "sqlite3",
	"db_path": "darkphish.db",
	"migrations_prefix": "db/db_",
	"contact_address": ""
}`)

func createTemporaryConfig(t *testing.T) *os.File {
	f, err := os.CreateTemp("", "darkphish-config")
	if err != nil {
		t.Fatalf("unable to create temporary config: %v", err)
	}
	return f
}

func TestPostgreSQLStructuredConfiguration(t *testing.T) {
	t.Setenv("DARKPHISH_POSTGRES_PASSWORD", "sensitive password")
	f := createTemporaryConfig(t)
	defer removeTemporaryConfig(t, f)
	contents := []byte(`{
		"admin_server":{"max_request_body_bytes":1024},
		"db_name":"postgres",
		"db_path":"",
		"migrations_prefix":"db/db_",
		"postgresql":{"host":"db.example.test","database":"darkphish","username":"service user","sslmode":"verify-full","connect_timeout_seconds":10}
	}`)
	if _, err := f.Write(contents); err != nil {
		t.Fatal(err)
	}
	conf, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if conf.MigrationsPath != filepath.Join("db", "db_postgres", "migrations") {
		t.Fatalf("unexpected migration path %q", conf.MigrationsPath)
	}
	if !strings.HasPrefix(conf.DBPath, "postgres://service%20user:sensitive%20password@db.example.test:5432/darkphish?") || !strings.Contains(conf.DBPath, "sslmode=verify-full") {
		t.Fatalf("unexpected PostgreSQL DSN %q", conf.DBPath)
	}
}

func TestPostgreSQLProductionRequiresVerifiedTLS(t *testing.T) {
	conf := &Config{
		AdminConf:      AdminServer{MaxRequestBodyBytes: DefaultMaxRequestBodyBytes},
		Session:        SessionConfig{LifetimeHours: DefaultSessionLifetimeHours},
		ProductionMode: true,
		DBName:         "postgres",
		PostgreSQL:     PostgreSQLConfig{SSLMode: "require"},
	}
	if err := conf.ValidateSecurity(); err == nil || !strings.Contains(err.Error(), "verify-full") {
		t.Fatalf("expected verified PostgreSQL TLS rejection, got %v", err)
	}
}

func removeTemporaryConfig(t *testing.T, f *os.File) {
	err := f.Close()
	if err != nil {
		t.Fatalf("unable to remove temporary config: %v", err)
	}
}

func TestLoadConfig(t *testing.T) {
	f := createTemporaryConfig(t)
	defer removeTemporaryConfig(t, f)
	_, err := f.Write(validConfig)
	if err != nil {
		t.Fatalf("error writing config to temporary file: %v", err)
	}
	// Load the valid config
	conf, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("error loading config from temporary file: %v", err)
	}

	expectedConfig := &Config{}
	err = json.Unmarshal(validConfig, &expectedConfig)
	if err != nil {
		t.Fatalf("error unmarshaling config: %v", err)
	}
	expectedConfig.MigrationsPath = filepath.Join(expectedConfig.MigrationsPath+expectedConfig.DBName, "migrations")
	expectedConfig.TestFlag = false
	expectedConfig.AdminConf.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	expectedConfig.Session.LifetimeHours = DefaultSessionLifetimeHours
	expectedConfig.Secrets.Keys = map[string]string{}
	expectedConfig.Secrets.Provider = "local"
	expectedConfig.Audit.RetentionDays = DefaultAuditRetentionDays
	expectedConfig.Audit.CheckpointInterval = DefaultAuditCheckpointInterval
	expectedConfig.Audit.SigningKeys = map[string]string{}
	expectedConfig.PAT.MaxLifetimeDays = DefaultPATMaxLifetimeDays
	expectedConfig.PrivilegedAccess.WindowMinutes = DefaultPrivilegedWindowMinutes
	expectedConfig.Logging = &log.Config{}
	if !reflect.DeepEqual(expectedConfig, conf) {
		t.Fatalf("invalid config received. expected %#v got %#v", expectedConfig, conf)
	}

	// Load an invalid config
	_, err = LoadConfig("bogusfile")
	if err == nil {
		t.Fatalf("expected error when loading invalid config, but got %v", err)
	}
}

func TestProductionSecurityValidation(t *testing.T) {
	conf := &Config{
		AdminConf:      AdminServer{MaxRequestBodyBytes: DefaultMaxRequestBodyBytes},
		Session:        SessionConfig{LifetimeHours: DefaultSessionLifetimeHours},
		ProductionMode: true,
	}
	if err := conf.ValidateSecurity(); err == nil {
		t.Fatal("expected missing production secrets to be rejected")
	}

	conf.Session.AuthKey = "short"
	conf.Session.EncryptionKey = "short"
	conf.Secrets.EncryptionKey = "short"
	if err := conf.ValidateSecurity(); err == nil {
		t.Fatal("expected weak production secrets to be rejected")
	}

	conf.Session.AuthKey = "hex:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	conf.Session.EncryptionKey = "0123456789abcdef0123456789abcdef"
	conf.Secrets.EncryptionKey = "base64:MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	conf.Audit.ActiveSigningKeyID = "test"
	conf.Audit.SigningKeys = map[string]string{"test": "0123456789abcdef0123456789abcdef"}
	if err := conf.ValidateSecurity(); err != nil {
		t.Fatalf("expected valid production secrets: %v", err)
	}

	conf.AdminConf.TrustedOrigins = []string{"http://admin.example.test"}
	if err := conf.ValidateSecurity(); err == nil {
		t.Fatal("expected a plaintext production trusted origin to be rejected")
	}
	conf.AdminConf.TrustedOrigins = []string{"https://admin.example.test"}
	if err := conf.ValidateSecurity(); err != nil {
		t.Fatalf("expected an exact HTTPS production trusted origin: %v", err)
	}
}

func TestTrustedOriginValidation(t *testing.T) {
	conf := &Config{
		AdminConf: AdminServer{MaxRequestBodyBytes: DefaultMaxRequestBodyBytes},
		Session:   SessionConfig{LifetimeHours: DefaultSessionLifetimeHours},
	}
	for _, origin := range []string{
		"admin.example.test",
		"ftp://admin.example.test",
		"https://user@admin.example.test",
		"https://admin.example.test/",
		"https://admin.example.test/path",
		"https://admin.example.test?query=true",
	} {
		conf.AdminConf.TrustedOrigins = []string{origin}
		if err := conf.ValidateSecurity(); err == nil {
			t.Errorf("expected invalid trusted origin %q to be rejected", origin)
		}
	}

	conf.AdminConf.TrustedOrigins = []string{"http://localhost:3333", "https://admin.example.test"}
	if err := conf.ValidateSecurity(); err != nil {
		t.Fatalf("expected exact development origins to be accepted: %v", err)
	}
}
