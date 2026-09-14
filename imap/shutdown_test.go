package imap

import (
	"context"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/models"
)

func TestShutdownBeforeServingGateIsSafe(t *testing.T) {
	im := NewMonitor()
	if err := im.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := im.Start(); err != nil {
		t.Fatal(err)
	}
	if err := im.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownJoinsActiveIMAPPoll(t *testing.T) {
	im := NewMonitor()
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	im.users = func() ([]models.User, error) { return []models.User{{Id: 1}}, nil }
	im.poll = func(int64, context.Context) {
		close(entered)
		<-release
		close(finished)
	}
	if err := im.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not start")
	}
	done := make(chan struct{})
	go func() { _ = im.Shutdown(); close(done) }()
	select {
	case <-done:
		t.Fatal("shutdown abandoned an active IMAP report batch")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("IMAP drain did not finish")
	}
	select {
	case <-finished:
	default:
		t.Fatal("poll result was not finished")
	}
}
