package models

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // Register the PostgreSQL dialect.
	_ "github.com/mattn/go-sqlite3"              // Blank import needed to import sqlite3
)

var db *gorm.DB
var conf *config.Config
var secretStore secretpkg.Store = secretpkg.PlaintextStore{}

// Health verifies that the configured database is reachable.
func Health() error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return db.DB().Ping()
}

const MaxDatabaseConnectionAttempts int = 10

// DefaultAdminUsername is the default username for the administrative user
const DefaultAdminUsername = "admin"

// InitialAdminPassword is the environment variable that specifies which
// password to use for the initial root login instead of generating one
// randomly
const InitialAdminPassword = "DARKPHISH_INITIAL_ADMIN_PASSWORD"
const LegacyInitialAdminPassword = "GOPHISH_INITIAL_ADMIN_PASSWORD"

var legacyEnvironmentWarnings sync.Map

func environmentValue(primary, legacy string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	value := os.Getenv(legacy)
	if value != "" {
		if _, loaded := legacyEnvironmentWarnings.LoadOrStore(legacy, struct{}{}); !loaded {
			log.Warnf("Deprecated Gophish environment variable %s detected; use %s. Support will be removed in a future Darkphish release.", legacy, primary)
		}
	}
	return value
}

func configureSecretStore(c *config.Config) error {
	providerName := c.Secrets.Provider
	if providerName == "" {
		providerName = "local"
	}
	var local secretpkg.VersionedStore
	if c.Secrets.ActiveKeyID != "" && len(c.Secrets.Keys) > 0 {
		keys := make(map[string][]byte, len(c.Secrets.Keys))
		for id, value := range c.Secrets.Keys {
			key, err := secretpkg.DecodeKey(value)
			if err != nil {
				return fmt.Errorf("decode secret key %s: %w", id, err)
			}
			keys[id] = key
		}
		var legacyKey []byte
		if c.Secrets.EncryptionKey != "" {
			var err error
			legacyKey, err = secretpkg.DecodeKey(c.Secrets.EncryptionKey)
			if err != nil {
				return fmt.Errorf("decode legacy secret key: %w", err)
			}
		}
		var err error
		local, err = secretpkg.NewKeyring(c.Secrets.ActiveKeyID, keys, legacyKey)
		if err != nil {
			return err
		}
	}
	providers := []secretpkg.KeyProvider{}
	if c.Secrets.Vault.Address != "" || providerName == "vault" {
		vault, err := secretpkg.NewVaultTransitProvider(secretpkg.VaultTransitOptions{
			Address: c.Secrets.Vault.Address, Token: c.Secrets.Vault.Token, Namespace: c.Secrets.Vault.Namespace,
			Mount: c.Secrets.Vault.Mount, KeyName: c.Secrets.Vault.KeyName, CACert: c.Secrets.Vault.CACert,
		})
		if err != nil {
			return fmt.Errorf("configure Vault Transit provider: %w", err)
		}
		providers = append(providers, vault)
	}
	if providerName == "local" && local == nil {
		if c.ProductionMode {
			return fmt.Errorf("a versioned secret keyring is required in production mode")
		}
		secretStore = secretpkg.PlaintextStore{}
		log.Warn("integration secrets are stored as plaintext in development mode; configure DARKPHISH_SECRET_ENCRYPTION_KEY")
		return nil
	}
	routing, err := secretpkg.NewRoutingStore(providerName, local, providers...)
	if err != nil {
		return err
	}
	secretStore = routing
	return nil
}

const (
	CampaignInProgress string = "In progress"
	CampaignQueued     string = "Queued"
	CampaignCreated    string = "Created"
	CampaignEmailsSent string = "Emails Sent"
	CampaignComplete   string = "Completed"
	EventSent          string = "Email Sent"
	EventSendingError  string = "Error Sending Email"
	EventOpened        string = "Email Opened"
	EventClicked       string = "Clicked Link"
	EventDataSubmit    string = "Submitted Data"
	EventReported      string = "Email Reported"
	EventProxyRequest  string = "Proxied request"
	StatusSuccess      string = "Success"
	StatusQueued       string = "Queued"
	StatusSending      string = "Sending"
	StatusUnknown      string = "Unknown"
	StatusScheduled    string = "Scheduled"
	StatusRetry        string = "Retrying"
	Error              string = "Error"
)

// Flash is used to hold flash information for use in templates.
type Flash struct {
	Type    string
	Message string
}

