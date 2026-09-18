package mailer

import (
	"context"
	"errors"
	"testing"
)

type pausedTestMail struct {
	Mail
	paused bool
}

func (m *pausedTestMail) PauseDelivery(error) error { m.paused = true; return nil }

func TestAuthorizationBlocksSMTPAndRetainsUnsentMail(t *testing.T) {
	denied := errors.New("license required")
	for _, scenario := range []string{"missing", "after-first-send", "before-reconnect"} {
		t.Run(scenario, func(t *testing.T) {
			allowed := scenario != "missing"
			check := func() error {
				if !allowed {
					return denied
				}
				return nil
			}
			dialer := newMockDialer()
			sender := newMockSender()
			sender.setSend(func(*mockMessage) error {
				allowed = false
				if scenario == "before-reconnect" {
					return errors.New("connection interrupted")
				}
				return nil
			})
			dialer.setDial(func() (Sender, error) { return sender, nil })
			original := generateMessages(dialer)
			first, second := &pausedTestMail{Mail: original[0]}, &pausedTestMail{Mail: original[1]}
			sendMail(context.Background(), dialer, []Mail{first, second}, check)
			wantDials := 1
			if scenario == "missing" {
				wantDials = 0
			}
			if dialer.dialCount != wantDials || len(sender.messages) != wantDials {
				t.Fatal("unauthorized SMTP operation")
			}
			if !second.paused || second.Mail.(*mockMessage).finished {
				t.Fatal("unsent recipient was not retained")
			}
			if scenario == "after-first-send" {
				if first.paused || !first.Mail.(*mockMessage).finished {
					t.Fatal("already accepted result was lost")
				}
			} else if !first.paused {
				t.Fatal("first unsent recipient lost")
			}
		})
	}
}

func TestAuthorizedMailerRejectsNilCallbackAndReportsTestEmailError(t *testing.T) {
	mw := NewAuthorizedMailWorker(nil)
	dialer := newMockDialer()
	messages := generateMessages(dialer)
	sendMail(context.Background(), dialer, messages, mw.authorize)
	if dialer.dialCount != 0 {
		t.Fatal("missing authorization opened SMTP")
	}
	for _, m := range messages {
		if m.(*mockMessage).err == nil {
			t.Fatal("synchronous request did not receive denial")
		}
	}
}

func TestAuthorizedMailerAllowsLicensedMessages(t *testing.T) {
	dialer := newMockDialer()
	sender := newMockSender()
	sender.setSend(func(*mockMessage) error { return nil })
	dialer.setDial(func() (Sender, error) { return sender, nil })
	messages := generateMessages(dialer)
	checks := 0
	sendMail(context.Background(), dialer, messages, func() error { checks++; return nil })
	if len(sender.messages) != 2 || checks < 3 {
		t.Fatal("licensed messages not checked and sent")
	}
}
