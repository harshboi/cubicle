package sourcefetch

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestFetcherRecordsSnapshotMetadata(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	fetcher := Fetcher{
		Client: NewStaticClient(http.StatusOK, map[string]string{
			"Content-Type":        "application/json",
			"X-RateLimit-Remain":  "ignore",
			"X-RateLimit-Limit":   "5000",
			"X-RateLimit-Reset":   "1781182800",
			"X-GitHub-Request-Id": "request-id",
		}, []byte(`{"ok":true}`)),
		Budget: Budget{
			MaxRequests: 2,
			MaxBytes:    1024,
		},
		Now: func() time.Time { return now },
	}

	records, usage, err := fetcher.FetchAll(context.Background(), []Request{{
		SnapshotKey:      "snapshot:github:pr:1",
		URL:              "https://api.github.test/repos/apache/flink/pulls/1",
		Headers:          map[string]string{"Authorization": "Bearer secret", "Accept": "application/json"},
		SourceKey:        "github",
		SourceInstance:   "github.com/apache/flink",
		SourceObjectType: "github_pull_request",
		SourceObjectID:   "apache/flink#1",
		SourceURL:        "https://github.com/apache/flink/pull/1",
	}})
	if err != nil {
		t.Fatalf("FetchAll returned error: %v", err)
	}
	if usage.Requests != 1 || usage.Bytes != int64(len(`{"ok":true}`)) {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record := records[0]
	if record.BodySHA256 != HashBody([]byte(`{"ok":true}`)) {
		t.Fatalf("BodySHA256 = %q", record.BodySHA256)
	}
	if record.Response.StatusCode != http.StatusOK || !record.Response.FetchedAt.Equal(now) {
		t.Fatalf("unexpected response metadata: %#v", record.Response)
	}
	if record.Request.Headers["Authorization"] != "" {
		t.Fatalf("authorization header leaked into metadata: %#v", record.Request.Headers)
	}
	if record.Request.Headers["Accept"] != "application/json" {
		t.Fatalf("expected safe request header to be retained: %#v", record.Request.Headers)
	}
	if record.Response.Headers["X-Ratelimit-Limit"] != "5000" {
		t.Fatalf("expected rate-limit response header, got %#v", record.Response.Headers)
	}
}

func TestFetcherStopsAtRequestBudget(t *testing.T) {
	fetcher := Fetcher{
		Client: NewStaticClient(http.StatusOK, nil, []byte("ok")),
		Budget: Budget{MaxRequests: 1},
	}
	requests := []Request{
		testRequest("one"),
		testRequest("two"),
	}

	records, usage, err := fetcher.FetchAll(context.Background(), requests)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("error = %v, want ErrBudgetExceeded", err)
	}
	if usage.Requests != 1 || len(records) != 1 {
		t.Fatalf("expected one completed request before budget stop, records=%d usage=%#v", len(records), usage)
	}
}

func TestFetcherPreservesRateLimitSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	fetcher := Fetcher{
		Client: NewStaticClient(http.StatusTooManyRequests, map[string]string{
			"Retry-After": "30",
		}, []byte(`{"message":"slow down"}`)),
		Now: func() time.Time { return now },
	}

	records, usage, err := fetcher.FetchAll(context.Background(), []Request{testRequest("limited")})
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].Response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", records[0].Response.StatusCode)
	}
	if string(records[0].Body) != `{"message":"slow down"}` {
		t.Fatalf("body = %q", records[0].Body)
	}
	if usage.Requests != 1 {
		t.Fatalf("usage = %#v", usage)
	}
	var rateLimit RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("error = %T %v, want RateLimitError", err, err)
	}
	if !rateLimit.RetryAfter.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("RetryAfter = %s", rateLimit.RetryAfter)
	}
}

func testRequest(key string) Request {
	return Request{
		SnapshotKey:      "snapshot:" + key,
		URL:              "https://source.test/" + key,
		SourceKey:        "jira",
		SourceInstance:   "apache-jira",
		SourceObjectType: "jira_search_page",
		SourceObjectID:   key,
	}
}
