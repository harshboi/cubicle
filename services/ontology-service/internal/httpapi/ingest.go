package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
)

type BeginIngestRunInput struct {
	Body domain.IngestRunStart
}

type IngestRunOutput struct {
	Body domain.IngestRun
}

type IngestRunPathInput struct {
	RunID string `path:"run_id"`
}

type WriteSnapshotInput struct {
	RunID string `path:"run_id"`
	Body  SourceSnapshotWriteBody
}

type SourceSnapshotOutput struct {
	Body domain.SourceSnapshot
}

type SourceSnapshotWriteBody struct {
	SnapshotKey      string    `json:"snapshot_key"`
	SourceObjectKind string    `json:"source_object_kind,omitempty"`
	SourceObjectID   string    `json:"source_object_id,omitempty"`
	BodySHA256       string    `json:"body_sha256"`
	BodyRef          string    `json:"body_ref"`
	SourceURL        string    `json:"source_url,omitempty"`
	FetchedAt        time.Time `json:"fetched_at,omitempty"`
	HeadersJSON      string    `json:"headers_json,omitempty"`
}

type WriteIngestBatchInput struct {
	RunID string `path:"run_id"`
	Body  IngestBatchBody
}

type IngestBatchResultOutput struct {
	Body domain.IngestBatchResult
}

