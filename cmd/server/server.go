package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"bitsden.com/wasteworks/internal/wasteworks"
)

const defaultWriteTimeout = 30 * time.Second

var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func main() {
	addr, ok := os.LookupEnv("HTTP_ADDR")
	if !ok {
		addr = "127.0.0.1:8080"
	}

	logger.Info("starting HTTP server", slog.String("addr", addr))

	s := http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(calendarHandler),
		WriteTimeout: defaultWriteTimeout,
	}
	s.ListenAndServe()
}

func calendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	logger.Info("handling request for calendar", slog.String("user-agent", r.Header.Get("User-Agent")))

	ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(30*time.Second))
	defer cancel()

	var pResp *http.Response
	var err error
	wc := wasteworks.NewClient()
	for pResp == nil && !errors.Is(err, context.DeadlineExceeded) {
		pResp, err = wc.FetchCalendar(ctx)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			logger.Debug("failed fetching calendar", slog.String("err", err.Error()))
			time.Sleep(time.Second)
		}
	}
	logger.Info("completed wasteworks query", slog.Int("session-requests", wc.SessionRequests), slog.Int("calendar-requests", wc.CalendarRequests))
	if err != nil {
		http.Error(w, "error retrieving data from wasteworks", http.StatusInternalServerError)
		return
	}
	defer pResp.Body.Close()

	// Copy headers from the calendar response to the actual request response
	for name, values := range pResp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(pResp.StatusCode)
	io.Copy(w, pResp.Body)
}