package licensing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshRunsImmediatelyAndPeriodicallyUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan struct{}, 10)
	done := make(chan struct{})
	go func() {
		RunRefresh(ctx, 10*time.Millisecond, func(context.Context) { calls <- struct{}{} })
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatal("refresh was not scheduled")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refresh did not stop")
	}
}

func TestRefreshCancelsInflightOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started, done := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	go func() {
		RunRefresh(ctx, time.Millisecond, func(ctx context.Context) { calls.Add(1); close(started); <-ctx.Done() })
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight refresh did not stop")
	}
	if calls.Load() != 1 {
		t.Fatal("overlapping refreshes")
	}
}
