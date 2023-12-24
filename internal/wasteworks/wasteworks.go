package wasteworks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	log "bitsden.com/wasteworks/internal/logger"
)

var logger *slog.Logger

func init() {
	logger = log.Logger.With(slog.String("module", "wasteworks"))
}

func FetchCalendarWithContext(ctx context.Context) (resp *http.Response, err error) {
	wc := NewClient()
	for resp == nil && !errors.Is(err, context.DeadlineExceeded) {
		resp, err = wc.FetchCalendar(ctx)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			logger.Debug("failed fetching calendar", slog.String("err", err.Error()))
			time.Sleep(time.Second)
		}
	}
	logger.Info("completed wasteworks query", slog.Int("session-requests", wc.SessionRequests), slog.Int("calendar-requests", wc.CalendarRequests))
	return
}