package wasteworks

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"slices"
	"time"
)

type retryableTransport struct {
	transport     http.RoundTripper
	maxRetryDelay time.Duration
	logger *slog.Logger
}

func newRetryableTransport() (retryableTransport) {
	return retryableTransport{
		transport: http.DefaultTransport,
		maxRetryDelay: 5 * time.Second,
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

func (t *retryableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	start := time.Now()
	resp, err := t.transport.RoundTrip(req)
	retries := 1

	for shouldRetry(err, resp) {
		select {
		case <-time.After(t.backoff(retries)):
			drainBody(resp)
			if req.Body != nil {
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
			t.logger.Debug("retrying failed request",
				"url", req.URL.String(),
				"retries", retries,
				"duration", time.Since(start))
			resp, err = t.transport.RoundTrip(req)
			retries++
		case <-req.Context().Done():
			t.logger.Debug("request timed out",
				"url", req.URL.String(),
				"retries", retries,
				"duration", time.Since(start))
			return nil, errors.New("request timed out")
		}
	}

	t.logger.Debug("request successful",
		"url", req.URL.String(),
		"retries", retries,
		"duration", time.Since(start))

	return resp, err
}

func (t *retryableTransport) backoff(retries int) time.Duration {
	delay := time.Duration(math.Pow(2, float64(retries))) * time.Second
	if delay < t.maxRetryDelay {
		return delay
	}
	return t.maxRetryDelay
}

var retryStatusCodes = []int{http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusServiceUnavailable}

func shouldRetry(err error, resp *http.Response) bool {
	return err != nil || slices.Contains(retryStatusCodes, resp.StatusCode)
}

func drainBody(resp *http.Response) {
	if resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
