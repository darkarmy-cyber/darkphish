package mailer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"sync"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/gophish/gomail"
	"github.com/sirupsen/logrus"
)

// MaxReconnectAttempts is the maximum number of times we should reconnect to a server
var MaxReconnectAttempts = 10

// ErrMaxConnectAttempts is thrown when the maximum number of reconnect attempts
// is reached.
type ErrMaxConnectAttempts struct {
	underlyingError error
}

// Error returns the wrapped error response
func (e *ErrMaxConnectAttempts) Error() string {
	errString := "Max connection attempts exceeded"
	if e.underlyingError != nil {
		errString = fmt.Sprintf("%s - %s", errString, e.underlyingError.Error())
	}
	return errString
}

// Mailer is an interface that defines an object used to queue and
// send mailer.Mail instances.
type Mailer interface {
	Start(ctx context.Context)
	Queue([]Mail)
}

// Sender exposes the common operations required for sending email.
type Sender interface {
	Send(from string, to []string, msg io.WriterTo) error
	Close() error
	Reset() error
}

// Dialer dials to an SMTP server and returns the SendCloser
type Dialer interface {
	Dial() (Sender, error)
}

// Mail is an interface that handles the common operations for email messages
type Mail interface {
	Backoff(reason error) error
	Error(err error) error
	Success() error
	Generate(msg *gomail.Message) error
	GetDialer() (Dialer, error)
	GetSmtpFrom() (string, error)
}

// MailWorker is the worker that receives slices of emails
// on a channel to send. It's assumed that every slice of emails received is meant
// to be sent to the same server.
type MailWorker struct {
	queue     chan []Mail
	done      chan struct{}
	authorize func() error
}

// NewAuthorizedMailWorker rechecks authorization before connecting, reconnecting
// and sending each message. A missing callback is denied, not silently bypassed.
func NewAuthorizedMailWorker(authorize func() error) *MailWorker {
	mw := NewMailWorker()
	if authorize == nil {
		authorize = func() error { return errors.New("email sending authorization is unavailable") }
	}
	mw.authorize = authorize
	return mw
}

type authorizationError struct{ error }

func (e *authorizationError) Unwrap() error { return e.error }

// Campaign mail remains retryable after license restoration. Synchronous test
// emails receive an error rather than being silently deferred or auto-retried.
func pauseUnauthorizedMail(err error, ms []Mail) {
	for _, m := range ms {
		if paused, ok := m.(interface{ PauseDelivery(error) error }); ok {
			paused.PauseDelivery(err)
		} else {
			m.Error(err)
		}
	}
}

// NewMailWorker returns an instance of MailWorker with the mail queue
// initialized.
func NewMailWorker() *MailWorker {
	return &MailWorker{
		queue: make(chan []Mail),
		done:  make(chan struct{}),
	}
}

// Start launches the mail worker to begin listening on the Queue channel
// for new slices of Mail instances to process.
func (mw *MailWorker) Start(ctx context.Context) {
	var deliveries sync.WaitGroup
	defer func() {
		deliveries.Wait()
		close(mw.done)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case ms := <-mw.queue:
			if len(ms) == 0 {
				continue
			}
			deliveries.Add(1)
			go func(ctx context.Context, ms []Mail) {
				defer deliveries.Done()
				dialer, err := ms[0].GetDialer()
				if err != nil {
					errorMail(err, ms)
					return
				}
				sendMail(ctx, dialer, ms, mw.authorize)
			}(ctx, ms)
		}
	}
}

// Queue sends the provided mail to the internal queue for processing.
func (mw *MailWorker) Queue(ms []Mail) {
	select {
	case mw.queue <- ms:
	case <-mw.done:
	}
}

// errorMail is a helper to handle erroring out a slice of Mail instances
// in the case that an unrecoverable error occurs.
func errorMail(err error, ms []Mail) {
	for _, m := range ms {
		m.Error(err)
	}
}

