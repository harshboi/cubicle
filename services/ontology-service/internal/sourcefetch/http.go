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

// maxErrorBodyBytes bounds error response snippets saved in fetch errors.
const maxErrorBodyBytes int64 = 4096

var (
	// ErrInvalidRequest indicates a source fetch request is missing required identity fields.
	ErrInvalidRequest = errors.New("invalid source fetch request")

	// ErrBudgetExceeded indicates fetch limits blocked another request or response body read.
	ErrBudgetExceeded = errors.New("source fetch budget exceeded")

	// ErrRateLimited indicates the upstream source returned a rate-limit response.
	ErrRateLimited = errors.New("source rate limited")
)

// Budget limits how much source fetching one run can perform.
type Budget struct {
	MaxRequests int   // MaxRequests caps successful HTTP attempts; zero means unlimited.
	MaxBytes    int64 // MaxBytes caps response bytes read across the fetcher; zero means unlimited.
}

// Usage tracks how much of a fetch budget has been consumed.
type Usage struct {
	Requests int   // Requests counts HTTP requests attempted through this fetcher.
	Bytes    int64 // Bytes counts response bytes read through this fetcher.
}

// Request describes one source snapshot fetch with enough metadata to build a SnapshotRecord.
type Request struct {
	Method           string      // Method is the HTTP method, defaulting to GET when empty.
	URL              string      // URL is the machine endpoint to fetch from.
	Source           string      // Source is the source system name, such as jira or github.
	SourceInstance   string      // SourceInstance is the project, repo, or workspace inside the source.
	SnapshotKey      string      // SnapshotKey is the stable local identity for the fetched payload.
	SourceObjectType string      // SourceObjectType is the source-native object type for the fetched payload.
	SourceObjectID   string      // SourceObjectID is the source-native object identifier.
	SourceURL        string      // SourceURL is the human URL used later as evidence provenance.
	Headers          http.Header // Headers are optional request headers, such as API version or auth headers.
}

// FetchedSnapshot is one HTTP response converted into an ingest pipeline snapshot record.
type FetchedSnapshot struct {
	Record         ingestpipeline.SnapshotRecord // Record is the snapshot payload and source identity used by mappers.
	ResponseStatus int                           // ResponseStatus is the upstream HTTP status code.
	ResponseHeader http.Header                   // ResponseHeader is a copy of upstream response headers.
}

// BudgetError describes which fetch budget was exceeded.
type BudgetError struct {
	Reason string // Reason explains the specific budget guard that blocked the operation.
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

// RateLimitError carries upstream retry timing when a source throttles fetches.
type RateLimitError struct {
	StatusCode    int           // StatusCode is the upstream HTTP status code.
	RetryAfter    time.Duration // RetryAfter is the parsed backoff duration.
	NextAllowedAt time.Time     // NextAllowedAt is the local wall-clock time when another attempt may be allowed.
	Body          string        // Body is a bounded error snippet for diagnostics.
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s: status=%d retry_after=%s", ErrRateLimited, e.StatusCode, e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// HTTPFetcher fetches source payloads while enforcing simple local request and byte budgets.
type HTTPFetcher struct {
	client *http.Client     // client performs outbound HTTP requests.
	budget Budget           // budget limits requests and bytes for this fetcher.
	usage  Usage            // usage accumulates consumed request and byte counts.
	now    func() time.Time // now supplies time for tests and Retry-After parsing.
}

// NewHTTPFetcher creates a budgeted HTTP source fetcher.
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

// SetNow overrides the fetcher's clock for deterministic tests.
func (f *HTTPFetcher) SetNow(now func() time.Time) {
	if now == nil {
		f.now = time.Now
		return
	}
	f.now = now
}

// Usage returns the current fetch budget usage counters.
func (f *HTTPFetcher) Usage() Usage {
	return f.usage
}

// Fetch executes one source request and converts the response body into a snapshot record.
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
			SourceObjectType: request.SourceObjectType,
			SourceObjectID:   request.SourceObjectID,
			SourceURL:        request.SourceURL,
			BodySHA256:       "sha256:" + hex.EncodeToString(hash[:]),
			Body:             body,
		},
		ResponseStatus: resp.StatusCode,
		ResponseHeader: resp.Header.Clone(),
	}, nil
}

// readBody reads a response body while charging bytes against the fetch budget.
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

// readErrorSnippet reads a bounded response body for diagnostics without exhausting the full budget.
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

// Validate checks that a source request has the identity fields needed for replayable snapshots.
func (r Request) Validate() error {
	if r.URL == "" || r.Source == "" || r.SourceInstance == "" || r.SnapshotKey == "" || r.SourceObjectType == "" {
		return fmt.Errorf("%w: url, source, source_instance, snapshot_key, and source_object_type are required", ErrInvalidRequest)
	}
	return nil
}

// isRateLimited detects common GitHub/Jira-style HTTP rate limit responses.
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

// parseRetryAfter converts Retry-After seconds or HTTP dates into a local backoff duration.
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
