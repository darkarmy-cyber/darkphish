package imap

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"

	"github.com/jordan-wright/email"
)

// Client interface for IMAP interactions
type Client interface {
	Login(username, password string) (cmd *imap.Command, err error)
	Logout(timeout time.Duration) (cmd *imap.Command, err error)
	Select(name string, readOnly bool) (mbox *imap.MailboxStatus, err error)
	Store(seq *imap.SeqSet, item imap.StoreItem, value interface{}, ch chan *imap.Message) (err error)
	Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) (err error)
}

// Email represents an email.Email with an included IMAP Sequence Number
type Email struct {
	SeqNum uint32 `json:"seqnum"`
	UID    uint32 `json:"uid"`
	*email.Email
}

// Mailbox holds onto the credentials and other information
// needed for connecting to an IMAP server.
type Mailbox struct {
	ctx              context.Context
	uidValidity      uint32
	Host             string
	TLS              bool
	IgnoreCertErrors bool
	User             string
	Pwd              string
	Folder           string
	// Read only mode, false (original logic) if not initialized
	ReadOnly bool
}

// Validate validates supplied IMAP model by connecting to the server
func Validate(s *models.IMAP) error {
	err := s.Validate()
	if err != nil {
		log.Error(err)
		return err
	}

	s.Host = s.Host + ":" + strconv.Itoa(int(s.Port)) // Append port
	mailServer := Mailbox{
		Host:             s.Host,
		TLS:              s.TLS,
		IgnoreCertErrors: s.IgnoreCertErrors,
		User:             s.Username,
		Pwd:              s.Password,
		Folder:           s.Folder}

	imapClient, err := mailServer.newClient()
	if err != nil {
		log.Error(err.Error())
	} else {
		imapClient.Logout()
	}
	return err
}

// MarkAsUnread will set the UNSEEN flag on a supplied slice of SeqNums
func (mbox *Mailbox) MarkAsUnread(seqs []uint32) error {
	return mbox.setSeen(seqs, false)
}

// MarkAsRead acknowledges messages only after report processing completes.
func (mbox *Mailbox) MarkAsRead(seqs []uint32) error {
	return mbox.setSeen(seqs, true)
}

func (mbox *Mailbox) setSeen(seqs []uint32, seen bool) error {
	imapClient, err := mbox.newClient()
	if err != nil {
		return err
	}

	defer imapClient.Logout()

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqs...)

	item := imap.FormatFlagsOp(imap.RemoveFlags, true)
	if seen {
		item = imap.FormatFlagsOp(imap.AddFlags, true)
	}
	err = imapClient.UidStore(seqSet, item, imap.SeenFlag, nil)
	if err != nil {
		return err
	}

	return nil

}

// DeleteEmails will delete emails from the supplied slice of SeqNums
func (mbox *Mailbox) DeleteEmails(seqs []uint32) error {
	imapClient, err := mbox.newClient()
	if err != nil {
		return err
	}

	defer imapClient.Logout()

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqs...)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	err = imapClient.UidStore(seqSet, item, imap.DeletedFlag, nil)
	if err != nil {
		return err
	}

	return nil
}

// GetUnread will find all unread emails in the folder and return them as a list.
func (mbox *Mailbox) GetUnread(markAsRead, delete bool) ([]Email, error) {
	var emails []Email

	imapClient, err := mbox.newClient()
	if err != nil {
		return emails, fmt.Errorf("failed to create IMAP connection: %s", err)
	}

	defer imapClient.Logout()

	// Search for unread emails
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	seqs, err := imapClient.UidSearch(criteria)
	if err != nil {
		return emails, err
	}

	if len(seqs) == 0 {
		return emails, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(seqs...)
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message)

	fetched := make(chan error, 1)
	go func() { fetched <- imapClient.UidFetch(seqset, items, messages) }()
	var parseErr error

	// Step through each email
	for msg := range messages {
		if parseErr != nil {
			continue
		}
		if msg.Uid == 0 {
			parseErr = errors.New("IMAP response omitted stable message UID")
			continue
		}
		// Extract raw message body. I can't find a better way to do this with the emersion library
		var em *email.Email
		var buf []byte
		for _, value := range msg.Body {
			if value.Len() > 25<<20 {
				parseErr = errors.New("IMAP message exceeds size limit")
				break
			}
			buf, err = io.ReadAll(io.LimitReader(value, (25<<20)+1))
			if err != nil {
				parseErr = err
			}
			break // There should only ever be one item in this map, but I'm not 100% sure
		}
		if parseErr != nil {
			continue
		}

		//Remove CR characters, see https://github.com/jordan-wright/email/issues/106
		tmp := string(buf)
		re := regexp.MustCompile(`\r`)
		tmp = re.ReplaceAllString(tmp, "")
		buf = []byte(tmp)

		rawBodyStream := bytes.NewReader(buf)
		em, err = email.NewEmailFromReader(rawBodyStream) // Parse with @jordanwright's library
		if err != nil {
			parseErr = err
			continue
		}

		emtmp := Email{Email: em, SeqNum: msg.SeqNum, UID: msg.Uid}
		emails = append(emails, emtmp)

	}
	if err := errors.Join(parseErr, <-fetched); err != nil {
		return nil, err
	}
	return emails, nil
}

