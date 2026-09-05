package models

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
)

const bootstrapPasswordFilename = "darkphish_initial_admin_password"

// bootstrapPasswordPath never interprets a network database connection string
// as a path. The *_PASSWORD_FILE variables retain their existing output-file
// semantics. Directories must already exist and be controlled by the operator.
func bootstrapPasswordPath(c *config.Config) (string, error) {
	if c == nil {
		return "", errors.New("bootstrap configuration is unavailable")
	}
	destination := environmentValue("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", "GOPHISH_INITIAL_ADMIN_PASSWORD_FILE")
	if destination == "" && c.BootstrapDirectory != "" {
		if !isFilesystemPath(c.BootstrapDirectory) {
			return "", errors.New("bootstrap_directory must be a filesystem directory")
		}
		destination = filepath.Join(c.BootstrapDirectory, bootstrapPasswordFilename)
	}
	if destination == "" {
		if c.ProductionMode {
			return "", errors.New("initial administrator setup requires DARKPHISH_INITIAL_ADMIN_PASSWORD, DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE, or bootstrap_directory in production")
		}
		if c.DBName == "sqlite3" && c.DBPath == ":memory:" {
			return "", nil // Ephemeral development/test database; no persistent credential.
		}
		if c.DBName == "sqlite3" && isFilesystemPath(c.DBPath) {
			destination = filepath.Join(filepath.Dir(c.DBPath), bootstrapPasswordFilename)
		} else {
			destination = bootstrapPasswordFilename // Documented working-directory development fallback.
		}
	}
	if !isFilesystemPath(destination) || strings.HasSuffix(destination, "/") || strings.HasSuffix(destination, "\\") || filepath.Base(destination) == "." {
		return "", errors.New("invalid bootstrap password file destination")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", errors.New("cannot resolve bootstrap password file destination")
	}
	return absolute, nil
}

func isFilesystemPath(value string) bool {
	// A colon is valid only as a Windows drive prefix, even on Unix. Reject
	// SQLite URI/keyword forms too: only genuine filenames get this fallback.
	if value == "" || strings.ContainsAny(value, "\x00\r\n@?=()") || strings.Contains(value, "://") {
		return false
	}
	if index := strings.IndexByte(value, ':'); index >= 0 {
		return index == 1 && len(value) > 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && (value[2] == '/' || value[2] == '\\') && !strings.Contains(value[2:], ":")
	}
	return true
}

func prepareBootstrapPassword(c *config.Config) (string, string, error) {
	if password := environmentValue(InitialAdminPassword, LegacyInitialAdminPassword); password != "" {
		return password, "", nil
	}
	destination, err := bootstrapPasswordPath(c)
	if err != nil {
		return "", "", err
	}
	password := auth.GenerateSecureKey(auth.MinPasswordLength)
	if destination == "" {
		return password, "", nil
	}
	file, err := createPrivateBootstrapFile(destination)
	if err != nil {
		// Do not wrap PathError: even explicitly misconfigured paths can contain
		// credentials. O_EXCL also refuses symlinks and existing operator files.
		return "", "", errors.New("cannot create bootstrap password file; use an unused filename in an existing writable directory")
	}
	_, writeErr := file.WriteString(password + "\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		return "", "", errors.New("cannot persist bootstrap password file; check destination permissions and free space")
	}
	return password, destination, nil
}
