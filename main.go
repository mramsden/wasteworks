package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	start := time.Now()

	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	logger.Info("handling request for calendar", "user-agent", r.Header.Get("User-Agent"))

	ctx := r.Context()

	pReq, err := http.NewRequestWithContext(ctx, "GET", cookieURL, nil)
	if err != nil {
		http.Error(w, "error creating proxy request", http.StatusInternalServerError)
		return
	}

	pReq.Header = http.Header{
		"Accept":     []string{"*/*"},
		"User-Agent": []string{"wasteworks/1.0 (admin@bitsden.com)"},
	}

	pResp, err := proxyTransport.RoundTrip(pReq)
	if err != nil {
		logger.Warn("failed starting wasteworks session", "error", err)
		http.Error(w, "error sending proxy request", http.StatusInternalServerError)
		return
	}
	pResp.Body.Close()

	headers := ""
	for name, values := range pResp.Header {
		for _, value := range values {
			headers += fmt.Sprintf("%s: %s\n", name, value)
		}
	}

	var sessionCookie *http.Cookie
	for _, cookie := range pResp.Cookies() {
		if cookie.Name == "fixmystreet_app_session" {
			sessionCookie = cookie
		}
	}

	ctx, cancel := context.WithTimeout(ctx, defaultWriteTimeout-time.Now().Sub(start))
	defer cancel()

	pReq, err = http.NewRequestWithContext(ctx, "GET", icsURL, nil)
	if err != nil {
		http.Error(w, "error creating proxy request", http.StatusInternalServerError)
		return
	}

	pReq.Header = http.Header{
		"Accept":     []string{"*/*"},
		"Referer":    []string{cookieURL},
		"User-Agent": []string{"wasteworks/1.0 (admin@bitsden.com)"},
	}
	pReq.AddCookie(sessionCookie)

	pResp, err = proxyTransport.RoundTrip(pReq)
	if err != nil {
		logger.Warn("failed to fetch calendar", "error", err)
		http.Error(w, "error sending proxy request", http.StatusInternalServerError)
		return
	}
	defer pResp.Body.Close()
	if pResp.StatusCode != http.StatusOK {
		logger.Warn("failed to fetch calendar, unexpected status code from wasteworks", "url", pReq.URL.String(), "status", pResp.StatusCode)
		http.Error(w, "error retrieving calendar", http.StatusInternalServerError)
		return
	}

	for name, values := range pResp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(pResp.StatusCode)
	io.Copy(w, pResp.Body)
}
