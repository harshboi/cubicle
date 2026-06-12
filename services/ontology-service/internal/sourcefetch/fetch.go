package sourcefetch

import (
	"bytes"
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
)

const defaultUserAgent = "cubicle-ontology-sourcefetch/0.1"

var ErrBudgetExceeded = errors.New("source fetch budget exceeded")

// Request describes one source-neutral HTTP request that should become a replayable snapshot.
type Request struct {
	SnapshotKey      string            `json:"snapshot_key"`              // SnapshotKey is the stable identity for replay and idempotency.
	Method           string            `json:"method,omitempty"`          // Method defaults to GET.
	URL              string            `json:"url"`                       // URL is the exact source endpoint to fetch.
	Body             string            `json:"body,omitempty"`            // Body is optional request payload text for POST-style source APIs.
	Headers          map[string]string `json:"headers,omitempty"`         // Headers are non-secret request headers.
	SourceKey        string            `json:"source_key"`                // SourceKey is the source family, such as jira, github, docs, or ponymail.
	SourceInstance   string            `json:"source_instance"`           // SourceInstance distinguishes a project, repo, tenant, or workspace.
	SourceObjectType string            `json:"source_object_type"`        // SourceObjectType is the source-native payload kind.
	SourceObjectID   string            `json:"source_object_id"`          // SourceObjectID is the source-native object/page identifier.
	SourceURL        string            `json:"source_url,omitempty"`      // SourceURL is the human-facing deep link when available.
	ExpectedSHA256   string            `json:"expected_sha256,omitempty"` // ExpectedSHA256 optionally pins a known body hash.
}

// SnapshotRecord is the raw fetched payload plus enough metadata to replay or normalize it later.
type SnapshotRecord struct {
	SnapshotKey      string           `json:"snapshot_key"`
	SourceKey        string           `json:"source_key"`
	SourceInstance   string           `json:"source_instance"`
	SourceObjectType string           `json:"source_object_type"`
	SourceObjectID   string           `json:"source_object_id"`
	SourceURL        string           `json:"source_url,omitempty"`
	Path             string           `json:"path,omitempty"`
	BodyRef          string           `json:"body_ref,omitempty"`
	BodySHA256       string           `json:"body_sha256"`
	Request          RequestMetadata  `json:"request"`
	Response         ResponseMetadata `json:"response"`
	Body             []byte           `json:"-"`
}

