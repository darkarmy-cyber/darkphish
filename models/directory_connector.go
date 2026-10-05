package models

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

const DirectoryProviderEntra = "entra"

var (
	ErrDirectoryConnectorNameRequired   = errors.New("directory connector name is required")
	ErrDirectoryConnectorProvider       = errors.New("unsupported directory connector provider")
	ErrDirectoryConnectorTenant         = errors.New("invalid directory connector tenant")
	ErrDirectoryConnectorClientID       = errors.New("invalid directory connector client id")
	ErrDirectoryConnectorRemoteGroupID  = errors.New("invalid remote directory group id")
	ErrDirectoryConnectorClientSecret   = errors.New("directory connector client secret is required")
	ErrDirectoryConnectorSyncInterval   = errors.New("directory connector sync interval must be 0 or between 15 and 10080 minutes")
	ErrDirectoryConnectorTargetGroup    = errors.New("directory connector target group is invalid")
	ErrDirectoryConnectorConcurrentEdit = errors.New("directory connector changed concurrently; reload and retry")
)

var (
	directoryGUIDPattern   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	directoryTenantPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
)

type DirectoryConnector struct {
	ID                  int64      `json:"id" gorm:"column:id;primaryKey"`
	Name                string     `json:"name"`
	OwnerUserID         int64      `json:"-" gorm:"column:owner_user_id"`
	Provider            string     `json:"provider"`
	TenantID            string     `json:"tenant_id"`
	ClientID            string     `json:"client_id"`
	ClientSecret        string     `json:"-" gorm:"column:client_secret"`
	ClientSecretSet     bool       `json:"client_secret_set" gorm:"-"`
	RemoteGroupID       string     `json:"remote_group_id"`
	TargetGroupID       *int64     `json:"target_group_id,omitempty"`
	EmailDomainsJSON    string     `json:"-" gorm:"column:email_domains"`
	EmailDomains        []string   `json:"email_domains" gorm:"-"`
	Enabled             bool       `json:"enabled"`
	SyncIntervalMinutes int        `json:"sync_interval_minutes"`
	LastSyncAt          *time.Time `json:"last_sync_at,omitempty"`
	NextSyncAt          *time.Time `json:"next_sync_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	ModifiedAt          time.Time  `json:"modified_at"`
}

func (DirectoryConnector) TableName() string { return "directory_connectors" }

type DirectorySyncRun struct {
	ID          int64      `json:"id" gorm:"column:id;primaryKey"`
	ConnectorID int64      `json:"connector_id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Discovered  int        `json:"discovered"`
	Added       int        `json:"added"`
	Updated     int        `json:"updated"`
	Removed     int        `json:"removed"`
	Skipped     int        `json:"skipped"`
	ErrorCode   string     `json:"error_code,omitempty"`
}

func (DirectorySyncRun) TableName() string { return "directory_sync_runs" }

