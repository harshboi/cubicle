package ingestclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/domain"
)

// Client implements graphstore.IngestWriter by calling ontology-service ingestion endpoints.
type Client struct {
	baseURL    string       // baseURL is the ontology-service base URL without a trailing slash.
	httpClient *http.Client // httpClient performs outbound ingestion API calls.
}

// HTTPError reports a non-2xx ingestion API response.
type HTTPError struct {
	StatusCode int    // StatusCode is the HTTP response status code.
	Body       string // Body is the response body returned by ontology-service.
}

// Error formats the non-2xx ingestion API response.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("ingest HTTP %d: %s", e.StatusCode, e.Body)
}

// New creates an ingestion API client for one ontology-service base URL.
func New(baseURL string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// BeginIngestRun starts a source ingestion run through the HTTP API.
func (c Client) BeginIngestRun(ctx context.Context, start domain.IngestRunStart) (domain.IngestRun, error) {
	var out domain.IngestRun
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs", start, &out)
	return out, err
}

// WriteSnapshot records one raw source snapshot reference through the HTTP API.
func (c Client) WriteSnapshot(ctx context.Context, write domain.SourceSnapshotWrite) (domain.SourceSnapshot, error) {
	body := snapshotWriteBody{
		SnapshotKey:      write.SnapshotKey,
		SourceObjectType: write.SourceObjectType,
		SourceObjectID:   write.SourceObjectID,
		BodySHA256:       write.BodySHA256,
		BodyRef:          write.BodyRef,
		SourceURL:        write.SourceURL,
		FetchedAt:        write.FetchedAt,
		HeadersJSON:      write.HeadersJSON,
	}
	var out domain.SourceSnapshot
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs/"+url.PathEscape(write.RunKey)+"/snapshots", body, &out)
	return out, err
}

// WriteMappedBatch writes mapped ontology facts for an ingestion run through the HTTP API.
func (c Client) WriteMappedBatch(ctx context.Context, batch domain.IngestBatch) (domain.IngestBatchResult, error) {
	body := ingestBatchBody{
		Slice:         batch.Slice,
		MapperVersion: batch.MapperVersion,
		SnapshotKeys:  batch.SnapshotKeys,
		ObservedAt:    batch.ObservedAt,
		Objects:       batch.Objects,
		Associations:  batch.Associations,
		Evidence:      batch.Evidence,
		Events:        batch.Events,
		Checkpoint:    batch.Checkpoint,
	}
	var out domain.IngestBatchResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs/"+url.PathEscape(batch.RunKey)+"/batches", body, &out)
	return out, err
}

// CompleteIngestRun marks an ingestion run finished through the HTTP API.
func (c Client) CompleteIngestRun(ctx context.Context, complete domain.IngestRunComplete) (domain.IngestRun, error) {
	body := completeRunBody{
		Status:       complete.Status,
		CompletedAt:  complete.CompletedAt,
		ErrorCode:    complete.ErrorCode,
		ErrorMessage: complete.ErrorMessage,
	}
	var out domain.IngestRun
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs/"+url.PathEscape(complete.RunKey)+"/complete", body, &out)
	return out, err
}

// GetIngestRun fetches the current run state through the HTTP API.
func (c Client) GetIngestRun(ctx context.Context, runKey string) (domain.IngestRun, error) {
	var out domain.IngestRun
	err := c.doJSON(ctx, http.MethodGet, "/v1/ingest/runs/"+url.PathEscape(runKey), nil, &out)
	return out, err
}

// ListSourceStatus fetches source status rows through the HTTP API.
func (c Client) ListSourceStatus(ctx context.Context) ([]domain.SourceStatus, error) {
	var out []domain.SourceStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/sources", nil, &out)
	return out, err
}

// doJSON sends one JSON request and decodes the JSON response body when requested.
func (c Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

// snapshotWriteBody is the JSON shape accepted by the snapshot ingestion endpoint.
type snapshotWriteBody struct {
	SnapshotKey      string    `json:"snapshot_key"`                 // SnapshotKey is the stable source snapshot identity.
	SourceObjectType string    `json:"source_object_type,omitempty"` // SourceObjectType is the source-native payload type.
	SourceObjectID   string    `json:"source_object_id,omitempty"`   // SourceObjectID is the source-native object identifier.
	BodySHA256       string    `json:"body_sha256"`                  // BodySHA256 is the content hash for the raw snapshot body.
	BodyRef          string    `json:"body_ref"`                     // BodyRef is the content-addressed body location in the snapshot store.
	SourceURL        string    `json:"source_url,omitempty"`         // SourceURL is the human source URL used as evidence provenance.
	FetchedAt        time.Time `json:"fetched_at,omitempty"`         // FetchedAt records when the source payload was observed.
	HeadersJSON      string    `json:"headers_json,omitempty"`       // HeadersJSON stores selected response headers when useful for replay.
}

// ingestBatchBody is the JSON shape accepted by the mapped-batch ingestion endpoint.
type ingestBatchBody struct {
	Slice         string                        `json:"slice,omitempty"`          // Slice is the product/workstream partition for the mapped facts.
	MapperVersion string                        `json:"mapper_version,omitempty"` // MapperVersion records the mapper code version that produced the facts.
	SnapshotKeys  []string                      `json:"snapshot_keys,omitempty"`  // SnapshotKeys are the raw snapshots that support this batch.
	ObservedAt    time.Time                     `json:"observed_at,omitempty"`    // ObservedAt is the logical observation timestamp for mapped facts.
	Objects       []domain.Object               `json:"objects,omitempty"`        // Objects are ontology objects to upsert.
	Associations  []domain.Association          `json:"associations,omitempty"`   // Associations are ontology edges to upsert.
	Evidence      []domain.Evidence             `json:"evidence,omitempty"`       // Evidence records explain why facts were emitted.
	Events        []domain.SourceEvent          `json:"events,omitempty"`         // Events record source-level observation history.
	Checkpoint    *domain.SourceCheckpointWrite `json:"checkpoint,omitempty"`     // Checkpoint records source progress for resumable imports.
}

// completeRunBody is the JSON shape accepted by the run-completion endpoint.
type completeRunBody struct {
	Status       domain.IngestRunStatus `json:"status"`                  // Status is the terminal ingest run state.
	CompletedAt  time.Time              `json:"completed_at,omitempty"`  // CompletedAt records when the run reached its terminal state.
	ErrorCode    domain.IngestErrorCode `json:"error_code,omitempty"`    // ErrorCode classifies failed imports.
	ErrorMessage string                 `json:"error_message,omitempty"` // ErrorMessage gives a human-readable failure description.
}