// RequestMetadata records the exact non-secret request shape used to create a snapshot.
type RequestMetadata struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	BodySHA256 string            `json:"body_sha256,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// ResponseMetadata records response status and replay-relevant headers.
type ResponseMetadata struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	FetchedAt  time.Time         `json:"fetched_at"`
}

// Budget is a hard stop for bounded live probes and source crawls.
type Budget struct {
	MaxRequests int   // MaxRequests limits request count when positive.
	MaxBytes    int64 // MaxBytes limits downloaded response bytes when positive.
}

// Usage reports consumed request and byte budget.
type Usage struct {
	Requests int   `json:"requests"`
	Bytes    int64 `json:"bytes"`
}

// HTTPClient is the narrow net/http surface used by Fetcher.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Fetcher turns planned source requests into raw snapshots without source-specific logic.
type Fetcher struct {
	Client    HTTPClient
	Budget    Budget
	UserAgent string
	Now       func() time.Time
}

// RateLimitError reports throttling without retrying inside the fetcher.
type RateLimitError struct {
	SnapshotKey   string
	StatusCode    int
	RetryAfter    time.Time
	ResetAt       time.Time
	ResponseBytes int64
}

func (e RateLimitError) Error() string {
	parts := []string{fmt.Sprintf("source rate limited for snapshot %s: status %d", e.SnapshotKey, e.StatusCode)}
	if !e.RetryAfter.IsZero() {
		parts = append(parts, "retry_after="+e.RetryAfter.Format(time.RFC3339))
	}
	if !e.ResetAt.IsZero() {
		parts = append(parts, "reset_at="+e.ResetAt.Format(time.RFC3339))
	}
	return strings.Join(parts, " ")
}

// FetchAll fetches requests in order and returns snapshots for completed requests only.
func (f Fetcher) FetchAll(ctx context.Context, requests []Request) ([]SnapshotRecord, Usage, error) {
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	now := f.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	userAgent := f.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	records := make([]SnapshotRecord, 0, len(requests))
	var usage Usage
	for _, request := range requests {
		if err := request.Validate(); err != nil {
			return records, usage, err
		}
		if f.Budget.MaxRequests > 0 && usage.Requests >= f.Budget.MaxRequests {
			return records, usage, fmt.Errorf("%w: max requests %d reached", ErrBudgetExceeded, f.Budget.MaxRequests)
		}

		httpRequest, err := request.HTTPRequest(ctx)
		if err != nil {
			return records, usage, err
		}
		if httpRequest.Header.Get("User-Agent") == "" {
			httpRequest.Header.Set("User-Agent", userAgent)
		}

		response, err := client.Do(httpRequest)
		if err != nil {
			return records, usage, fmt.Errorf("fetch snapshot %s: %w", request.SnapshotKey, err)
		}
		usage.Requests++

		body, readErr := readBudgetedBody(response.Body, f.Budget, &usage)
		closeErr := response.Body.Close()
		if readErr != nil {
			return records, usage, fmt.Errorf("read snapshot %s: %w", request.SnapshotKey, readErr)
		}
		if closeErr != nil {
			return records, usage, fmt.Errorf("close snapshot %s body: %w", request.SnapshotKey, closeErr)
		}

		record, err := snapshotFromResponse(request, response, body, now())
		if err != nil {
			return records, usage, err
		}
		records = append(records, record)
		if isRateLimited(response) {
			return records, usage, RateLimitError{
				SnapshotKey:   request.SnapshotKey,
				StatusCode:    response.StatusCode,
				RetryAfter:    retryAfterTime(response.Header, now()),
				ResetAt:       unixHeaderTime(response.Header.Get("x-ratelimit-reset")),
				ResponseBytes: int64(len(body)),
			}
		}
	}
	return records, usage, nil
}

// Validate checks the fields required for durable replay.
func (r Request) Validate() error {
	if r.SnapshotKey == "" {
		return errors.New("snapshot key is required")
	}
	if r.URL == "" {
		return fmt.Errorf("snapshot %s: url is required", r.SnapshotKey)
	}
	if r.SourceKey == "" || r.SourceInstance == "" || r.SourceObjectType == "" || r.SourceObjectID == "" {
		return fmt.Errorf("snapshot %s: source identity fields are required", r.SnapshotKey)
	}
	return nil
}

// HTTPRequest builds a net/http request from the replayable source request.
func (r Request) HTTPRequest(ctx context.Context) (*http.Request, error) {
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, strings.NewReader(r.Body))
	if err != nil {
		return nil, fmt.Errorf("build snapshot %s request: %w", r.SnapshotKey, err)
	}
	for key, value := range r.Headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

func snapshotFromResponse(request Request, response *http.Response, body []byte, fetchedAt time.Time) (SnapshotRecord, error) {
	bodySHA256 := HashBody(body)
	if request.ExpectedSHA256 != "" && request.ExpectedSHA256 != bodySHA256 {
		return SnapshotRecord{}, fmt.Errorf("snapshot %s: body hash mismatch: got %s want %s", request.SnapshotKey, bodySHA256, request.ExpectedSHA256)
	}
	return SnapshotRecord{
		SnapshotKey:      request.SnapshotKey,
		SourceKey:        request.SourceKey,
		SourceInstance:   request.SourceInstance,
		SourceObjectType: request.SourceObjectType,
		SourceObjectID:   request.SourceObjectID,
		SourceURL:        request.SourceURL,
		BodySHA256:       bodySHA256,
		Request: RequestMetadata{
			Method:     methodOrDefault(request.Method),
			URL:        request.URL,
			BodySHA256: optionalHashString(request.Body),
			Headers:    safeRequestHeaders(request.Headers),
		},
		Response: ResponseMetadata{
			StatusCode: response.StatusCode,
			Headers:    replayResponseHeaders(response.Header),
			FetchedAt:  fetchedAt,
		},
		Body: body,
	}, nil
}

func readBudgetedBody(reader io.Reader, budget Budget, usage *Usage) ([]byte, error) {
	if budget.MaxBytes <= 0 {
		body, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		usage.Bytes += int64(len(body))
		return body, nil
	}
	remaining := budget.MaxBytes - usage.Bytes
	if remaining <= 0 {
		return nil, fmt.Errorf("%w: max bytes %d reached", ErrBudgetExceeded, budget.MaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(reader, remaining+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > remaining {
		return nil, fmt.Errorf("%w: max bytes %d reached", ErrBudgetExceeded, budget.MaxBytes)
	}
	usage.Bytes += int64(len(body))
	return body, nil
}

// HashBody returns the stable sha256 digest format stored on snapshot metadata.
func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func optionalHashString(body string) string {
	if body == "" {
		return ""
	}
	return HashBody([]byte(body))
}

func methodOrDefault(method string) string {
	if method == "" {
		return http.MethodGet
	}
	return method
}

func isRateLimited(response *http.Response) bool {
	if response.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if response.StatusCode != http.StatusForbidden {
		return false
	}
	remaining := response.Header.Get("x-ratelimit-remaining")
	return remaining == "0"
}

func retryAfterTime(header http.Header, now time.Time) time.Time {
	value := header.Get("retry-after")
	if value == "" {
		return time.Time{}
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if parsed, err := http.ParseTime(value); err == nil {
		return parsed
	}
	return time.Time{}
}

func unixHeaderTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func safeRequestHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string]string)
	for key, value := range headers {
		switch strings.ToLower(key) {
		case "authorization", "cookie", "set-cookie", "x-api-key":
			continue
		default:
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func replayResponseHeaders(headers http.Header) map[string]string {
	keep := map[string]struct{}{
		"content-length":        {},
		"content-type":          {},
		"etag":                  {},
		"last-modified":         {},
		"link":                  {},
		"retry-after":           {},
		"x-github-request-id":   {},
		"x-ratelimit-limit":     {},
		"x-ratelimit-remaining": {},
		"x-ratelimit-reset":     {},
		"x-ratelimit-resource":  {},
		"x-ratelimit-used":      {},
	}
	filtered := make(map[string]string)
	for key, values := range headers {
		normalized := strings.ToLower(key)
		if _, ok := keep[normalized]; !ok {
			continue
		}
		filtered[http.CanonicalHeaderKey(key)] = strings.Join(values, ",")
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// NewStaticClient returns an HTTP client stub useful for package tests and dry-run probes.
func NewStaticClient(statusCode int, headers map[string]string, body []byte) HTTPClient {
	return staticClient{statusCode: statusCode, headers: headers, body: body}
}

type staticClient struct {
	statusCode int
	headers    map[string]string
	body       []byte
}

func (c staticClient) Do(req *http.Request) (*http.Response, error) {
	header := make(http.Header)
	for key, value := range c.headers {
		header.Set(key, value)
	}
	return &http.Response{
		StatusCode: c.statusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(c.body)),
		Request:    req,
	}, nil
}
