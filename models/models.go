package models

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/auth"
	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/persistence"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	"github.com/pressly/goose/v3"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"gorm.io/gorm"
)

var db *gorm.DB
var conf *config.Config
var secretStore secretpkg.Store = secretpkg.PlaintextStore{}

// Close releases database resources after all application workers have stopped.
func Close() error {
	if db == nil {
		return nil
	}
	connection, err := db.DB()
	if err != nil {
		return err
	}
	return connection.Close()
}

// Health verifies that the configured database is reachable.
func Health() error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	connection, err := db.DB()
	if err != nil {
		return err
	}
	return connection.Ping()
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
	temporaryPassword, passwordPath, err := prepareBootstrapPassword(conf)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed && passwordPath != "" {
			_ = os.Remove(passwordPath)
		}
	}()
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
	committed = true
	if passwordPath != "" {
		log.Info("Initial administrator password written to the bootstrap password file; it is removed after the required first password change")
	}
	return nil
}

// RemoveInitialAdminPasswordFile removes the generated development bootstrap
// credential after the administrator completes the required password change.
func RemoveInitialAdminPasswordFile() {
	if conf == nil || environmentValue(InitialAdminPassword, LegacyInitialAdminPassword) != "" {
		return
	}
	passwordPath, err := bootstrapPasswordPath(conf)
	if err != nil || passwordPath == "" {
		return
	}
	if err := os.Remove(passwordPath); err != nil && !os.IsNotExist(err) {
		log.Warn("Unable to remove the bootstrap password file; remove it manually after the first password change")
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

	// Open our database connection
	i := 0
	for {
		db, err = persistence.Open(conf)
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
	connection, err := db.DB()
	if err != nil {
		return err
	}
	// Migrate up to the latest version
	err = goose.Up(connection, conf.MigrationsPath)
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
	if err := db.Model(&User{}).Count(&userCount).Error; err != nil {
		return err
	}
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
