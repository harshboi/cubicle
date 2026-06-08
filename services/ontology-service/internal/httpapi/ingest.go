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

// BeginIngestRunInput is the Huma request envelope for starting an ingestion run.
type BeginIngestRunInput struct {
	Body domain.IngestRunStart // Body carries the run identity and source metadata.
}

// IngestRunOutput is the Huma response envelope for ingestion run state.
type IngestRunOutput struct {
	Body domain.IngestRun // Body is the persisted ingestion run.
}

// IngestRunPathInput is the Huma path envelope for endpoints scoped to one run.
type IngestRunPathInput struct {
	RunID string `path:"run_id"` // RunID is the path-escaped ingest run key.
}

// WriteSnapshotInput is the Huma request envelope for recording a source snapshot.
type WriteSnapshotInput struct {
	RunID string                  `path:"run_id"` // RunID is the ingest run key that owns the snapshot.
	Body  SourceSnapshotWriteBody // Body carries snapshot metadata and body references.
}

// SourceSnapshotOutput is the Huma response envelope for recorded source snapshots.
type SourceSnapshotOutput struct {
	Body domain.SourceSnapshot // Body is the persisted source snapshot metadata.
}

// SourceSnapshotWriteBody is the JSON request shape for recording source snapshot metadata.
type SourceSnapshotWriteBody struct {
	SnapshotKey      string    `json:"snapshot_key"`                 // SnapshotKey is the stable source snapshot identity.
	SourceObjectType string    `json:"source_object_type,omitempty"` // SourceObjectType is the source-native payload type.
	SourceObjectID   string    `json:"source_object_id,omitempty"`   // SourceObjectID is the source-native object identifier.
	BodySHA256       string    `json:"body_sha256"`                  // BodySHA256 is the content hash for the raw snapshot body.
	BodyRef          string    `json:"body_ref"`                     // BodyRef is the content-addressed body location.
	SourceURL        string    `json:"source_url,omitempty"`         // SourceURL is the human source URL used as evidence provenance.
	FetchedAt        time.Time `json:"fetched_at,omitempty"`         // FetchedAt records when the source payload was observed.
	HeadersJSON      string    `json:"headers_json,omitempty"`       // HeadersJSON stores selected response headers when useful for replay.
}

// WriteIngestBatchInput is the Huma request envelope for mapped ontology facts.
type WriteIngestBatchInput struct {
	RunID string          `path:"run_id"` // RunID is the ingest run key that owns the mapped batch.
	Body  IngestBatchBody // Body carries mapped ontology objects, associations, evidence, and events.
}

// IngestBatchResultOutput is the Huma response envelope for mapped-batch write counts.
type IngestBatchResultOutput struct {
	Body domain.IngestBatchResult // Body summarizes how many mapped facts were upserted.
}

// IngestBatchBody is the JSON request shape for mapped ontology facts.
type IngestBatchBody struct {
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

// CompleteIngestRunInput is the Huma request envelope for completing an ingestion run.
type CompleteIngestRunInput struct {
	RunID string                `path:"run_id"` // RunID is the ingest run key to complete.
	Body  CompleteIngestRunBody // Body carries the terminal status and optional error metadata.
}

// CompleteIngestRunBody is the JSON request shape for terminal run status.
type CompleteIngestRunBody struct {
	Status       domain.IngestRunStatus `json:"status"`                  // Status is the terminal ingest run state.
	CompletedAt  time.Time              `json:"completed_at,omitempty"`  // CompletedAt records when the run reached its terminal state.
	ErrorCode    domain.IngestErrorCode `json:"error_code,omitempty"`    // ErrorCode classifies failed imports.
	ErrorMessage string                 `json:"error_message,omitempty"` // ErrorMessage gives a human-readable failure description.
}

// registerIngest registers the ingestion write API on the Huma router.
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
			"source_object_type": "custom_ticket",
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
			"objects": []map[string]any{
				{"object_type": "workstream", "key": "workstream:custom-project", "title": "Custom Project"},
				{"object_type": "ticket", "key": "ticket:TICKET-1", "title": "Example ticket", "snapshot_key": "snapshot:custom:ticket:1"},
			},
			"evidence": []map[string]any{
				{"evidence_key": "evidence:custom:TICKET-1", "snapshot_key": "snapshot:custom:ticket:1", "text_hash": "sha256:evidence-text"},
			},
			"associations": []map[string]any{{
				"from":             map[string]any{"object_type": "workstream", "key": "workstream:custom-project"},
				"to":               map[string]any{"object_type": "ticket", "key": "ticket:TICKET-1"},
				"association_type": "contains",
				"metadata":         map[string]any{"evidence_key": "evidence:custom:TICKET-1", "snapshot_key": "snapshot:custom:ticket:1"},
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

// toDomain converts snapshot request JSON into the graphstore write contract.
func (b SourceSnapshotWriteBody) toDomain(runID string, run domain.IngestRun) domain.SourceSnapshotWrite {
	return domain.SourceSnapshotWrite{
		RunKey:           runID,
		Source:           run.Source,
		SourceInstance:   run.SourceInstance,
		SnapshotKey:      b.SnapshotKey,
		SourceObjectType: b.SourceObjectType,
		SourceObjectID:   b.SourceObjectID,
		BodySHA256:       b.BodySHA256,
		BodyRef:          b.BodyRef,
		SourceURL:        b.SourceURL,
		FetchedAt:        b.FetchedAt,
		HeadersJSON:      b.HeadersJSON,
	}
}

// toDomain converts mapped-batch request JSON into the graphstore write contract.
func (b IngestBatchBody) toDomain(runID string) domain.IngestBatch {
	return domain.IngestBatch{
		RunKey:        runID,
		Slice:         b.Slice,
		MapperVersion: b.MapperVersion,
		SnapshotKeys:  b.SnapshotKeys,
		ObservedAt:    b.ObservedAt,
		Objects:       b.Objects,
		Associations:  b.Associations,
		Evidence:      b.Evidence,
		Events:        b.Events,
		Checkpoint:    b.Checkpoint,
	}
}

// toDomain converts run-completion request JSON into the graphstore write contract.
func (b CompleteIngestRunBody) toDomain(runID string) domain.IngestRunComplete {
	return domain.IngestRunComplete{
		RunKey:       runID,
		Status:       b.Status,
		CompletedAt:  b.CompletedAt,
		ErrorCode:    b.ErrorCode,
		ErrorMessage: b.ErrorMessage,
	}
}

// defaultBatchIdentity inherits source identity from the run when a client omits repeated batch fields.
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

// ingestHTTPError maps graphstore ingestion errors onto stable HTTP status codes.
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
		errors.Is(err, graphstore.ErrMissingObject):
		return huma.Error400BadRequest(string(domain.IngestErrorValidationFailed)+": "+err.Error(), err)
	default:
		return huma.Error500InternalServerError("ingest request failed")
	}
}

// jsonRequestExample builds compact OpenAPI examples for Huma operations.
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