// Response contains the attributes found in an API response
type Response struct {
	Message string      `json:"message"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

func createTemporaryPassword(u *User) error {
	var temporaryPassword string
	if envPassword := environmentValue(InitialAdminPassword, LegacyInitialAdminPassword); envPassword != "" {
		temporaryPassword = envPassword
	} else {
		temporaryPassword = auth.GenerateSecureKey(auth.MinPasswordLength)
		if conf.DBPath != ":memory:" {
			passwordPath := environmentValue("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", "GOPHISH_INITIAL_ADMIN_PASSWORD_FILE")
			if passwordPath == "" {
				passwordPath = filepath.Join(filepath.Dir(conf.DBPath), "darkphish_initial_admin_password")
			}
			if err := os.WriteFile(passwordPath, []byte(temporaryPassword+"\n"), 0600); err != nil {
				return fmt.Errorf("write initial administrator password file: %w", err)
			}
			log.Infof("Initial administrator password written to %s; delete this file after first login", passwordPath)
		}
	}
	hash, err := auth.GeneratePasswordHash(temporaryPassword)
	if err != nil {
		return err
	}
	u.Hash = hash
	// Anytime a temporary password is created, we will force the user
	// to change their password
	u.PasswordChangeRequired = true
	err = db.Save(u).Error
	if err != nil {
		return err
	}
	return nil
}

// RemoveInitialAdminPasswordFile removes the generated development bootstrap
// credential after the administrator completes the required password change.
func RemoveInitialAdminPasswordFile() {
	if conf == nil || conf.DBPath == ":memory:" || environmentValue(InitialAdminPassword, LegacyInitialAdminPassword) != "" {
		return
	}
	passwordPath := environmentValue("DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE", "GOPHISH_INITIAL_ADMIN_PASSWORD_FILE")
	if passwordPath == "" {
		passwordPath = filepath.Join(filepath.Dir(conf.DBPath), "darkphish_initial_admin_password")
	}
	if err := os.Remove(passwordPath); err != nil && !os.IsNotExist(err) {
		log.Warnf("unable to remove initial administrator password file %s: %v", passwordPath, err)
	}
}

// Setup initializes the database and runs any needed migrations.
//
// First, it establishes a connection to the database, then runs any migrations
// newer than the version the database is on.
//
// Once the database is up-to-date, we create an admin user when needed with a
// disabled legacy-key placeholder and a generated initial password.
func Setup(c *config.Config) error {
	// Setup the package-scoped config
	conf = c
	var err error
	if err := configureSecretStore(c); err != nil {
		return err
	}
	if err := goose.SetDialect(conf.DBName); err != nil {
		log.Error(err)
		return fmt.Errorf("configure database migrations: %w", err)
	}

	// Register certificates for tls encrypted db connections
	if conf.DBSSLCaPath != "" {
		switch conf.DBName {
		case "mysql":
			rootCertPool := x509.NewCertPool()
			pem, err := os.ReadFile(conf.DBSSLCaPath)
			if err != nil {
				log.Error(err)
				return err
			}
			if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
				log.Error("Failed to append PEM.")
				return err
			}
			mysql.RegisterTLSConfig("ssl_ca", &tls.Config{
				RootCAs: rootCertPool,
			})
			// Default database is sqlite3, which supports no tls, as connection
			// is file based
		default:
		}
	}

	// Open our database connection
	i := 0
	for {
		db, err = gorm.Open(conf.DBName, conf.DBPath)
		if err == nil {
			break
		}
		if err != nil && i >= MaxDatabaseConnectionAttempts {
			log.Error(err)
			return err
		}
		i += 1
		log.Warn("waiting for database to be up...")
		time.Sleep(5 * time.Second)
	}
	db.LogMode(false)
	db.SetLogger(log.Logger)
	maxOpen := conf.DBMaxOpenConns
	if maxOpen == 0 {
		if conf.DBName == "sqlite3" {
			maxOpen = 1
		} else {
			maxOpen = 10
		}
	}
	maxIdle := conf.DBMaxIdleConns
	if maxIdle == 0 {
		maxIdle = maxOpen
	}
	db.DB().SetMaxOpenConns(maxOpen)
	db.DB().SetMaxIdleConns(maxIdle)
	if conf.DBConnMaxLifetimeMinutes > 0 {
		db.DB().SetConnMaxLifetime(time.Duration(conf.DBConnMaxLifetimeMinutes) * time.Minute)
	}
	if err != nil {
		log.Error(err)
		return err
	}
	// Migrate up to the latest version
	err = goose.Up(db.DB(), conf.MigrationsPath)
	if err != nil {
		log.Error(err)
		return err
	}
	if err := configureAuditStore(); err != nil {
		return fmt.Errorf("configure audit integrity: %w", err)
	}
	if err := FlushAuditOutbox(); err != nil {
		log.Errorf("flush audit outbox: %v", err)
	}
	// Create the admin user if it doesn't exist
	var userCount int64
	var adminUser User
	db.Model(&User{}).Count(&userCount)
	adminRole, err := GetRoleBySlug(RoleAdmin)
	if err != nil {
		log.Error(err)
		return err
	}
	if userCount == 0 {
		adminUser := User{
			Username:               DefaultAdminUsername,
			Role:                   adminRole,
			RoleID:                 adminRole.ID,
			PasswordChangeRequired: true,
		}

		adminUser.ApiKey = "disabled-" + auth.GenerateSecureKey(16)

		err = db.Save(&adminUser).Error
		if err != nil {
			log.Error(err)
			return err
		}
	}
	// Initialize a password only when the new or migrated account has no hash.
	// Existing password-reset-required accounts retain their temporary password
	// across restarts.
	if adminUser.Username == "" {
		adminUser, err = GetUserByUsername(DefaultAdminUsername)
		if err != nil {
			log.Error(err)
			return err
		}
	}
	if strings.TrimSpace(adminUser.Hash) == "" {
		err = createTemporaryPassword(&adminUser)
		if err != nil {
			log.Error(err)
			return err
		}
	}
	return nil
}
