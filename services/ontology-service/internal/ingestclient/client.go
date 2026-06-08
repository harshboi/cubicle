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

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("ingest HTTP %d: %s", e.StatusCode, e.Body)
}

func New(baseURL string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c Client) BeginIngestRun(ctx context.Context, start domain.IngestRunStart) (domain.IngestRun, error) {
	var out domain.IngestRun
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs", start, &out)
	return out, err
}

func (c Client) WriteSnapshot(ctx context.Context, write domain.SourceSnapshotWrite) (domain.SourceSnapshot, error) {
	body := snapshotWriteBody{
		SnapshotKey:      write.SnapshotKey,
		SourceObjectKind: write.SourceObjectKind,
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

func (c Client) WriteMappedBatch(ctx context.Context, batch domain.IngestBatch) (domain.IngestBatchResult, error) {
	body := ingestBatchBody{
		Slice:         batch.Slice,
		MapperVersion: batch.MapperVersion,
		SnapshotKeys:  batch.SnapshotKeys,
		ObservedAt:    batch.ObservedAt,
		Nodes:         batch.Nodes,
		Edges:         batch.Edges,
		Evidence:      batch.Evidence,
		Events:        batch.Events,
		Checkpoint:    batch.Checkpoint,
	}
	var out domain.IngestBatchResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/ingest/runs/"+url.PathEscape(batch.RunKey)+"/batches", body, &out)
	return out, err
}

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

func (c Client) GetIngestRun(ctx context.Context, runKey string) (domain.IngestRun, error) {
	var out domain.IngestRun
	err := c.doJSON(ctx, http.MethodGet, "/v1/ingest/runs/"+url.PathEscape(runKey), nil, &out)
	return out, err
}

func (c Client) ListSourceStatus(ctx context.Context) ([]domain.SourceStatus, error) {
	var out []domain.SourceStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/sources", nil, &out)
	return out, err
}

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

type snapshotWriteBody struct {
	SnapshotKey      string    `json:"snapshot_key"`
	SourceObjectKind string    `json:"source_object_kind,omitempty"`
	SourceObjectID   string    `json:"source_object_id,omitempty"`
	BodySHA256       string    `json:"body_sha256"`
	BodyRef          string    `json:"body_ref"`
	SourceURL        string    `json:"source_url,omitempty"`
	FetchedAt        time.Time `json:"fetched_at,omitempty"`
	HeadersJSON      string    `json:"headers_json,omitempty"`
}

type ingestBatchBody struct {
	Slice         string                        `json:"slice,omitempty"`
	MapperVersion string                        `json:"mapper_version,omitempty"`
	SnapshotKeys  []string                      `json:"snapshot_keys,omitempty"`
	ObservedAt    time.Time                     `json:"observed_at,omitempty"`
	Nodes         []domain.Node                 `json:"nodes,omitempty"`
	Edges         []domain.Edge                 `json:"edges,omitempty"`
	Evidence      []domain.Evidence             `json:"evidence,omitempty"`
	Events        []domain.SourceEvent          `json:"events,omitempty"`
	Checkpoint    *domain.SourceCheckpointWrite `json:"checkpoint,omitempty"`
}

type completeRunBody struct {
	Status       domain.IngestRunStatus `json:"status"`
	CompletedAt  time.Time              `json:"completed_at,omitempty"`
	ErrorCode    domain.IngestErrorCode `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}
