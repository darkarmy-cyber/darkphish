package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"

	ctx "github.com/darkarmy-cyber/darkphish/context"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/sirupsen/logrus"
)

// SendTestEmail sends a test email using the template name
// and Target given.
func (as *Server) SendTestEmail(w http.ResponseWriter, r *http.Request) {
	s := &models.EmailRequest{
		ErrorChan: make(chan error),
		UserId:    ctx.Get(r, "user_id").(int64),
	}
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusBadRequest)
		return
	}
	if err := models.CheckSendingLicense(); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusForbidden)
		return
	}
	err := json.NewDecoder(r.Body).Decode(s)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Error decoding JSON Request"}, http.StatusBadRequest)
		return
	}

	storeRequest := false

	// If a Template is not specified use a default
	if s.Template.Name == "" {
		//default message body
		text := "It works!\n\nThis is an email letting you know that your darkphish\nconfiguration was successful.\n" +
			"Here are the details:\n\nWho you sent from: {{.From}}\n\nWho you sent to: \n" +
			"{{if .FirstName}} First Name: {{.FirstName}}\n{{end}}" +
			"{{if .LastName}} Last Name: {{.LastName}}\n{{end}}" +
			"{{if .Position}} Position: {{.Position}}\n{{end}}" +
			"\nNow go send some phish!"
		t := models.Template{
			Subject: "Default Email from DarkPhish",
			Text:    text,
		}
		s.Template = t
	} else {
		// Get the Template requested by name
		s.Template, err = models.GetTemplateByName(s.Template.Name, s.UserId)
		if errors.Is(err, models.ErrRecordNotFound) {
			log.WithFields(logrus.Fields{
				"template": s.Template.Name,
			}).Error("Template does not exist")
			JSONResponse(w, models.Response{Success: false, Message: models.ErrTemplateNotFound.Error()}, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		s.TemplateId = s.Template.Id
		// We'll only save the test request to the database if there is a
		// user-specified template to use.
		storeRequest = true
	}

	if s.Page.Name != "" {
		s.Page, err = models.GetPageByName(s.Page.Name, s.UserId)
		if errors.Is(err, models.ErrRecordNotFound) {
			log.WithFields(logrus.Fields{
				"page": s.Page.Name,
			}).Error("Page does not exist")
			JSONResponse(w, models.Response{Success: false, Message: models.ErrPageNotFound.Error()}, http.StatusBadRequest)
			return
		} else if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		s.PageId = s.Page.Id
	}

	// Saved profile passwords are write-only and are never returned to the browser.
	// When testing an existing profile with a blank password field, restore only
	// that user's stored password server-side. Any explicitly entered password
	// remains a one-off test value and is not replaced here.
	if s.SMTP.Id != 0 && s.SMTP.Password == "" {
		stored, lookupErr := models.GetSMTP(s.SMTP.Id, s.UserId)
		if lookupErr != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Sending profile not found"}, http.StatusBadRequest)
			return
		}
		if err := s.SMTP.ReusePassword(stored); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
	}

	// If a complete sending profile is provided use it
	if err := s.SMTP.Validate(); err != nil {
		// Otherwise get the SMTP requested by name
		smtp, lookupErr := models.GetSMTPByName(s.SMTP.Name, s.UserId)
		// If the Sending Profile doesn't exist, let's err on the side
		// of caution and assume that the validation failure was more important.
		if lookupErr != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		s.SMTP = smtp
	}

	_, err = mail.ParseAddress(s.Template.EnvelopeSender)
	if err != nil {
		_, err = mail.ParseAddress(s.SMTP.FromAddress)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		} else {
			s.FromAddress = s.SMTP.FromAddress
		}
	} else {
		s.FromAddress = s.Template.EnvelopeSender
	}

	// Validate the given request
	if err = s.Validate(); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
		return
	}

	// Store the request if this wasn't the default template
	if storeRequest {
		err = models.PostEmailRequest(s)
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	// Send the test email
	err = as.worker.SendTestEmail(s)
	if err != nil {
		if errors.Is(err, models.ErrSendingLicenseRequired) {
			JSONResponse(w, models.Response{Success: false, Message: models.ErrSendingLicenseRequired.Error()}, http.StatusForbidden)
			return
		}
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Email Sent"}, http.StatusOK)
}
