package sourcefetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPFetcherBuildsSnapshotRecordFromResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/objects/1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("ETag", `"abc"`)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	t.Cleanup(server.Close)

	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 2, MaxBytes: 1024})
	fetched, err := fetcher.Fetch(context.Background(), Request{
		Method:           http.MethodGet,
		URL:              server.URL + "/objects/1",
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:1",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
		SourceURL:        "https://example.test/objects/1",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if fetched.Record.BodySHA256 == "" || fetched.Record.BodyRef != "" {
		t.Fatalf("unexpected body metadata before snapshot store write: %#v", fetched.Record)
	}
	if string(fetched.Record.Body) != `{"id":"1"}` {
		t.Fatalf("body = %q", string(fetched.Record.Body))
	}
	if fetched.ResponseStatus != http.StatusOK {
		t.Fatalf("status = %d", fetched.ResponseStatus)
	}
	if fetcher.Usage().Requests != 1 || fetcher.Usage().Bytes != int64(len(`{"id":"1"}`)) {
		t.Fatalf("usage = %#v", fetcher.Usage())
	}
}

func TestHTTPFetcherStopsBeforeRequestWhenBudgetExhausted(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(server.Close)

	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 1, MaxBytes: 1024})
	req := Request{
		Method:           http.MethodGet,
		URL:              server.URL,
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:1",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
	}
	if _, err := fetcher.Fetch(context.Background(), req); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	_, err := fetcher.Fetch(context.Background(), req)
	var budgetErr *BudgetError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetError, got %T: %v", err, err)
	}
	if calls != 1 {
		t.Fatalf("budget should stop second HTTP call, calls=%d", calls)
	}
}

func TestHTTPFetcherReturnsRateLimitWithoutRetrySpinning(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "2")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 2, MaxBytes: 1024})
	fetcher.SetNow(func() time.Time { return now })
	_, err := fetcher.Fetch(context.Background(), Request{
		Method:           http.MethodGet,
		URL:              server.URL,
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:rate-limited",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
	})
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateErr.NextAllowedAt != now.Add(2*time.Second) {
		t.Fatalf("next allowed = %s", rateErr.NextAllowedAt)
	}
	if calls != 1 {
		t.Fatalf("fetcher should not retry-spin, calls=%d", calls)
	}
}

func TestHTTPFetcherDetectsForbiddenRateLimitHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, "secondary rate limit", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 2, MaxBytes: 1024})
	_, err := fetcher.Fetch(context.Background(), Request{
		Method:           http.MethodGet,
		URL:              server.URL,
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:rate-limited",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
	})
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateErr.RetryAfter != 3*time.Second {
		t.Fatalf("retry after = %s", rateErr.RetryAfter)
	}
}

func TestHTTPFetcherTreatsPlainForbiddenAsSourceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 2, MaxBytes: 1024})
	_, err := fetcher.Fetch(context.Background(), Request{
		Method:           http.MethodGet,
		URL:              server.URL,
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:forbidden",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
	})
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		t.Fatalf("plain forbidden should not be rate limited: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "source fetch status 403") {
		t.Fatalf("expected source status error, got %T: %v", err, err)
	}
}

func TestHTTPFetcherReturnsRateLimitWhenErrorBodyExceedsByteBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		http.Error(w, strings.Repeat("rate limited ", 16), http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	fetcher := NewHTTPFetcher(server.Client(), Budget{MaxRequests: 2, MaxBytes: 4})
	_, err := fetcher.Fetch(context.Background(), Request{
		Method:           http.MethodGet,
		URL:              server.URL,
		Source:           "custom",
		SourceInstance:   "example/project",
		SnapshotKey:      "snapshot:custom:rate-limited",
		SourceObjectType: "custom_object",
		SourceObjectID:   "1",
	})
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
}
