package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	log "github.com/darkarmy-cyber/darkphish/logger"
)

const (
	DefaultMaxRequestBodyBytes     int64 = 16 << 20
	DefaultSessionLifetimeHours          = 24
	DefaultAuditRetentionDays            = 365
	DefaultAuditCheckpointInterval       = 1000
	DefaultPATMaxLifetimeDays            = 90
	DefaultPrivilegedWindowMinutes       = 5
)

// AdminServer represents the Admin server configuration details
type AdminServer struct {
	ListenURL            string   `json:"listen_url"`
	UseTLS               bool     `json:"use_tls"`
	CertPath             string   `json:"cert_path"`
	KeyPath              string   `json:"key_path"`
	AllowedInternalHosts []string `json:"allowed_internal_hosts"`
	TrustedOrigins       []string `json:"trusted_origins"`
	CORSAllowedOrigins   []string `json:"cors_allowed_origins"`
	MaxRequestBodyBytes  int64    `json:"max_request_body_bytes"`
}

// PhishServer represents the Phish server configuration details
type PhishServer struct {
	ListenURL string `json:"listen_url"`
	UseTLS    bool   `json:"use_tls"`
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
}

// SessionConfig configures persistent authenticated browser sessions. Key
// values can be supplied directly, through the corresponding file setting, or
// through DARKPHISH_SESSION_* environment variables.
type SessionConfig struct {
	AuthKey           string `json:"auth_key"`
	AuthKeyFile       string `json:"auth_key_file"`
	EncryptionKey     string `json:"encryption_key"`
	EncryptionKeyFile string `json:"encryption_key_file"`
	LifetimeHours     int    `json:"lifetime_hours"`
}

// SecretsConfig configures encryption at rest for integration credentials.
type SecretsConfig struct {
	EncryptionKey     string            `json:"encryption_key"`
	EncryptionKeyFile string            `json:"encryption_key_file"`
	ActiveKeyID       string            `json:"active_key_id"`
	Keys              map[string]string `json:"keys"`
	Provider          string            `json:"provider"`
	Vault             VaultConfig       `json:"vault"`
}

type AuditConfig struct {
	InitializationTimeoutSeconds int               `json:"initialization_timeout_seconds"`
	AllowLegacyEphemeralRecovery bool              `json:"allow_legacy_ephemeral_recovery"`
	MultiInstance                bool              `json:"multi_instance"`
	RetentionDays                int               `json:"retention_days"`
	CheckpointInterval           int               `json:"checkpoint_interval"`
	ActiveSigningKeyID           string            `json:"active_signing_key_id"`
	SigningKeys                  map[string]string `json:"signing_keys"`
	SigningKeyFile               string            `json:"signing_key_file"`
}

// VaultConfig configures the production Vault Transit envelope-key provider.
// Token is resolved from a file or DARKPHISH_VAULT_TOKEN and is never logged.
type VaultConfig struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	TokenFile string `json:"token_file"`
	Namespace string `json:"namespace"`
	Mount     string `json:"mount"`
	KeyName   string `json:"key_name"`
	CACert    string `json:"ca_cert"`
}

type PrivilegedAccessConfig struct {
	WindowMinutes int `json:"window_minutes"`
}

// PostgreSQLConfig describes a PostgreSQL connection without requiring
// credentials to be embedded in db_path. Password is normally supplied via a
// file or DARKPHISH_POSTGRES_PASSWORD.
type PostgreSQLConfig struct {
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Database              string `json:"database"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	PasswordFile          string `json:"password_file"`
	SSLMode               string `json:"sslmode"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
}

type PATConfig struct {
	MaxLifetimeDays int `json:"max_lifetime_days"`
}

