package worker

import (
	"context"
	"fmt"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/mailer"
	"github.com/darkarmy-cyber/darkphish/models"
)

type logMailer struct {
	queue chan []mailer.Mail
}

type drainingMailer struct {
	started chan struct{}
	release chan struct{}
}

func (m *drainingMailer) Start(ctx context.Context) {
	close(m.started)
	<-ctx.Done()
	<-m.release
}

func (m *drainingMailer) Queue([]mailer.Mail) {}

func TestShutdownWaitsForMailer(t *testing.T) {
	m := &drainingMailer{started: make(chan struct{}), release: make(chan struct{})}
	w := &DefaultWorker{mailer: m}
	go w.Start()
	<-m.started
	done := make(chan struct{})
	go func() { w.Shutdown(); close(done) }()
	select {
	case <-done:
		t.Fatal("worker did not wait for SMTP drain")
	case <-time.After(50 * time.Millisecond):
	}
	close(m.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	if w.begin() {
		t.Fatal("worker accepted work after shutdown")
	}
}

func (m *logMailer) Start(ctx context.Context) {}

func (m *logMailer) Queue(ms []mailer.Mail) {
	m.queue <- ms
}

// testContext is context to cover API related functions
type testContext struct {
	config *config.Config
}

func setupTest(t *testing.T) *testContext {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	err := models.Setup(conf)
	if err != nil {
		t.Fatalf("Failed creating database: %v", err)
	}
	ctx := &testContext{}
	ctx.config = conf
	createTestData(t, ctx)
	return ctx
}

func createTestData(t *testing.T, ctx *testContext) {
	ctx.config.TestFlag = true
	// Add a group
	group := models.Group{Name: "Test Group"}
	for i := 0; i < 10; i++ {
		group.Targets = append(group.Targets, models.Target{
			BaseRecipient: models.BaseRecipient{
				Email:     fmt.Sprintf("test%d@example.com", i),
				FirstName: "First",
				LastName:  "Example"}})
	}
	group.UserId = 1
	models.PostGroup(&group)

	// Add a template
	template := models.Template{Name: "Test Template"}
	template.Subject = "Test subject"
	template.Text = "Text text"
	template.HTML = "<html>Test</html>"
	template.UserId = 1
	models.PostTemplate(&template)

	// Add a landing page
	p := models.Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	models.PostPage(&p)

	// Add a sending profile
	smtp := models.SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	models.PostSMTP(&smtp)
}

func setupCampaign(id int) (*models.Campaign, error) {
	// Setup and "launch" our campaign
	// Set the status such that no emails are attempted
	c := models.Campaign{Name: fmt.Sprintf("Test campaign - %d", id)}
	c.UserId = 1
	template, err := models.GetTemplate(1, 1)
	if err != nil {
		return nil, err
	}
	c.Template = template

	page, err := models.GetPage(1, 1)
	if err != nil {
		return nil, err
	}
	c.Page = page

	smtp, err := models.GetSMTP(1, 1)
	if err != nil {
		return nil, err
	}
	c.SMTP = smtp

	group, err := models.GetGroup(1, 1)
	if err != nil {
		return nil, err
	}
	c.Groups = []models.Group{group}
	err = models.PostCampaign(&c, c.UserId)
	if err != nil {
		return nil, err
	}
	err = c.UpdateStatus(models.CampaignEmailsSent)
	return &c, err
}

func TestMailLogGrouping(t *testing.T) {
	setupTest(t)

	// Create the campaigns and unlock the maillogs so that they're picked up
	// by the worker
	for i := 0; i < 10; i++ {
		campaign, err := setupCampaign(i)
		if err != nil {
			t.Fatalf("error creating campaign: %v", err)
		}
		ms, err := models.GetMailLogsByCampaign(campaign.Id)
		if err != nil {
			t.Fatalf("error getting maillogs for campaign: %v", err)
		}
		for _, m := range ms {
			m.Unlock()
		}
	}

	lm := &logMailer{queue: make(chan []mailer.Mail)}
	worker := &DefaultWorker{}
	worker.mailer = lm

	// Trigger the worker, generating the maillogs and sending them to the
	// mailer
	worker.processCampaigns(time.Now())

	// Verify that each slice of maillogs received belong to the same campaign
	for i := 0; i < 10; i++ {
		ms := <-lm.queue
		maillog, ok := ms[0].(*models.MailLog)
		if !ok {
			t.Fatalf("unable to cast mail to models.MailLog")
		}
		expected := maillog.CampaignId
		for _, m := range ms {
			maillog, ok = m.(*models.MailLog)
			if !ok {
				t.Fatalf("unable to cast mail to models.MailLog")
			}
			got := maillog.CampaignId
			if got != expected {
				t.Fatalf("unexpected campaign ID received for maillog: got %d expected %d", got, expected)
			}
		}
	}
}

func TestUnlicensedScheduledCampaignRemainsRetryable(t *testing.T) {
	setupTest(t)
	campaign, err := setupCampaign(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := campaign.UpdateStatus(models.CampaignQueued); err != nil {
		t.Fatal(err)
	}
	logs, err := models.GetMailLogsByCampaign(campaign.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.LockMailLogs(logs, false); err != nil {
		t.Fatal(err)
	}
	manager, err := licensing.OpenManager(filepath.Join(t.TempDir(), "license.json"), nil, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	models.ConfigureLicenseManager(manager)
	t.Cleanup(func() { models.ConfigureLicenseManager(nil) })
	mail := &logMailer{queue: make(chan []mailer.Mail, 10)}
	w := &DefaultWorker{mailer: mail}
	if err := w.processCampaigns(time.Now()); err == nil {
		t.Fatal("unlicensed scheduled campaign was accepted")
	}
	w.LaunchCampaign(*campaign)
	if len(mail.queue) != 0 {
		t.Fatal("unlicensed campaign reached mailer")
	}
	logs, err = models.GetMailLogsByCampaign(campaign.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range logs {
		if entry.Processing {
			t.Fatal("denied mail stayed locked")
		}
	}
	current, err := models.GetCampaignMailContext(campaign.Id, campaign.UserId)
	if err != nil || current.Status != models.CampaignQueued {
		t.Fatalf("denied campaign changed status: %v", err)
	}
}
