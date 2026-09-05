package models

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
)

func clearBootstrapEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{InitialAdminPassword, LegacyInitialAdminPassword, "DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", "GOPHISH_INITIAL_ADMIN_PASSWORD_FILE"} {
		t.Setenv(name, "")
	}
}

func TestBootstrapPasswordPaths(t *testing.T) {
	clearBootstrapEnvironment(t)
	dir := t.TempDir()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, backend, dsn, directory, want string
	}{
		{"sqlite filename", "sqlite3", "darkphish.db", "", filepath.Join(current, bootstrapPasswordFilename)},
		{"sqlite relative", "sqlite3", "data/darkphish.db", "", filepath.Join(current, "data", bootstrapPasswordFilename)},
		{"sqlite absolute", "sqlite3", filepath.Join(dir, "darkphish.db"), "", filepath.Join(dir, bootstrapPasswordFilename)},
		{"sqlite memory", "sqlite3", ":memory:", "", ""},
		{"sqlite URI", "sqlite3", "file:memory?mode=memory", "", filepath.Join(current, bootstrapPasswordFilename)},
		{"postgres URL", "postgres", "postgres://user:dsn-secret@db/app", "", filepath.Join(current, bootstrapPasswordFilename)},
		{"postgres keywords", "postgres", "host=db password=dsn-secret dbname=app", "", filepath.Join(current, bootstrapPasswordFilename)},
		{"postgres structured", "postgres", "", dir, filepath.Join(dir, bootstrapPasswordFilename)},
		{"mysql DSN", "mysql", "user:dsn-secret@tcp(db:3306)/app", "", filepath.Join(current, bootstrapPasswordFilename)},
		{"explicit directory", "mysql", "user:dsn-secret@tcp(db:3306)/app", dir, filepath.Join(dir, bootstrapPasswordFilename)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bootstrapPasswordPath(&config.Config{DBName: tc.backend, DBPath: tc.dsn, BootstrapDirectory: tc.directory})
			if err != nil || got != tc.want || strings.Contains(got, "dsn-secret") {
				t.Fatalf("bootstrap destination mismatch: got %q, err %v", got, err)
			}
		})
	}
}

func TestBootstrapPasswordExplicitSourcesAndProduction(t *testing.T) {
	clearBootstrapEnvironment(t)
	conf := &config.Config{DBName: "postgres", DBPath: "postgres://user:dsn-secret@db/app", ProductionMode: true}
	if _, _, err := prepareBootstrapPassword(conf); err == nil || strings.Contains(err.Error(), "dsn-secret") {
		t.Fatal("production must require explicit bootstrap configuration without leaking the DSN")
	}
	t.Setenv(InitialAdminPassword, "explicit-test-bootstrap-password")
	t.Setenv("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", "invalid://unused")
	value, destination, err := prepareBootstrapPassword(conf)
	if err != nil || value != "explicit-test-bootstrap-password" || destination != "" {
		t.Fatal("explicit password must win without filesystem operations")
	}
	t.Setenv(InitialAdminPassword, "")
	destination = filepath.Join(t.TempDir(), "bootstrap")
	t.Setenv("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", destination)
	value, got, err := prepareBootstrapPassword(conf)
	if err != nil || got != destination || len(value) < auth.MinPasswordLength {
		t.Fatalf("explicit password output file failed: %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != value+"\n" {
		t.Fatal("generated file does not contain the temporary password")
	}
	info, err := os.Stat(destination)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("generated password file permissions are not owner-only")
	}
	if _, _, err := prepareBootstrapPassword(conf); err == nil {
		t.Fatal("bootstrap must not overwrite an existing file")
	}
	previous := modelsConfigForBootstrapTest(conf)
	defer func() { modelsConfigForBootstrapTest(previous) }()
	RemoveInitialAdminPasswordFile()
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("first-password-change cleanup did not remove the generated file")
	}
}

func modelsConfigForBootstrapTest(value *config.Config) *config.Config {
	previous := conf
	conf = value
	return previous
}

func TestBootstrapErrorsAreRedacted(t *testing.T) {
	clearBootstrapEnvironment(t)
	if _, err := bootstrapPasswordPath(&config.Config{BootstrapDirectory: "bad\x00dsn-secret"}); err == nil || strings.Contains(err.Error(), "dsn-secret") {
		t.Fatal("malformed configured directory was not rejected safely")
	}
	for _, destination := range []string{"postgres://user:dsn-secret@db/app", filepath.Join(t.TempDir(), "missing-dsn-secret", "password"), t.TempDir()} {
		t.Setenv("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", destination)
		value, path, err := prepareBootstrapPassword(&config.Config{DBName: "mysql", DBPath: "user:dsn-secret@tcp(db)/app"})
		if err == nil || value != "" || path != "" || strings.Contains(err.Error(), "dsn-secret") || strings.Contains(err.Error(), destination) {
			t.Fatal("filesystem failure was not fail-closed and redacted")
		}
	}
}

func TestBootstrapLegacyOutputAndEnvironmentCompatibility(t *testing.T) {
	clearBootstrapEnvironment(t)
	destination := filepath.Join(t.TempDir(), "legacy-bootstrap")
	t.Setenv("GOPHISH_INITIAL_ADMIN_PASSWORD_FILE", destination)
	_, got, err := prepareBootstrapPassword(&config.Config{DBName: "mysql"})
	if err != nil || got != destination {
		t.Fatal("legacy output-file variable lost compatibility")
	}
	t.Setenv(LegacyInitialAdminPassword, "legacy-explicit-test-password")
	value, got, err := prepareBootstrapPassword(&config.Config{DBName: "postgres"})
	if err != nil || value != "legacy-explicit-test-password" || got != "" {
		t.Fatal("legacy explicit-password variable lost compatibility")
	}
}

func TestBootstrapUnwritableDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission test; Windows uses a protected DACL and exclusive create")
	}
	clearBootstrapEnvironment(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	t.Setenv("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", filepath.Join(dir, "password"))
	if _, _, err := prepareBootstrapPassword(&config.Config{}); err == nil {
		t.Fatal("unwritable destination accepted")
	}
}
