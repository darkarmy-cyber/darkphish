package webhook

import (
	"testing"
	"time"
)

func TestShutdownJoinsAcceptedDelivery(t *testing.T) {
	var group deliveryGroup
	entered := make(chan struct{})
	release := make(chan struct{})
	group.start(func() {
		close(entered)
		<-release
	})
	<-entered
	done := make(chan struct{})
	go func() {
		group.shutdown()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("shutdown abandoned accepted webhook")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after delivery")
	}
	called := make(chan struct{}, 1)
	group.start(func() { called <- struct{}{} })
	group.shutdown()
	select {
	case <-called:
		t.Fatal("delivery accepted after shutdown")
	default:
	}
}