func (c *DirectoryConnector) UnmarshalJSON(data []byte) error {
	type alias DirectoryConnector
	payload := struct {
		ClientSecret string `json:"client_secret"`
		*alias
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	c.ClientSecret = payload.ClientSecret
	c.ClientSecretSet = payload.ClientSecret != ""
	return nil
}

func normalizeDirectoryDomains(domains []string) ([]string, error) {
	if len(domains) > 100 {
		return nil, errors.New("too many directory email domains")
	}
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	total := 0
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
		if domain == "" || strings.ContainsAny(domain, " /\\") || len(domain) > 253 {
			return nil, errors.New("invalid directory email domain")
		}
		total += len(domain)
		if total > 8192 {
			return nil, errors.New("directory email domains exceed size limit")
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out, nil
}

func (c *DirectoryConnector) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.TenantID = strings.TrimSpace(c.TenantID)
	c.ClientID = strings.ToLower(strings.TrimSpace(c.ClientID))
	c.RemoteGroupID = strings.ToLower(strings.TrimSpace(c.RemoteGroupID))
	switch {
	case c.Name == "" || len(c.Name) > 255:
		return ErrDirectoryConnectorNameRequired
	case c.Provider != DirectoryProviderEntra:
		return ErrDirectoryConnectorProvider
	case !directoryTenantPattern.MatchString(c.TenantID):
		return ErrDirectoryConnectorTenant
	case !directoryGUIDPattern.MatchString(c.ClientID):
		return ErrDirectoryConnectorClientID
	case !directoryGUIDPattern.MatchString(c.RemoteGroupID):
		return ErrDirectoryConnectorRemoteGroupID
	case c.SyncIntervalMinutes != 0 && (c.SyncIntervalMinutes < 15 || c.SyncIntervalMinutes > 10080):
		return ErrDirectoryConnectorSyncInterval
	}
	if c.TargetGroupID != nil {
		if *c.TargetGroupID <= 0 {
			return ErrDirectoryConnectorTargetGroup
		}
		var count int64
		if err := db.Model(&Group{}).Where("id=? AND user_id=?", *c.TargetGroupID, c.OwnerUserID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrDirectoryConnectorTargetGroup
		}
	}
	domains, err := normalizeDirectoryDomains(c.EmailDomains)
	if err != nil {
		return err
	}
	c.EmailDomains = domains
	encoded, err := json.Marshal(domains)
	if err != nil {
		return err
	}
	c.EmailDomainsJSON = string(encoded)
	return nil
}

func hydrateDirectoryConnector(c *DirectoryConnector) error {
	if strings.TrimSpace(c.EmailDomainsJSON) == "" {
		c.EmailDomainsJSON = "[]"
	}
	if err := json.Unmarshal([]byte(c.EmailDomainsJSON), &c.EmailDomains); err != nil {
		return err
	}
	c.ClientSecretSet = c.ClientSecret != ""
	c.ClientSecret = ""
	return nil
}

func GetDirectoryConnectors(ownerUserID int64) ([]DirectoryConnector, error) {
	var connectors []DirectoryConnector
	if err := db.Where("owner_user_id=?", ownerUserID).Order("id ASC").Find(&connectors).Error; err != nil {
		return nil, err
	}
	for i := range connectors {
		if err := hydrateDirectoryConnector(&connectors[i]); err != nil {
			return nil, err
		}
	}
	return connectors, nil
}

func GetDirectoryConnector(id, ownerUserID int64) (DirectoryConnector, error) {
	var connector DirectoryConnector
	if err := db.Where("id=? AND owner_user_id=?", id, ownerUserID).Take(&connector).Error; err != nil {
		return connector, err
	}
	if err := hydrateDirectoryConnector(&connector); err != nil {
		return connector, err
	}
	return connector, nil
}

func getDirectoryConnectorStored(id, ownerUserID int64) (DirectoryConnector, error) {
	var connector DirectoryConnector
	err := db.Where("id=? AND owner_user_id=?", id, ownerUserID).Take(&connector).Error
	return connector, err
}

func GetDirectoryConnectorWithSecret(id, ownerUserID int64) (DirectoryConnector, error) {
	connector, err := getDirectoryConnectorStored(id, ownerUserID)
	if err != nil {
		return connector, err
	}
	secret, err := secretStore.Open(connector.ClientSecret)
	if err != nil {
		return connector, err
	}
	connector.ClientSecret = secret
	connector.ClientSecretSet = secret != ""
	if strings.TrimSpace(connector.EmailDomainsJSON) == "" {
		connector.EmailDomainsJSON = "[]"
	}
	if err := json.Unmarshal([]byte(connector.EmailDomainsJSON), &connector.EmailDomains); err != nil {
		return connector, err
	}
	return connector, nil
}

func PostDirectoryConnector(connector *DirectoryConnector) error {
	if connector.ID != 0 {
		return errors.New("new directory connectors must not specify an id")
	}
	if connector.ClientSecret == "" {
		return ErrDirectoryConnectorClientSecret
	}
	if err := connector.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	connector.CreatedAt = now
	connector.ModifiedAt = now
	protected, err := secretStore.Seal(connector.ClientSecret)
	if err != nil {
		return err
	}
	plain := connector.ClientSecret
	connector.ClientSecret = protected
	err = db.Create(connector).Error
	connector.ClientSecret = plain
	connector.ClientSecretSet = plain != ""
	return err
}

func PutDirectoryConnector(connector *DirectoryConnector) error {
	if connector.ID <= 0 {
		return gorm.ErrRecordNotFound
	}
	stored, err := getDirectoryConnectorStored(connector.ID, connector.OwnerUserID)
	if err != nil {
		return err
	}
	if connector.ClientSecret == "" {
		if connector.TenantID != stored.TenantID || connector.ClientID != stored.ClientID {
			return ErrDirectoryConnectorClientSecret
		}
		connector.ClientSecret = stored.ClientSecret
	} else {
		protected, err := secretStore.Seal(connector.ClientSecret)
		if err != nil {
			return err
		}
		connector.ClientSecret = protected
	}
	if err := connector.Validate(); err != nil {
		return err
	}
	connector.CreatedAt = stored.CreatedAt
	connector.ModifiedAt = time.Now().UTC()
	result := db.Model(&DirectoryConnector{}).
		Where("id=? AND owner_user_id=? AND modified_at=?", connector.ID, connector.OwnerUserID, stored.ModifiedAt).
		Updates(map[string]interface{}{
			"name":                  connector.Name,
			"provider":              connector.Provider,
			"tenant_id":             connector.TenantID,
			"client_id":             connector.ClientID,
			"client_secret":         connector.ClientSecret,
			"remote_group_id":       connector.RemoteGroupID,
			"target_group_id":       connector.TargetGroupID,
			"email_domains":         connector.EmailDomainsJSON,
			"enabled":               connector.Enabled,
			"sync_interval_minutes": connector.SyncIntervalMinutes,
			"modified_at":           connector.ModifiedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrDirectoryConnectorConcurrentEdit
	}
	connector.ClientSecret = ""
	connector.ClientSecretSet = true
	return nil
}

func DeleteDirectoryConnector(id, ownerUserID int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("connector_id=?", id).Delete(&DirectorySyncRun{}).Error; err != nil {
			return err
		}
		result := tx.Where("id=? AND owner_user_id=?", id, ownerUserID).Delete(&DirectoryConnector{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func GetDirectorySyncRuns(connectorID, ownerUserID int64, limit int) ([]DirectorySyncRun, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	var count int64
	if err := db.Model(&DirectoryConnector{}).Where("id=? AND owner_user_id=?", connectorID, ownerUserID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	var runs []DirectorySyncRun
	err := db.Where("connector_id=?", connectorID).Order("started_at DESC, id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}
