package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	addr, ok := os.LookupEnv("HTTP_ADDR")
	if !ok {
		addr = "127.0.0.1:8080"
	}

	slog.Info("starting HTTP server", "addr", addr)

	s := http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(calendarHandler),
	}
	s.ListenAndServe()
}

const cookieURL = "https://recyclingservices.bromley.gov.uk/waste/6053242"
const icsURL = "https://recyclingservices.bromley.gov.uk/waste/6053242/calendar.ics"

var proxyTransport = http.DefaultTransport

func calendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pReq, err := http.NewRequest("GET", cookieURL, nil)
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

	pReq, err = http.NewRequest("GET", icsURL, nil)
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

	slog.Info("sending calendar request", "request", pReq)

	pResp, err = proxyTransport.RoundTrip(pReq)
	if err != nil {
		http.Error(w, "error sending proxy request", http.StatusInternalServerError)
		return
	}
	defer pResp.Body.Close()

	for name, values := range pResp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(pResp.StatusCode)
	io.Copy(w, pResp.Body)
}