type IngestBatchBody struct {
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

type CompleteIngestRunInput struct {
	RunID string `path:"run_id"`
	Body  CompleteIngestRunBody
}

type CompleteIngestRunBody struct {
	Status       domain.IngestRunStatus `json:"status"`
	CompletedAt  time.Time              `json:"completed_at,omitempty"`
	ErrorCode    domain.IngestErrorCode `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

func registerIngest(api huma.API, store graphstore.IngestWriter) {
	huma.Register(api, huma.Operation{
		OperationID: "begin-ingest-run",
		Method:      http.MethodPost,
		Path:        "/v1/ingest/runs",
		Summary:     "Start a source ingestion run",
		Tags:        []string{"ingest"},
		Errors:      []int{http.StatusBadRequest, http.StatusConflict},
		RequestBody: jsonRequestExample("custom project fixture run", map[string]any{
			"run_key":         "run-custom-fixture-1",
			"source":          "custom",
			"source_instance": "example/project",
			"slice":           "custom-workstream",
			"mapper_version":  "custom-fixture/v1",
		}),
	}, func(ctx context.Context, input *BeginIngestRunInput) (*IngestRunOutput, error) {
		run, err := store.BeginIngestRun(ctx, input.Body)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &IngestRunOutput{Body: run}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "write-ingest-snapshot",
		Method:      http.MethodPost,
		Path:        "/v1/ingest/runs/{run_id}/snapshots",
		Summary:     "Record a raw source snapshot reference",
		Tags:        []string{"ingest"},
		Errors:      []int{http.StatusBadRequest, http.StatusConflict},
		RequestBody: jsonRequestExample("custom ticket snapshot", map[string]any{
			"snapshot_key":       "snapshot:custom:ticket:1",
			"source_object_kind": "custom_ticket",
			"source_object_id":   "TICKET-1",
			"body_sha256":        "sha256:ticket-body",
			"body_ref":           "sha256/ticket-body.json",
			"source_url":         "https://example.test/tickets/1",
		}),
	}, func(ctx context.Context, input *WriteSnapshotInput) (*SourceSnapshotOutput, error) {
		run, err := store.GetIngestRun(ctx, input.RunID)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		write := input.Body.toDomain(input.RunID, run)
		snapshot, err := store.WriteSnapshot(ctx, write)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &SourceSnapshotOutput{Body: snapshot}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "write-ingest-batch",
		Method:      http.MethodPost,
		Path:        "/v1/ingest/runs/{run_id}/batches",
		Summary:     "Persist mapped ontology facts for an ingestion run",
		Tags:        []string{"ingest"},
		Errors:      []int{http.StatusBadRequest, http.StatusConflict},
		RequestBody: jsonRequestExample("mapped custom ticket batch", map[string]any{
			"snapshot_keys": []string{"snapshot:custom:ticket:1"},
			"nodes": []map[string]any{
				{"kind": "workstream", "key": "workstream:custom-project", "title": "Custom Project"},
				{"kind": "ticket", "key": "ticket:TICKET-1", "title": "Example ticket", "snapshot_key": "snapshot:custom:ticket:1"},
			},
			"evidence": []map[string]any{
				{"evidence_key": "evidence:custom:TICKET-1", "snapshot_key": "snapshot:custom:ticket:1", "text_hash": "sha256:evidence-text"},
			},
			"edges": []map[string]any{{
				"from":     map[string]any{"kind": "workstream", "key": "workstream:custom-project"},
				"to":       map[string]any{"kind": "ticket", "key": "ticket:TICKET-1"},
				"metadata": map[string]any{"predicate": "contains", "evidence_key": "evidence:custom:TICKET-1", "snapshot_key": "snapshot:custom:ticket:1"},
			}},
			"checkpoint": map[string]any{"checkpoint_key": "fixture-manifest", "checkpoint_value": "snapshot:custom:ticket:1"},
		}),
	}, func(ctx context.Context, input *WriteIngestBatchInput) (*IngestBatchResultOutput, error) {
		run, err := store.GetIngestRun(ctx, input.RunID)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		batch := input.Body.toDomain(input.RunID)
		defaultBatchIdentity(&batch, run)
		result, err := store.WriteMappedBatch(ctx, batch)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &IngestBatchResultOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-ingest-run",
		Method:      http.MethodPost,
		Path:        "/v1/ingest/runs/{run_id}/complete",
		Summary:     "Complete an ingestion run",
		Tags:        []string{"ingest"},
		Errors:      []int{http.StatusBadRequest, http.StatusConflict},
		RequestBody: jsonRequestExample("complete run", map[string]any{
			"status": "completed",
		}),
	}, func(ctx context.Context, input *CompleteIngestRunInput) (*IngestRunOutput, error) {
		complete := input.Body.toDomain(input.RunID)
		run, err := store.CompleteIngestRun(ctx, complete)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &IngestRunOutput{Body: run}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-ingest-run",
		Method:      http.MethodGet,
		Path:        "/v1/ingest/runs/{run_id}",
		Summary:     "Get an ingestion run",
		Tags:        []string{"ingest"},
		Errors:      []int{http.StatusBadRequest},
	}, func(ctx context.Context, input *IngestRunPathInput) (*IngestRunOutput, error) {
		run, err := store.GetIngestRun(ctx, input.RunID)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &IngestRunOutput{Body: run}, nil
	})
}

func (b SourceSnapshotWriteBody) toDomain(runID string, run domain.IngestRun) domain.SourceSnapshotWrite {
	return domain.SourceSnapshotWrite{
		RunKey:           runID,
		Source:           run.Source,
		SourceInstance:   run.SourceInstance,
		SnapshotKey:      b.SnapshotKey,
		SourceObjectKind: b.SourceObjectKind,
		SourceObjectID:   b.SourceObjectID,
		BodySHA256:       b.BodySHA256,
		BodyRef:          b.BodyRef,
		SourceURL:        b.SourceURL,
		FetchedAt:        b.FetchedAt,
		HeadersJSON:      b.HeadersJSON,
	}
}

func (b IngestBatchBody) toDomain(runID string) domain.IngestBatch {
	return domain.IngestBatch{
		RunKey:        runID,
		Slice:         b.Slice,
		MapperVersion: b.MapperVersion,
		SnapshotKeys:  b.SnapshotKeys,
		ObservedAt:    b.ObservedAt,
		Nodes:         b.Nodes,
		Edges:         b.Edges,
		Evidence:      b.Evidence,
		Events:        b.Events,
		Checkpoint:    b.Checkpoint,
	}
}

func (b CompleteIngestRunBody) toDomain(runID string) domain.IngestRunComplete {
	return domain.IngestRunComplete{
		RunKey:       runID,
		Status:       b.Status,
		CompletedAt:  b.CompletedAt,
		ErrorCode:    b.ErrorCode,
		ErrorMessage: b.ErrorMessage,
	}
}

func defaultBatchIdentity(batch *domain.IngestBatch, run domain.IngestRun) {
	if batch.Source == "" {
		batch.Source = run.Source
	}
	if batch.SourceInstance == "" {
		batch.SourceInstance = run.SourceInstance
	}
	if batch.Slice == "" {
		batch.Slice = run.Slice
	}
	if batch.MapperVersion == "" {
		batch.MapperVersion = run.MapperVersion
	}
}

func ingestHTTPError(err error) error {
	switch {
	case errors.Is(err, graphstore.ErrIngestConflict):
		return huma.Error409Conflict(string(domain.IngestErrorConflict)+": "+err.Error(), err)
	case errors.Is(err, graphstore.ErrRunNotOpen):
		return huma.Error409Conflict(string(domain.IngestErrorRunNotOpen)+": "+err.Error(), err)
	case errors.Is(err, graphstore.ErrSnapshotNotFound):
		return huma.Error400BadRequest(string(domain.IngestErrorSnapshotNotFound)+": "+err.Error(), err)
	case errors.Is(err, graphstore.ErrInvalidIngest),
		errors.Is(err, graphstore.ErrInvalidExpansion),
		errors.Is(err, graphstore.ErrMissingNode):
		return huma.Error400BadRequest(string(domain.IngestErrorValidationFailed)+": "+err.Error(), err)
	default:
		return huma.Error500InternalServerError("ingest request failed")
	}
}

func jsonRequestExample(summary string, value any) *huma.RequestBody {
	return &huma.RequestBody{
		Content: map[string]*huma.MediaType{
			"application/json": {
				Examples: map[string]*huma.Example{
					"default": {
						Summary: summary,
						Value:   value,
					},
				},
			},
		},
	}
}