// Config represents the configuration information.
type Config struct {
	AdminConf                AdminServer            `json:"admin_server"`
	PhishConf                PhishServer            `json:"phish_server"`
	DBName                   string                 `json:"db_name"`
	DBPath                   string                 `json:"db_path"`
	BootstrapDirectory       string                 `json:"bootstrap_directory"`
	DBSSLCaPath              string                 `json:"db_sslca_path"`
	DBMaxOpenConns           int                    `json:"db_max_open_conns"`
	DBMaxIdleConns           int                    `json:"db_max_idle_conns"`
	DBConnMaxLifetimeMinutes int                    `json:"db_connection_max_lifetime_minutes"`
	MigrationsPath           string                 `json:"migrations_prefix"`
	TestFlag                 bool                   `json:"test_flag"`
	ProductionMode           bool                   `json:"production_mode"`
	Session                  SessionConfig          `json:"session"`
	Secrets                  SecretsConfig          `json:"secrets"`
	Audit                    AuditConfig            `json:"audit"`
	PAT                      PATConfig              `json:"personal_access_tokens"`
	PrivilegedAccess         PrivilegedAccessConfig `json:"privileged_access"`
	PostgreSQL               PostgreSQLConfig       `json:"postgresql"`
	ContactAddress           string                 `json:"contact_address"`
	Logging                  *log.Config            `json:"logging"`
}

// Version contains the current darkphish version
var Version = ""

// ServerName is the server type that is returned in the transparency response.
const ServerName = "darkphish"

// LoadConfig loads the configuration from the specified filepath
func LoadConfig(configPath string) (*Config, error) {
	// Get the config file
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(configFile, config)
	if err != nil {
		return nil, err
	}
	if config.Logging == nil {
		config.Logging = &log.Config{}
	}
	if config.AdminConf.MaxRequestBodyBytes == 0 {
		config.AdminConf.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}
	if config.Session.LifetimeHours == 0 {
		config.Session.LifetimeHours = DefaultSessionLifetimeHours
	}
	if config.Audit.RetentionDays == 0 {
		config.Audit.RetentionDays = DefaultAuditRetentionDays
	}
	if config.Audit.CheckpointInterval == 0 {
		config.Audit.CheckpointInterval = DefaultAuditCheckpointInterval
	}
	if config.PAT.MaxLifetimeDays == 0 {
		config.PAT.MaxLifetimeDays = DefaultPATMaxLifetimeDays
	}
	if config.PrivilegedAccess.WindowMinutes == 0 {
		config.PrivilegedAccess.WindowMinutes = DefaultPrivilegedWindowMinutes
	}
	if value := os.Getenv("DARKPHISH_PRODUCTION"); value != "" {
		production, parseErr := strconv.ParseBool(value)
		if parseErr != nil {
			return nil, fmt.Errorf("parse DARKPHISH_PRODUCTION: %w", parseErr)
		}
		config.ProductionMode = production
	}
	baseDir := filepath.Dir(configPath)
	if config.BootstrapDirectory != "" && !filepath.IsAbs(config.BootstrapDirectory) {
		config.BootstrapDirectory = filepath.Join(baseDir, config.BootstrapDirectory)
	}
	config.Session.AuthKey, err = resolveSecret(config.Session.AuthKey, config.Session.AuthKeyFile, baseDir, "DARKPHISH_SESSION_AUTH_KEY", "GOPHISH_SESSION_AUTH_KEY")
	if err != nil {
		return nil, err
	}
	config.Session.EncryptionKey, err = resolveSecret(config.Session.EncryptionKey, config.Session.EncryptionKeyFile, baseDir, "DARKPHISH_SESSION_ENCRYPTION_KEY", "GOPHISH_SESSION_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	config.Secrets.EncryptionKey, err = resolveSecret(config.Secrets.EncryptionKey, config.Secrets.EncryptionKeyFile, baseDir, "DARKPHISH_SECRET_ENCRYPTION_KEY", "GOPHISH_SECRET_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	if config.Secrets.Keys == nil {
		config.Secrets.Keys = make(map[string]string)
	}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, "DARKPHISH_SECRET_KEY_") || strings.TrimSpace(value) == "" {
			continue
		}
		config.Secrets.Keys[strings.TrimPrefix(name, "DARKPHISH_SECRET_KEY_")] = strings.TrimSpace(value)
	}
	if active := strings.TrimSpace(os.Getenv("DARKPHISH_SECRET_ACTIVE_KEY")); active != "" {
		config.Secrets.ActiveKeyID = active
	}
	config.Secrets.Vault.Token, err = resolveSecret(config.Secrets.Vault.Token, config.Secrets.Vault.TokenFile, baseDir, "DARKPHISH_VAULT_TOKEN", "")
	if err != nil {
		return nil, err
	}
	if config.Audit.SigningKeys == nil {
		config.Audit.SigningKeys = make(map[string]string)
	}
	if config.Audit.SigningKeyFile != "" {
		value, signingErr := resolveSecret("", config.Audit.SigningKeyFile, baseDir, "DARKPHISH_AUDIT_SIGNING_KEY", "")
		if signingErr != nil {
			return nil, signingErr
		}
		if config.Audit.ActiveSigningKeyID == "" {
			config.Audit.ActiveSigningKeyID = "default"
		}
		config.Audit.SigningKeys[config.Audit.ActiveSigningKeyID] = value
	}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, "DARKPHISH_AUDIT_SIGNING_KEY_") && strings.TrimSpace(value) != "" {
			config.Audit.SigningKeys[strings.TrimPrefix(name, "DARKPHISH_AUDIT_SIGNING_KEY_")] = strings.TrimSpace(value)
		}
	}
	if active := strings.TrimSpace(os.Getenv("DARKPHISH_AUDIT_ACTIVE_SIGNING_KEY")); active != "" {
		config.Audit.ActiveSigningKeyID = active
	}
	config.PostgreSQL.Password, err = resolveSecret(config.PostgreSQL.Password, config.PostgreSQL.PasswordFile, baseDir, "DARKPHISH_POSTGRES_PASSWORD", "")
	if err != nil {
		return nil, err
	}
	if config.Secrets.ActiveKeyID == "" && config.Secrets.EncryptionKey != "" {
		config.Secrets.ActiveKeyID = "legacy"
		config.Secrets.Keys["legacy"] = config.Secrets.EncryptionKey
	}
	if err = config.ValidateSecurity(); err != nil {
		return nil, err
	}
	if err = config.configureDatabase(); err != nil {
		return nil, err
	}
	// Choose the actual migration directory for the configured database. The
	// configured value remains a prefix for backwards-compatible config files.
	config.MigrationsPath = filepath.Join(config.MigrationsPath+config.DBName, "migrations")
	// Explicitly set the TestFlag to false to prevent config.json overrides
	config.TestFlag = false
	return config, nil
}

