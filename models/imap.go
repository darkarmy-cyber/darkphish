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

var ErrIMAPHostNotSpecified = errors.New("No IMAP Host specified")
var ErrIMAPPortNotSpecified = errors.New("No IMAP Port specified")
var ErrInvalidIMAPHost = errors.New("Invalid IMAP server address")
var ErrInvalidIMAPPort = errors.New("Invalid IMAP Port")
var ErrIMAPUsernameNotSpecified = errors.New("No Username specified")
var ErrIMAPPasswordNotSpecified = errors.New("No Password specified")
var ErrIMAPConcurrentUpdate = errors.New("IMAP settings changed concurrently; retry the update")
var ErrIMAPAmbiguousState = errors.New("IMAP settings are ambiguous; administrative repair is required")
var ErrInvalidIMAPFreq = errors.New("Invalid polling frequency")

func (im IMAP) TableName() string { return "imap" }

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
	if im.Folder == "" {
		im.Folder = DefaultIMAPFolder
	}
	ip := net.ParseIP(im.Host)
	_, err := net.LookupHost(im.Host)
	if ip == nil && err != nil {
		return ErrInvalidIMAPHost
	}
	if im.IMAPFreq < 30 || im.IMAPFreq > 31540000 {
		im.IMAPFreq = DefaultIMAPFreq
	}
	return nil
}

func (im *IMAP) Validate() error { return im.validate(true) }

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
	return a.Enabled == b.Enabled && a.Host == b.Host && a.Port == b.Port && a.Username == b.Username && a.TLS == b.TLS && a.IgnoreCertErrors == b.IgnoreCertErrors && a.Folder == b.Folder && a.RestrictDomain == b.RestrictDomain && a.DeleteReportedCampaignEmail == b.DeleteReportedCampaignEmail && a.IMAPFreq == b.IMAPFreq
}

// uniqueIMAPForPasswordPreservation proves there is exactly one legacy row for
// the user and that it contains a password. Counting every row is intentional:
// even an empty/NULL-password duplicate can be selected later by legacy readers.
func uniqueIMAPForPasswordPreservation(tx *gorm.DB, uid int64) (*IMAP, error) {
	var rows []IMAP
	if err := tx.Where("user_id = ?", uid).Limit(2).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 1 {
		return nil, ErrIMAPAmbiguousState
	}
	if len(rows) == 0 || rows[0].Password == "" {
		return nil, ErrIMAPPasswordNotSpecified
	}
	return &rows[0], nil
}

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
		if _, err := uniqueIMAPForPasswordPreservation(tx, uid); err != nil {
			return err
		}
		apply := func() (*gorm.DB, error) {
			result := tx.Model(&IMAP{}).Where("user_id = ? AND password <> ''", uid).Updates(updates)
			return result, result.Error
		}
		result, err := apply()
		if err != nil {
			return err
		}
		if result.RowsAffected > 1 {
			return ErrIMAPAmbiguousState
		}
		current, err := uniqueIMAPForPasswordPreservation(tx, uid)
		if err != nil {
			return err
		}
		if result.RowsAffected == 1 && sameIMAPSettings(current, im) {
			return nil
		}
		if result.RowsAffected == 0 && sameIMAPSettings(current, im) {
			return nil
		}
		result, err = apply()
		if err != nil {
			return err
		}
		if result.RowsAffected > 1 {
			return ErrIMAPAmbiguousState
		}
		current, err = uniqueIMAPForPasswordPreservation(tx, uid)
		if err != nil {
			return err
		}
		if result.RowsAffected <= 1 && sameIMAPSettings(current, im) {
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
