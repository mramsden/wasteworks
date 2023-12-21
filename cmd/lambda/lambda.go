package main

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"unicode/utf8"

	"bitsden.com/wasteworks/internal/wasteworks"
	"github.com/aws/aws-lambda-go/lambda"
)

type handlerResponse struct {
	StatusCode int `json:"statusCode"`
	Headers http.Header
	Body string `json:"body"`
	IsBase64Encoded bool `json:"isBase64Encoded,omitempty"`
}

func handler(ctx context.Context) (handlerResponse, error) {
	headers := http.Header{
		"Cache-Control": {"max-age=0"},
	}
	resp, err := wasteworks.FetchCalendarWithContext(ctx)
	if err != nil {
		headers.Add("Content-Type", "text/plain")
		
		return handlerResponse{
			StatusCode: http.StatusInternalServerError,
			Headers: headers,
			Body: http.StatusText(http.StatusInternalServerError),
		}, nil
	}
	defer resp.Body.Close()

	var output string
	isBase64 := false
	bb, err := io.ReadAll(resp.Body)
	if err != nil {
		headers.Add("Content-Type", "text/plain")

		return handlerResponse{
			StatusCode: http.StatusInternalServerError,
			Headers: headers,
			Body: http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	if utf8.Valid(bb) {
		output = string(bb)
	} else {
		output = base64.StdEncoding.EncodeToString(bb)
		isBase64 = true
	}

	// Copy headers from the calendar response to the actual request response
	for name, values := range resp.Header {
		for _, value := range values {
			headers.Add(name, value)
		}
	}

	hr := handlerResponse{
		StatusCode: http.StatusOK,
		Headers: headers,
		Body: output,
		IsBase64Encoded: isBase64,
	}

	return hr, nil
}

func main() {
	lambda.Start(handler)
}