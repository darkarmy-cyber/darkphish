package models

import (
	"encoding/json"
	"errors"

	log "github.com/darkarmy-cyber/darkphish/logger"
)

// Webhook represents the webhook model
type Webhook struct {
	Id        int64  `json:"id" gorm:"column:id; primary_key:yes"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Secret    string `json:"-"`
	SecretSet bool   `json:"secret_set" gorm:"-"`
	IsActive  bool   `json:"is_active"`
}

// UnmarshalJSON accepts a write-only signing secret.
func (wh *Webhook) UnmarshalJSON(data []byte) error {
	type alias Webhook
	payload := struct {
		Secret string `json:"secret"`
		*alias
	}{alias: (*alias)(wh)}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	wh.Secret = payload.Secret
	wh.SecretSet = payload.Secret != ""
	return nil
}

func (wh *Webhook) openSecret() error {
	secret, err := secretStore.Open(wh.Secret)
	if err != nil {
		return err
	}
	wh.Secret = secret
	wh.SecretSet = secret != ""
	return nil
}

func (wh *Webhook) saveWithProtectedSecret() error {
	plain := wh.Secret
	protected, err := secretStore.Seal(plain)
	if err != nil {
		return err
	}
	wh.Secret = protected
	err = db.Save(wh).Error
	wh.Secret = plain
	wh.SecretSet = plain != ""
	return err
}

// ErrURLNotSpecified indicates there was no URL specified
var ErrURLNotSpecified = errors.New("URL can't be empty")

// ErrNameNotSpecified indicates there was no name specified
var ErrNameNotSpecified = errors.New("Name can't be empty")

// GetWebhooks returns the webhooks
func GetWebhooks() ([]Webhook, error) {
	whs := []Webhook{}
	err := db.Find(&whs).Error
	for i := range whs {
		if err == nil {
			err = whs[i].openSecret()
		}
	}
	return whs, err
}

// GetActiveWebhooks returns the active webhooks
func GetActiveWebhooks() ([]Webhook, error) {
	whs := []Webhook{}
	err := db.Where("is_active=?", true).Find(&whs).Error
	for i := range whs {
		if err == nil {
			err = whs[i].openSecret()
		}
	}
	return whs, err
}

// GetWebhook returns the webhook that the given id corresponds to.
// If no webhook is found, an error is returned.
func GetWebhook(id int64) (Webhook, error) {
	wh := Webhook{}
	err := db.Where("id=?", id).First(&wh).Error
	if err == nil {
		err = wh.openSecret()
	}
	return wh, err
}

// PostWebhook creates a new webhook in the database.
func PostWebhook(wh *Webhook) error {
	err := wh.Validate()
	if err != nil {
		log.Error(err)
		return err
	}
	err = wh.saveWithProtectedSecret()
	if err != nil {
		log.Error(err)
	}
	return err
}

// PutWebhook edits an existing webhook in the database.
func PutWebhook(wh *Webhook) error {
	err := wh.Validate()
	if err != nil {
		log.Error(err)
		return err
	}
	err = wh.saveWithProtectedSecret()
	return err
}

// DeleteWebhook deletes an existing webhook in the database.
// An error is returned if a webhook with the given id isn't found.
func DeleteWebhook(id int64) error {
	err := db.Where("id=?", id).Delete(&Webhook{}).Error
	return err
}

func (wh *Webhook) Validate() error {
	if wh.URL == "" {
		return ErrURLNotSpecified
	}
	if wh.Name == "" {
		return ErrNameNotSpecified
	}
	return nil
}