func (c *Config) configureDatabase() error {
	if c.DBName != "postgres" || strings.TrimSpace(c.DBPath) != "" {
		return nil
	}
	if strings.TrimSpace(c.PostgreSQL.Host) == "" || strings.TrimSpace(c.PostgreSQL.Database) == "" || strings.TrimSpace(c.PostgreSQL.Username) == "" {
		return errors.New("postgresql.host, postgresql.database, and postgresql.username are required when db_path is empty")
	}
	port := c.PostgreSQL.Port
	if port == 0 {
		port = 5432
	}
	sslMode := strings.TrimSpace(c.PostgreSQL.SSLMode)
	if sslMode == "" {
		sslMode = "require"
		if c.ProductionMode {
			sslMode = "verify-full"
		}
	}
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgreSQL.Username, c.PostgreSQL.Password),
		Host:   net.JoinHostPort(c.PostgreSQL.Host, strconv.Itoa(port)),
		Path:   "/" + c.PostgreSQL.Database,
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	if c.DBSSLCaPath != "" {
		query.Set("sslrootcert", c.DBSSLCaPath)
	}
	if c.PostgreSQL.ConnectTimeoutSeconds > 0 {
		query.Set("connect_timeout", strconv.Itoa(c.PostgreSQL.ConnectTimeoutSeconds))
	}
	dsn.RawQuery = query.Encode()
	c.DBPath = dsn.String()
	return nil
}

func firstEnvironmentValue(primary, legacy string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	value := os.Getenv(legacy)
	if value != "" {
		warnLegacyEnvironment(legacy, primary)
	}
	return value
}

var legacyWarnings sync.Map

