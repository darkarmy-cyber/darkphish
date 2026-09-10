package models

import (
	"encoding/json"
	"errors"
	"net"
	"time"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"gorm.io/gorm"
)

const DefaultIMAPFolder = "INBOX"
const DefaultIMAPFreq = 60 // Every 60 seconds

// IMAP contains the attributes needed to handle logging into an IMAP server to check
// for reported emails
type IMAP struct {
	UserId                      int64      `json:"-" gorm:"column:user_id"`
	Enabled                     bool       `json:"enabled"`
	Host                        string     `json:"host"`
	Port                        uint16     `json:"port,string,omitempty"`
	Username                    string     `json:"username"`
	Password                    string     `json:"-"`
	PasswordSet                 bool       `json:"password_set" gorm:"-"`
	TLS                         bool       `json:"tls"`
	IgnoreCertErrors            bool       `json:"ignore_cert_errors"`
	Folder                      string     `json:"folder"`
	RestrictDomain              string     `json:"restrict_domain"`
	DeleteReportedCampaignEmail bool       `json:"delete_reported_campaign_email"`
	LastLogin                   *time.Time `json:"last_login,omitempty"`
	ModifiedDate                time.Time  `json:"modified_date"`
	IMAPFreq                    uint32     `json:"imap_freq,string,omitempty"`
}

// UnmarshalJSON accepts a write-only password while normal serialization
// exposes only password_set metadata.
func (im *IMAP) UnmarshalJSON(data []byte) error {
	type alias IMAP
	payload := struct {
		Password string `json:"password"`
		*alias
	}{alias: (*alias)(im)}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	im.Password = payload.Password
	im.PasswordSet = payload.Password != ""
	return nil
}

func (im *IMAP) openPassword() error {
	password, err := secretStore.Open(im.Password)
	if err != nil {
		return err
	}
	im.Password = password
	im.PasswordSet = password != ""
	return nil
}

// ErrIMAPHostNotSpecified is thrown when there is no Host specified
var ErrIMAPHostNotSpecified = errors.New("No IMAP Host specified")

// ErrIMAPPortNotSpecified is thrown when there is no Port specified
var ErrIMAPPortNotSpecified = errors.New("No IMAP Port specified")

// ErrInvalidIMAPHost indicates that the IMAP server string is invalid
var ErrInvalidIMAPHost = errors.New("Invalid IMAP server address")

// ErrInvalidIMAPPort indicates that the IMAP Port is invalid
var ErrInvalidIMAPPort = errors.New("Invalid IMAP Port")

// ErrIMAPUsernameNotSpecified is thrown when there is no Username specified
var ErrIMAPUsernameNotSpecified = errors.New("No Username specified")

// ErrIMAPPasswordNotSpecified is thrown when there is no Password specified
var ErrIMAPPasswordNotSpecified = errors.New("No Password specified")

// ErrIMAPConcurrentUpdate is returned when a password-preserving update loses a
// race with a concurrent replacement. The caller must retry from fresh state.
var ErrIMAPConcurrentUpdate = errors.New("IMAP settings changed concurrently; retry the update")

// ErrInvalidIMAPFreq is thrown when the frequency for polling the
// IMAP server is invalid
var ErrInvalidIMAPFreq = errors.New("Invalid polling frequency")

// TableName specifies the database tablename for Gorm to use
func (im IMAP) TableName() string {
	return "imap"
}

func (im *IMAP) validate(requirePassword bool) error {
	switch {
	case im.Host == "":
		return ErrIMAPHostNotSpecified
	case im.Port == 0:
		return ErrIMAPPortNotSpecified
	case im.Username == "":
		return ErrIMAPUsernameNotSpecified
	case requirePassword && im.Password == "":
		return ErrIMAPPasswordNotSpecified
	}

	// Set the default value for Folder
	if im.Folder == "" {
		im.Folder = DefaultIMAPFolder
	}

	// Make sure im.Host is an IP or hostname. NB will fail if unable to resolve the hostname.
	ip := net.ParseIP(im.Host)
	_, err := net.LookupHost(im.Host)
	if ip == nil && err != nil {
		return ErrInvalidIMAPHost
	}

	// Make sure the polling frequency is between every 30 seconds and every year
	// If not set it to the default
	if im.IMAPFreq < 30 || im.IMAPFreq > 31540000 {
		im.IMAPFreq = DefaultIMAPFreq
	}

	return nil
}

// Validate ensures that IMAP configs/connections are valid.
func (im *IMAP) Validate() error {
	return im.validate(true)
}

