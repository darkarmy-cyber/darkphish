package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	secretpkg "github.com/darkarmy-cyber/darkphish/internal/secrets"
	log "github.com/darkarmy-cyber/darkphish/logger"
)

const (
	DefaultMaxRequestBodyBytes  int64 = 16 << 20
	DefaultSessionLifetimeHours       = 24
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
	EncryptionKey     string `json:"encryption_key"`
	EncryptionKeyFile string `json:"encryption_key_file"`
}

// Config represents the configuration information.
type Config struct {
	AdminConf      AdminServer   `json:"admin_server"`
	PhishConf      PhishServer   `json:"phish_server"`
	DBName         string        `json:"db_name"`
	DBPath         string        `json:"db_path"`
	DBSSLCaPath    string        `json:"db_sslca_path"`
	DBMaxOpenConns int           `json:"db_max_open_conns"`
	DBMaxIdleConns int           `json:"db_max_idle_conns"`
	MigrationsPath string        `json:"migrations_prefix"`
	TestFlag       bool          `json:"test_flag"`
	ProductionMode bool          `json:"production_mode"`
	Session        SessionConfig `json:"session"`
	Secrets        SecretsConfig `json:"secrets"`
	ContactAddress string        `json:"contact_address"`
	Logging        *log.Config   `json:"logging"`
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
	if value := os.Getenv("DARKPHISH_PRODUCTION"); value != "" {
		production, parseErr := strconv.ParseBool(value)
		if parseErr != nil {
			return nil, fmt.Errorf("parse DARKPHISH_PRODUCTION: %w", parseErr)
		}
		config.ProductionMode = production
	}
	baseDir := filepath.Dir(configPath)
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
	if err = config.ValidateSecurity(); err != nil {
		return nil, err
	}
	// Choosing the migrations directory based on the database used.
	config.MigrationsPath = config.MigrationsPath + config.DBName
	// Explicitly set the TestFlag to false to prevent config.json overrides
	config.TestFlag = false
	return config, nil
}

func firstEnvironmentValue(primary, legacy string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	return os.Getenv(legacy)
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
	if c.AdminConf.MaxRequestBodyBytes < 1 {
		return errors.New("admin_server.max_request_body_bytes must be positive")
	}
	if c.Session.LifetimeHours < 1 || c.Session.LifetimeHours > 24*30 {
		return errors.New("session.lifetime_hours must be between 1 and 720")
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
	if c.Secrets.EncryptionKey == "" {
		missing = append(missing, "DARKPHISH_SECRET_ENCRYPTION_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("production security configuration is incomplete; set %s (or configured key files)", strings.Join(missing, ", "))
	}
	for name, value := range map[string]string{
		"DARKPHISH_SESSION_AUTH_KEY":       c.Session.AuthKey,
		"DARKPHISH_SESSION_ENCRYPTION_KEY": c.Session.EncryptionKey,
		"DARKPHISH_SECRET_ENCRYPTION_KEY":  c.Secrets.EncryptionKey,
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
