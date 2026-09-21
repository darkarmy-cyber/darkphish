package models

import (
	"errors"
	"net/mail"
	"time"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Template models hold the attributes for an email template to be sent to targets
type Template struct {
	Id             int64        `json:"id" gorm:"column:id; primaryKey"`
	UserId         int64        `json:"-" gorm:"column:user_id"`
	Name           string       `json:"name"`
	EnvelopeSender string       `json:"envelope_sender"`
	Subject        string       `json:"subject"`
	Text           string       `json:"text"`
	HTML           string       `json:"html" gorm:"column:html"`
	ModifiedDate   time.Time    `json:"modified_date"`
	Attachments    []Attachment `json:"attachments" gorm:"-"`
}

// ErrTemplateNameNotSpecified is thrown when a template name is not specified
var ErrTemplateNameNotSpecified = errors.New("Template name not specified")

// ErrTemplateMissingParameter is thrown when a needed parameter is not provided
var ErrTemplateMissingParameter = errors.New("Need to specify at least plaintext or HTML content")

// ErrTemplateIDSpecified rejects create payloads that could otherwise upsert an existing row.
var ErrTemplateIDSpecified = errors.New("new templates must not specify an id")

// Validate checks the given template to make sure values are appropriate and complete
func (t *Template) Validate() error {
	switch {
	case t.Name == "":
		return ErrTemplateNameNotSpecified
	case t.Text == "" && t.HTML == "":
		return ErrTemplateMissingParameter
	case t.EnvelopeSender != "":
		_, err := mail.ParseAddress(t.EnvelopeSender)
		if err != nil {
			return err
		}
	}
	if err := ValidateTemplate(t.HTML); err != nil {
		return err
	}
	if err := ValidateTemplate(t.Text); err != nil {
		return err
	}
	for _, a := range t.Attachments {
		if err := a.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetTemplates returns the templates owned by the given user.
func GetTemplates(uid int64) ([]Template, error) {
	ts := []Template{}
	err := db.Where("user_id=?", uid).Find(&ts).Error
	if err != nil {
		log.Error(err)
		return ts, err
	}
	for i := range ts {
		// Get Attachments
		err = db.Where("template_id=?", ts[i].Id).Find(&ts[i].Attachments).Error
		if err == nil && len(ts[i].Attachments) == 0 {
			ts[i].Attachments = make([]Attachment, 0)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error(err)
			return ts, err
		}
	}
	return ts, err
}

// GetTemplate returns the template, if it exists, specified by the given id and user_id.
func GetTemplate(id int64, uid int64) (Template, error) {
	t := Template{}
	err := db.Where("user_id=? and id=?", uid, id).Take(&t).Error
	if err != nil {
		log.Error(err)
		return t, err
	}

	// Get Attachments
	err = db.Where("template_id=?", t.Id).Find(&t.Attachments).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error(err)
		return t, err
	}
	if err == nil && len(t.Attachments) == 0 {
		t.Attachments = make([]Attachment, 0)
	}
	return t, err
}

// GetTemplateByName returns the template, if it exists, specified by the given name and user_id.
func GetTemplateByName(n string, uid int64) (Template, error) {
	t := Template{}
	err := db.Where("user_id=? and name=?", uid, n).Take(&t).Error
	if err != nil {
		log.Error(err)
		return t, err
	}

	// Get Attachments
	err = db.Where("template_id=?", t.Id).Find(&t.Attachments).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error(err)
		return t, err
	}
	if err == nil && len(t.Attachments) == 0 {
		t.Attachments = make([]Attachment, 0)
	}
	return t, err
}

// PostTemplate creates a new template in the database.
func PostTemplate(t *Template) error {
	if t.Id != 0 {
		return ErrTemplateIDSpecified
	}
	if err := t.Validate(); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		for i := range t.Attachments {
			t.Attachments[i].Id = 0
			t.Attachments[i].TemplateId = t.Id
			if err := tx.Create(&t.Attachments[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PutTemplate edits an existing template in the database.
// Per the PUT Method RFC, it presumes all data for a template is provided.
func PutTemplate(t *Template) error {
	if err := t.Validate(); err != nil {
		return err
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing Template
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id=? AND user_id=?", t.Id, t.UserId).Take(&existing).Error; err != nil {
			return err
		}
		if err := tx.Where("template_id=?", t.Id).Delete(&Attachment{}).Error; err != nil {
			return err
		}
		for i := range t.Attachments {
			t.Attachments[i].Id = 0
			t.Attachments[i].TemplateId = t.Id
			if err := tx.Create(&t.Attachments[i]).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&Template{}).Where("id=? AND user_id=?", t.Id, t.UserId).
			Select("name", "envelope_sender", "subject", "text", "html", "modified_date").Updates(t)
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
	if err != nil {
		log.Error(err)
	}
	return err
}

// DeleteTemplate deletes an existing template in the database.
// An error is returned if a template with the given user id and template id is not found.
func DeleteTemplate(id int64, uid int64) error {
	// Delete attachments
	err := db.Where("template_id=?", id).Delete(&Attachment{}).Error
	if err != nil {
		log.Error(err)
		return err
	}

	// Finally, delete the template itself
	err = db.Where("user_id=?", uid).Delete(Template{Id: id}).Error
	if err != nil {
		log.Error(err)
		return err
	}
	return nil
}