// dialHost attempts to make a connection to the host specified by the Dialer.
// It returns MaxReconnectAttempts if the number of connection attempts has been
// exceeded.
func dialHost(ctx context.Context, dialer Dialer, checks ...func() error) (Sender, error) {
	sendAttempt := 0
	var sender Sender
	var err error
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, check := range checks {
			if check != nil {
				if err := check(); err != nil {
					return nil, &authorizationError{err}
				}
			}
		}
		sender, err = dialer.Dial()
		if err == nil {
			break
		}
		sendAttempt++
		if sendAttempt == MaxReconnectAttempts {
			err = &ErrMaxConnectAttempts{
				underlyingError: err,
			}
			break
		}
	}
	return sender, err
}

// sendMail attempts to send the provided Mail instances.
// If the context is cancelled before all of the mail are sent,
// sendMail just returns and does not modify those emails.
func sendMail(ctx context.Context, dialer Dialer, ms []Mail, authorize func() error) {
	sender, err := dialHost(ctx, dialer, authorize)
	if err != nil {
		var denied *authorizationError
		if errors.As(err, &denied) {
			pauseUnauthorizedMail(denied.error, ms)
			return
		}
		if ctx.Err() != nil {
			return
		}
		log.Warn(err)
		errorMail(err, ms)
		return
	}
	defer sender.Close()
	message := gomail.NewMessage()
	for i, m := range ms {
		if ctx.Err() != nil {
			return
		}
		if authorize != nil {
			if err := authorize(); err != nil {
				pauseUnauthorizedMail(err, ms[i:])
				return
			}
		}
		message.Reset()
		err = m.Generate(message)
		if err != nil {
			log.Warn(err)
			m.Error(err)
			continue
		}

		smtp_from, err := m.GetSmtpFrom()
		if err != nil {
			m.Error(err)
			continue
		}

		if authorize != nil {
			if err := authorize(); err != nil {
				pauseUnauthorizedMail(err, ms[i:])
				return
			}
		}
		err = gomail.SendCustomFrom(sender, smtp_from, message)
		if err != nil {
			if te, ok := err.(*textproto.Error); ok {
				switch {
				// If it's a temporary error, we should backoff and try again later.
				// We'll reset the connection so future messages don't incur a
				// different error (see https://github.com/darkarmy-cyber/darkphish/issues/787).
				case te.Code >= 400 && te.Code <= 499:
					log.WithFields(logrus.Fields{
						"code":  te.Code,
						"email": message.GetHeader("To")[0],
					}).Warn(err)
					m.Backoff(err)
					sender.Reset()
					continue
				// Otherwise, if it's a permanent error, we shouldn't backoff this message,
				// since the RFC specifies that running the same commands won't work next time.
				// We should reset our sender and error this message out.
				case te.Code >= 500 && te.Code <= 599:
					log.WithFields(logrus.Fields{
						"code":  te.Code,
						"email": message.GetHeader("To")[0],
					}).Warn(err)
					m.Error(err)
					sender.Reset()
					continue
				// If something else happened, let's just error out and reset the
				// sender
				default:
					log.WithFields(logrus.Fields{
						"code":  "unknown",
						"email": message.GetHeader("To")[0],
					}).Warn(err)
					m.Error(err)
					sender.Reset()
					continue
				}
			} else {
				// This likely indicates that something happened to the underlying
				// connection. We'll try to reconnect and, if that fails, we'll
				// error out the remaining emails.
				log.WithFields(logrus.Fields{
					"email": message.GetHeader("To")[0],
				}).Warn(err)
				origErr := err
				sender, err = dialHost(ctx, dialer, authorize)
				if err != nil {
					var denied *authorizationError
					if errors.As(err, &denied) {
						pauseUnauthorizedMail(denied.error, ms[i:])
						return
					}
					if ctx.Err() != nil {
						// Shutdown is not a permanent delivery failure. Leave the
						// current and unattempted logs eligible for a later retry.
						return
					}
					errorMail(err, ms[i:])
					break
				}
				m.Backoff(origErr)
				continue
			}
		}
		log.WithFields(logrus.Fields{
			"smtp_from":     smtp_from,
			"envelope_from": message.GetHeader("From")[0],
			"email":         message.GetHeader("To")[0],
		}).Info("Email sent")
		m.Success()
	}
}
