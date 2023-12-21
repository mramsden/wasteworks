package wasteworks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const cookieURL = "https://recyclingservices.bromley.gov.uk/waste/6053242"
const icsURL = "https://recyclingservices.bromley.gov.uk/waste/6053242/calendar.ics"

var proxyTransport = newRetryableTransport()

type Client struct {
	sessionCookie *http.Cookie
	SessionRequests int
	CalendarRequests int
	logger *slog.Logger
}

func NewClient() Client {
	return Client{
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

// StartSession will create a new sessionwith the fixmystreet web
// application. It saves the session cookie ready for other
// requests
func (c *Client) StartSession(ctx context.Context) error {
	c.SessionRequests += 1
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
func (c *Client) FetchCalendar(ctx context.Context) (*http.Response, error) {
	c.CalendarRequests += 1

	var err error
	for c.sessionCookie == nil && !errors.Is(err, context.DeadlineExceeded) {
		err = c.StartSession(ctx)
		if err != nil {
			c.logger.Debug("failed starting session", slog.String("err", err.Error()))
		}
	}

	c.logger.Debug("fixmystreet session started")

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
		c.logger.Warn("failed to fetch calendar", slog.String("error", err.Error()))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("failed to fetch calendar, unexpected status code from wasteworks", slog.String("url", req.URL.String()), slog.Int("status", resp.StatusCode))
		return nil, err
	}

	contentType := resp.Header.Get("content-type")
	if !strings.HasPrefix(contentType, "text/calendar") {
		return nil, errors.New("missing expected content-type text/calendar")
	}

	return resp, nil
}