package licensing

import (
	"context"
	"time"
)

// RunRefresh refreshes once on startup and then periodically. Each call completes
// before the next starts, and cancellation reaches an in-flight HTTP request.
func RunRefresh(ctx context.Context, interval time.Duration, refresh func(context.Context)) {
	if interval <= 0 || ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
