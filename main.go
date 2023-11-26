package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

var defaultWriteTimeout = 30 * time.Second
var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func main() {
	addr, ok := os.LookupEnv("HTTP_ADDR")
	if !ok {
		addr = "127.0.0.1:8080"
	}

	logger.Info("starting HTTP server", "addr", addr)

	s := http.Server{
		Addr:         addr,
		Handler:      http.HandlerFunc(calendarHandler),
		WriteTimeout: defaultWriteTimeout,
	}
	s.ListenAndServe()
}

const cookieURL = "https://recyclingservices.bromley.gov.uk/waste/6053242"
const icsURL = "https://recyclingservices.bromley.gov.uk/waste/6053242/calendar.ics"

var proxyTransport = retryableTransport{
	transport:     http.DefaultTransport,
	maxRetryDelay: 5 * time.Second,
}

func calendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	logger.Info("handling request for calendar", "user-agent", r.Header.Get("User-Agent"))

	ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(30*time.Second))
	defer cancel()

	var pResp *http.Response
	var err error
	wc := wasteworksClient{}
	for pResp == nil && !errors.Is(err, context.DeadlineExceeded) {
		pResp, err = wc.FetchCalendar(ctx)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			logger.Debug("failed fetching calendar", slog.String("err", err.Error()))
			time.Sleep(time.Second)
		}
	}
	logger.Info("completed wasteworks query", slog.Int("session-requests", wc.sessionRequests), slog.Int("calendar-requests", wc.calendarRequests))
	if err != nil {
		http.Error(w, "error retrieving data from wasteworks", http.StatusInternalServerError)
		return
	}
	defer pResp.Body.Close()

	// Copy headers from the calendar response to our actual request response
	for name, values := range pResp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(pResp.StatusCode)
	io.Copy(w, pResp.Body)
}

type wasteworksClient struct {
	sessionCookie    *http.Cookie
	sessionRequests  int
	calendarRequests int
}

// StartSession will create a new session with the fixmystreet web
// application. It saves the session cookie ready for other
// requests
func (c *wasteworksClient) StartSession(ctx context.Context) error {
	c.sessionRequests += 1
	req, err := http.NewRequestWithContext(ctx, "GET", cookieURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "wasteworks/1.0 (admin@bitsden.com)")

	resp, err := proxyTransport.RoundTrip(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "fixmystreet_app_session" {
			c.sessionCookie = cookie
			return nil
		}
	}

	return errors.New("unable to find 'fixmystreet_app_session' cookie")
}

// FetchCalendar will fetch the appropriate iCal URL from fixmystreet
func (c *wasteworksClient) FetchCalendar(ctx context.Context) (*http.Response, error) {
	c.calendarRequests += 1

	var err error
	for c.sessionCookie == nil && !errors.Is(err, context.DeadlineExceeded) {
		err = c.StartSession(ctx)
		if err != nil {
			logger.Debug("failed starting session", slog.String("err", err.Error()))
		}
	}

	logger.Debug("fixmystreet session started")

	req, err := http.NewRequestWithContext(ctx, "GET", icsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "*/*")
	req.Header.Add("Referer", cookieURL)
	req.Header.Add("User-Agent", "wasteworks/1.0 (admin@bitsden.com)")
	req.AddCookie(c.sessionCookie)

	resp, err := proxyTransport.RoundTrip(req)
	if err != nil {
		logger.Warn("failed to fetch calendar", slog.String("error", err.Error()))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		logger.Warn("failed to fetch calendar, unexpected status code from wasteworks", slog.String("url", req.URL.String()), slog.Int("status", resp.StatusCode))
		return nil, err
	}

	contentType := resp.Header.Get("content-type")
	if !strings.HasPrefix(contentType, "text/calendar") {
		return nil, errors.New("missing expected content-type text/calendar")
	}

	return resp, nil
}
