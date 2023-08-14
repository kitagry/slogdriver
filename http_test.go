package slogdriver_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/kitagry/slogdriver"
)

func TestMakeHTTPAttr(t *testing.T) {
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 100,
		Body:          io.NopCloser(nil),
	}

	got := slogdriver.MakeHTTPAttr(req, resp)
	expected := slog.Any(slogdriver.HTTPKey, slogdriver.HTTPPayload{
		RequestMethod: "GET",
		RequestURL:    "https://example.com",
		Status:        http.StatusOK,
		ResponseSize:  "100",
		Protocol:      "HTTP/1.1",
	})

	if !got.Equal(expected) {
		t.Errorf("MakeHTTPAttr expected %+v, got %+v", expected, got)
	}
}