// GetIMAP returns the IMAP server owned by the given user.
func GetIMAP(uid int64) ([]IMAP, error) {
	im := []IMAP{}
	err := db.Where("user_id=?", uid).Find(&im).Error

	if err != nil {
		log.Error(err)
		return im, err
	}
	for i := range im {
		if err = im[i].openPassword(); err != nil {
			return im, err
		}
	}
	return im, nil
}

func sameIMAPSettings(a, b *IMAP) bool {
	return a.Enabled == b.Enabled &&
		a.Host == b.Host &&
		a.Port == b.Port &&
		a.Username == b.Username &&
		a.TLS == b.TLS &&
		a.IgnoreCertErrors == b.IgnoreCertErrors &&
		a.Folder == b.Folder &&
		a.RestrictDomain == b.RestrictDomain &&
		a.DeleteReportedCampaignEmail == b.DeleteReportedCampaignEmail &&
		a.IMAPFreq == b.IMAPFreq
}

// updateIMAPWithoutPassword updates only non-secret settings. The password
// column is deliberately excluded so an empty write-only password means
// "preserve the currently stored secret" without a read/decrypt/write race.
func updateIMAPWithoutPassword(im *IMAP, uid int64) error {
	if err := im.validate(false); err != nil {
		log.Error(err)
		return err
	}
	im.UserId = uid
	updates := map[string]interface{}{
		"enabled":                        im.Enabled,
		"host":                           im.Host,
		"port":                           im.Port,
		"username":                       im.Username,
		"tls":                            im.TLS,
		"ignore_cert_errors":             im.IgnoreCertErrors,
		"folder":                         im.Folder,
		"restrict_domain":                im.RestrictDomain,
		"delete_reported_campaign_email": im.DeleteReportedCampaignEmail,
		"modified_date":                  im.ModifiedDate,
		"imap_freq":                      im.IMAPFreq,
	}
	err := withSecurityTransaction(func(tx *gorm.DB) error {
		apply := func() (*gorm.DB, error) {
			result := tx.Model(&IMAP{}).Where("user_id = ? AND password <> ''", uid).Updates(updates)
			return result, result.Error
		}

		result, err := apply()
		if err != nil {
			return err
		}
		if result.RowsAffected == 1 {
			return nil
		}

		// MySQL reports changed rows, not matched rows, so an idempotent update can
		// legitimately report zero. Re-read the password-bearing row and accept the
		// request only when the persisted non-secret settings already match.
		var current IMAP
		if err := tx.Where("user_id = ? AND password <> ''", uid).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIMAPPasswordNotSpecified
			}
			return err
		}
		if sameIMAPSettings(&current, im) {
			return nil
		}

		// A concurrent replacement may have committed between the first update and
		// the re-read. Retry once against the fresh row while still excluding the
		// password column. If it still cannot be proven applied, fail closed.
		result, err = apply()
		if err != nil {
			return err
		}
		if result.RowsAffected == 1 {
			return nil
		}
		if err := tx.Where("user_id = ? AND password <> ''", uid).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIMAPPasswordNotSpecified
			}
			return err
		}
		if sameIMAPSettings(&current, im) {
			return nil
		}
		return ErrIMAPConcurrentUpdate
	})
	im.PasswordSet = err == nil
	if err != nil {
		log.Error("Unable to save to database: ", err.Error())
	}
	return err
}

// PostIMAP updates IMAP settings for a user in the database.
func PostIMAP(im *IMAP, uid int64) error {
	if im.ModifiedDate.IsZero() {
		im.ModifiedDate = time.Now().UTC()
	}
	if im.Password == "" {
		return updateIMAPWithoutPassword(im, uid)
	}
	if err := im.Validate(); err != nil {
		log.Error(err)
		return err
	}

	// Protect first, then atomically replace the user's row. This legacy table
	// has no primary key, so inserting settings must not use GORM's Save upsert.
	plain := im.Password
	protected, protectErr := secretStore.Seal(plain)
	if protectErr != nil {
		return protectErr
	}
	im.Password = protected
	im.UserId = uid
	err := withSecurityTransaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id=?", uid).Delete(&IMAP{}).Error; err != nil {
			return err
		}
		return tx.Create(im).Error
	})
	im.Password = plain
	im.PasswordSet = plain != ""
	if err != nil {
		log.Error("Unable to save to database: ", err.Error())
	}
	return err
}

// DeleteIMAP deletes the existing IMAP in the database.
func DeleteIMAP(uid int64) error {
	err := db.Where("user_id=?", uid).Delete(&IMAP{}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

func SuccessfulLogin(im *IMAP) error {
	err := db.Model(&im).Where("user_id = ?", im.UserId).Update("last_login", time.Now().UTC()).Error
	if err != nil {
		log.Error("Unable to update database: ", err.Error())
	}
	return err
}
