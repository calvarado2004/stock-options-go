package data

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetURLWithRetrySucceedsAfter429(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		current := atomic.AddInt32(&calls, 1)
		if current == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})}
	p := NewAlphaVantageProvider("")
	p.Client = client
	p.MinRequestDelay = 0
	p.InitialBackoff = 1 * time.Millisecond
	p.MaxHTTPRetries = 3

	body, err := p.getURLWithRetry("yahoo", "https://example.com/test")
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestGetURLWithRetryDoesNotRetryOn400(t *testing.T) {
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}
	p := NewAlphaVantageProvider("")
	p.Client = client
	p.MinRequestDelay = 0
	p.InitialBackoff = 1 * time.Millisecond
	p.MaxHTTPRetries = 4

	_, err := p.getURLWithRetry("yahoo", "https://example.com/test")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call for non-retryable status, got %d", calls)
	}
}