func init() {
	imap.CharsetReader = charset.Reader
}

// newClient will initiate a new IMAP connection with the given creds.
func (mbox *Mailbox) newClient() (*boundedIMAPClient, error) {
	var imapClient *client.Client
	var err error
	ctx := mbox.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	restrictedDialer := &boundedIMAPDialer{dialer: dialer.Dialer(), ctx: ctx}
	if mbox.TLS {
		config := new(tls.Config)
		config.InsecureSkipVerify = mbox.IgnoreCertErrors
		imapClient, err = client.DialWithDialerTLS(restrictedDialer, mbox.Host, config)
	} else {
		imapClient, err = client.DialWithDialer(restrictedDialer, mbox.Host)
	}
	if err != nil {
		if restrictedDialer.conn != nil {
			_ = restrictedDialer.conn.Close()
		}
		return nil, err
	}
	bounded := &boundedIMAPClient{Client: imapClient, conn: restrictedDialer.conn}
	imapClient.Timeout = 30 * time.Second

	err = imapClient.Login(mbox.User, mbox.Pwd)
	if err != nil {
		_ = bounded.conn.Close()
		return nil, err
	}

	status, err := imapClient.Select(mbox.Folder, mbox.ReadOnly)
	if err != nil {
		_ = bounded.conn.Close()
		return nil, err
	}
	if status.UidValidity == 0 || (mbox.uidValidity != 0 && mbox.uidValidity != status.UidValidity) {
		_ = bounded.conn.Close()
		return nil, errors.New("IMAP mailbox identity changed; leaving messages unread")
	}
	mbox.uidValidity = status.UidValidity

	return bounded, nil
}

type boundedIMAPClient struct {
	*client.Client
	conn net.Conn
}

func (c *boundedIMAPClient) Logout() error {
	return errors.Join(c.Client.Logout(), c.conn.Close())
}

type boundedIMAPDialer struct {
	dialer *net.Dialer
	ctx    context.Context
	conn   net.Conn
}

func (d *boundedIMAPDialer) Dial(network, address string) (net.Conn, error) {
	conn, err := d.dialer.DialContext(d.ctx, network, address)
	if err != nil {
		return nil, err
	}
	d.conn, err = boundIMAPConnection(d.ctx, conn, 30*time.Second)
	return d.conn, err
}

type boundedIMAPConn struct {
	net.Conn
	deadline time.Time
	stop     func() bool
}

func boundIMAPConnection(ctx context.Context, conn net.Conn, timeout time.Duration) (net.Conn, error) {
	c := &boundedIMAPConn{Conn: conn, deadline: time.Now().Add(timeout)}
	if err := conn.SetDeadline(c.deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	c.stop = context.AfterFunc(ctx, func() { _ = conn.Close() })
	return c, nil
}

func (c *boundedIMAPConn) SetDeadline(deadline time.Time) error {
	// Library command setup must not clear/extend the whole-session bound,
	// including the capability exchange performed before client.New returns.
	if deadline.IsZero() || deadline.After(c.deadline) {
		deadline = c.deadline
	}
	return c.Conn.SetDeadline(deadline)
}

func (c *boundedIMAPConn) Close() error {
	c.stop()
	return c.Conn.Close()
}
