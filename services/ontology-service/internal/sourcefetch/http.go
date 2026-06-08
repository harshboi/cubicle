package sourcefetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/ingestpipeline"
)

const maxErrorBodyBytes int64 = 4096

var (
	ErrInvalidRequest = errors.New("invalid source fetch request")
	ErrBudgetExceeded = errors.New("source fetch budget exceeded")
	ErrRateLimited    = errors.New("source rate limited")
)

type Budget struct {
	MaxRequests int
	MaxBytes    int64
}

type Usage struct {
	Requests int
	Bytes    int64
}

type Request struct {
	Method           string
	URL              string
	Source           string
	SourceInstance   string
	SnapshotKey      string
	SourceObjectKind string
	SourceObjectID   string
	SourceURL        string
	Headers          http.Header
}

type FetchedSnapshot struct {
	Record         ingestpipeline.SnapshotRecord
	ResponseStatus int
	ResponseHeader http.Header
}

type BudgetError struct {
	Reason string
}

func (e *BudgetError) Error() string {
	if e.Reason == "" {
		return ErrBudgetExceeded.Error()
	}
	return ErrBudgetExceeded.Error() + ": " + e.Reason
}

func (e *BudgetError) Unwrap() error {
	return ErrBudgetExceeded
}

type RateLimitError struct {
	StatusCode    int
	RetryAfter    time.Duration
	NextAllowedAt time.Time
	Body          string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s: status=%d retry_after=%s", ErrRateLimited, e.StatusCode, e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

type HTTPFetcher struct {
	client *http.Client
	budget Budget
	usage  Usage
	now    func() time.Time
}

func NewHTTPFetcher(client *http.Client, budget Budget) *HTTPFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPFetcher{
		client: client,
		budget: budget,
		now:    time.Now,
	}
}

func (f *HTTPFetcher) SetNow(now func() time.Time) {
	if now == nil {
		f.now = time.Now
		return
	}
	f.now = now
}

func (f *HTTPFetcher) Usage() Usage {
	return f.usage
}

func (f *HTTPFetcher) Fetch(ctx context.Context, request Request) (FetchedSnapshot, error) {
	if err := request.Validate(); err != nil {
		return FetchedSnapshot{}, err
	}
	if f.budget.MaxRequests > 0 && f.usage.Requests >= f.budget.MaxRequests {
		return FetchedSnapshot{}, &BudgetError{Reason: "max requests reached"}
	}
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, request.URL, nil)
	if err != nil {
		return FetchedSnapshot{}, err
	}
	for key, values := range request.Headers {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	resp, err := f.client.Do(httpReq)
	if err != nil {
		return FetchedSnapshot{}, err
	}
	defer resp.Body.Close()
	f.usage.Requests++

	if isRateLimited(resp.StatusCode, resp.Header) {
		now := f.now()
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), now)
		return FetchedSnapshot{}, &RateLimitError{
			StatusCode:    resp.StatusCode,
			RetryAfter:    retryAfter,
			NextAllowedAt: now.Add(retryAfter),
			Body:          f.readErrorSnippet(resp.Body),
		}
	}

	body, err := f.readBody(resp.Body)
	if err != nil {
		return FetchedSnapshot{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchedSnapshot{}, fmt.Errorf("source fetch status %d: %s", resp.StatusCode, string(body))
	}
	hash := sha256.Sum256(body)
	return FetchedSnapshot{
		Record: ingestpipeline.SnapshotRecord{
			SnapshotKey:      request.SnapshotKey,
			Source:           request.Source,
			SourceInstance:   request.SourceInstance,
			SourceObjectKind: request.SourceObjectKind,
			SourceObjectID:   request.SourceObjectID,
			SourceURL:        request.SourceURL,
			BodySHA256:       "sha256:" + hex.EncodeToString(hash[:]),
			Body:             body,
		},
		ResponseStatus: resp.StatusCode,
		ResponseHeader: resp.Header.Clone(),
	}, nil
}

func (f *HTTPFetcher) readBody(reader io.Reader) ([]byte, error) {
	if f.budget.MaxBytes <= 0 {
		body, err := io.ReadAll(reader)
		if err == nil {
			f.usage.Bytes += int64(len(body))
		}
		return body, err
	}
	remaining := f.budget.MaxBytes - f.usage.Bytes
	if remaining <= 0 {
		return nil, &BudgetError{Reason: "max bytes reached"}
	}
	limited := io.LimitReader(reader, remaining+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > remaining {
		return nil, &BudgetError{Reason: "max bytes reached"}
	}
	f.usage.Bytes += int64(len(body))
	return body, nil
}

func (f *HTTPFetcher) readErrorSnippet(reader io.Reader) string {
	limit := maxErrorBodyBytes
	if f.budget.MaxBytes > 0 {
		remaining := f.budget.MaxBytes - f.usage.Bytes
		if remaining <= 0 {
			return ""
		}
		if remaining < limit {
			limit = remaining
		}
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return ""
	}
	f.usage.Bytes += int64(len(body))
	return string(body)
}

func (r Request) Validate() error {
	if r.URL == "" || r.Source == "" || r.SourceInstance == "" || r.SnapshotKey == "" || r.SourceObjectKind == "" {
		return fmt.Errorf("%w: url, source, source_instance, snapshot_key, and source_object_kind are required", ErrInvalidRequest)
	}
	return nil
}

func isRateLimited(status int, header http.Header) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	if header.Get("Retry-After") != "" {
		return true
	}
	return strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0" ||
		strings.TrimSpace(header.Get("RateLimit-Remaining")) == "0"
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if retryAt.Before(now) {
			return 0
		}
		return retryAt.Sub(now)
	}
	return 0
}