func warnLegacyEnvironment(legacy, replacement string) {
	if legacy == "" {
		return
	}
	if _, loaded := legacyWarnings.LoadOrStore(legacy, struct{}{}); loaded {
		return
	}
	log.Warnf("Deprecated Gophish environment variable %s detected; use %s. Support will be removed in a future Darkphish release.", legacy, replacement)
}

func resolveSecret(configured, configuredFile, baseDir, primaryEnv, legacyEnv string) (string, error) {
	if value := firstEnvironmentValue(primaryEnv, legacyEnv); value != "" {
		return strings.TrimSpace(value), nil
	}
	if configured != "" {
		return strings.TrimSpace(configured), nil
	}
	if configuredFile == "" {
		return "", nil
	}
	secretPath := configuredFile
	if !filepath.IsAbs(secretPath) {
		secretPath = filepath.Join(baseDir, secretPath)
	}
	contents, err := os.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("read secret file %q: %w", secretPath, err)
	}
	return strings.TrimSpace(string(contents)), nil
}

// ValidateSecurity rejects incomplete production security configuration.
func (c *Config) ValidateSecurity() error {
	if c.Audit.RetentionDays == 0 {
		c.Audit.RetentionDays = DefaultAuditRetentionDays
	}
	if c.Audit.CheckpointInterval == 0 {
		c.Audit.CheckpointInterval = DefaultAuditCheckpointInterval
	}
	if c.PAT.MaxLifetimeDays == 0 {
		c.PAT.MaxLifetimeDays = DefaultPATMaxLifetimeDays
	}
	if c.PrivilegedAccess.WindowMinutes == 0 {
		c.PrivilegedAccess.WindowMinutes = DefaultPrivilegedWindowMinutes
	}
	if c.Secrets.EncryptionKey != "" {
		if c.Secrets.Keys == nil {
			c.Secrets.Keys = make(map[string]string)
		}
		c.Secrets.Keys["legacy"] = c.Secrets.EncryptionKey
		if c.Secrets.ActiveKeyID == "" {
			c.Secrets.ActiveKeyID = "legacy"
		}
	}
	if c.AdminConf.MaxRequestBodyBytes < 1 {
		return errors.New("admin_server.max_request_body_bytes must be positive")
	}
	if c.Session.LifetimeHours < 1 || c.Session.LifetimeHours > 24*30 {
		return errors.New("session.lifetime_hours must be between 1 and 720")
	}
	if c.Audit.RetentionDays < 1 {
		return errors.New("audit.retention_days must be positive")
	}
	if c.Audit.CheckpointInterval < 1 || c.Audit.CheckpointInterval > 1000000 {
		return errors.New("audit.checkpoint_interval must be between 1 and 1000000")
	}
	if c.PrivilegedAccess.WindowMinutes < 1 || c.PrivilegedAccess.WindowMinutes > 15 {
		return errors.New("privileged_access.window_minutes must be between 1 and 15")
	}
	if c.DBMaxOpenConns < 0 || c.DBMaxIdleConns < 0 || c.DBConnMaxLifetimeMinutes < 0 || c.DBConnMaxLifetimeMinutes > 24*60 {
		return errors.New("database connection pool settings are outside supported bounds")
	}
	if c.PostgreSQL.Port < 0 || c.PostgreSQL.Port > 65535 || c.PostgreSQL.ConnectTimeoutSeconds < 0 || c.PostgreSQL.ConnectTimeoutSeconds > 300 {
		return errors.New("PostgreSQL port or connect timeout is outside supported bounds")
	}
	if c.DBName == "postgres" {
		sslMode := strings.TrimSpace(c.PostgreSQL.SSLMode)
		if sslMode != "" {
			supported := map[string]bool{"disable": true, "allow": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true}
			if !supported[sslMode] {
				return fmt.Errorf("postgresql.sslmode %q is unsupported", sslMode)
			}
			if c.ProductionMode && sslMode != "verify-full" {
				return errors.New("postgresql.sslmode must be verify-full in production")
			}
		}
		if c.ProductionMode && strings.TrimSpace(c.DBPath) != "" {
			return errors.New("production PostgreSQL must use structured postgresql configuration so verified TLS and secret resolution can be enforced")
		}
	}
	if c.PAT.MaxLifetimeDays < 1 || c.PAT.MaxLifetimeDays > 3650 {
		return errors.New("personal_access_tokens.max_lifetime_days must be between 1 and 3650")
	}
	for _, origin := range c.AdminConf.TrustedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("admin_server.trusted_origins entry %q must be an exact HTTP(S) origin without credentials, path, query, or fragment", origin)
		}
		if c.ProductionMode && parsed.Scheme != "https" {
			return fmt.Errorf("admin_server.trusted_origins entry %q must use HTTPS in production", origin)
		}
	}
	for id, value := range c.Secrets.Keys {
		decoded, err := secretpkg.DecodeKey(value)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("DARKPHISH_SECRET_KEY_%s must contain exactly 32 bytes", id)
		}
	}
	if c.Secrets.ActiveKeyID != "" {
		if _, ok := c.Secrets.Keys[c.Secrets.ActiveKeyID]; !ok {
			return fmt.Errorf("secrets.active_key_id %q has no configured key", c.Secrets.ActiveKeyID)
		}
	}
	if c.Secrets.Provider == "" {
		c.Secrets.Provider = "local"
	}
	if c.Secrets.Provider != "local" && c.Secrets.Provider != "vault" {
		return fmt.Errorf("secrets.provider %q is unsupported", c.Secrets.Provider)
	}
	if c.Secrets.Provider == "vault" {
		if strings.TrimSpace(c.Secrets.Vault.Address) == "" || strings.TrimSpace(c.Secrets.Vault.KeyName) == "" || strings.TrimSpace(c.Secrets.Vault.Token) == "" {
			return errors.New("Vault secret provider requires address, key_name, and token")
		}
		vaultURL, err := url.Parse(c.Secrets.Vault.Address)
		if err != nil || vaultURL.Host == "" || vaultURL.User != nil || (vaultURL.Scheme != "https" && !(vaultURL.Scheme == "http" && !c.ProductionMode)) {
			return errors.New("Vault address must be HTTPS (HTTP is development-only)")
		}
	}
	if c.Audit.ActiveSigningKeyID != "" {
		if _, ok := c.Audit.SigningKeys[c.Audit.ActiveSigningKeyID]; !ok {
			return fmt.Errorf("audit.active_signing_key_id %q has no configured signing key", c.Audit.ActiveSigningKeyID)
		}
		if _, err := audit.NewSigningKeyring(c.Audit.ActiveSigningKeyID, c.Audit.SigningKeys); err != nil {
			return err
		}
	}
	if !c.ProductionMode {
		return nil
	}
	missing := make([]string, 0, 3)
	if c.Session.AuthKey == "" {
		missing = append(missing, "DARKPHISH_SESSION_AUTH_KEY")
	}
	if c.Session.EncryptionKey == "" {
		missing = append(missing, "DARKPHISH_SESSION_ENCRYPTION_KEY")
	}
	if c.Secrets.Provider != "vault" && c.Secrets.ActiveKeyID == "" {
		missing = append(missing, "DARKPHISH_SECRET_ACTIVE_KEY and DARKPHISH_SECRET_KEY_<ID>")
	}
	if c.Audit.ActiveSigningKeyID == "" {
		missing = append(missing, "DARKPHISH_AUDIT_ACTIVE_SIGNING_KEY and DARKPHISH_AUDIT_SIGNING_KEY_<ID>")
	}
	if len(missing) > 0 {
		return fmt.Errorf("production security configuration is incomplete; set %s (or configured key files)", strings.Join(missing, ", "))
	}
	for name, value := range map[string]string{
		"DARKPHISH_SESSION_AUTH_KEY":       c.Session.AuthKey,
		"DARKPHISH_SESSION_ENCRYPTION_KEY": c.Session.EncryptionKey,
	} {
		decoded, err := secretpkg.DecodeKey(value)
		if err != nil {
			return fmt.Errorf("%s is invalid: %w", name, err)
		}
		if len(decoded) != 32 {
			return fmt.Errorf("%s must contain exactly 32 bytes", name)
		}
	}
	return nil
}
