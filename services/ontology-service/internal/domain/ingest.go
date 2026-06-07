package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidIngestRun      = errors.New("invalid ingest run")
	ErrInvalidSnapshot       = errors.New("invalid source snapshot")
	ErrInvalidIngestBatch    = errors.New("invalid ingest batch")
	ErrInvalidIngestComplete = errors.New("invalid ingest completion")
)

type IngestRunStatus string

const (
	IngestRunOpen      IngestRunStatus = "open"
	IngestRunCompleted IngestRunStatus = "completed"
	IngestRunFailed    IngestRunStatus = "failed"
)

type IngestErrorCode string

const (
	IngestErrorValidationFailed  IngestErrorCode = "validation_failed"
	IngestErrorConflict          IngestErrorCode = "conflict"
	IngestErrorRunNotOpen        IngestErrorCode = "run_not_open"
	IngestErrorSnapshotNotFound  IngestErrorCode = "snapshot_not_found"
	IngestErrorRateLimited       IngestErrorCode = "rate_limited"
	IngestErrorSourceUnavailable IngestErrorCode = "source_unavailable"
)

type SourceStatusValue string

const (
	SourceStatusHealthy SourceStatusValue = "healthy"
	SourceStatusRunning SourceStatusValue = "running"
	SourceStatusBlocked SourceStatusValue = "blocked"
	SourceStatusFailed  SourceStatusValue = "failed"
)

// IngestRunStart names one logical crawler attempt.
//
// RunKey is caller-owned so fixture replay can be deterministic. The service
// will still expose a database row internally, but crawler code should never
// need to coordinate around Ent IDs.
type IngestRunStart struct {
	RunKey         string    `json:"run_key"`
	Source         string    `json:"source"`
	SourceInstance string    `json:"source_instance"`
	Slice          string    `json:"slice,omitempty"`
	MapperVersion  string    `json:"mapper_version,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
}

func (s IngestRunStart) Validate() error {
	if s.RunKey == "" || s.Source == "" || s.SourceInstance == "" {
		return fmt.Errorf("%w: run_key, source, and source_instance are required", ErrInvalidIngestRun)
	}
	return nil
}

type IngestRun struct {
	RunKey         string          `json:"run_key"`
	Source         string          `json:"source"`
	SourceInstance string          `json:"source_instance"`
	Slice          string          `json:"slice,omitempty"`
	MapperVersion  string          `json:"mapper_version,omitempty"`
	Status         IngestRunStatus `json:"status"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    time.Time       `json:"completed_at,omitempty"`
	ErrorCode      IngestErrorCode `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
}

// SourceSnapshotWrite records a raw source object that the crawler already
// wrote to content-addressed storage. The service stores only the hash/ref
// metadata, leaving body persistence and replay mechanics with the crawler.
type SourceSnapshotWrite struct {
	RunKey           string    `json:"run_key"`
	Source           string    `json:"source"`
	SourceInstance   string    `json:"source_instance"`
	SnapshotKey      string    `json:"snapshot_key"`
	SourceObjectType string    `json:"source_object_type"`
	SourceObjectID   string    `json:"source_object_id"`
	BodySHA256       string    `json:"body_sha256"`
	BodyRef          string    `json:"body_ref"`
	SourceURL        string    `json:"source_url,omitempty"`
	FetchedAt        time.Time `json:"fetched_at,omitempty"`
	HeadersJSON      string    `json:"headers_json,omitempty"`
}

func (w SourceSnapshotWrite) Validate() error {
	if w.RunKey == "" || w.Source == "" || w.SourceInstance == "" || w.SnapshotKey == "" || w.BodySHA256 == "" || w.BodyRef == "" {
		return fmt.Errorf("%w: run, source, snapshot_key, body_sha256, and body_ref are required", ErrInvalidSnapshot)
	}
	return nil
}

type SourceSnapshot struct {
	SourceSnapshotWrite
	CreatedAt time.Time `json:"created_at"`
}

type SourceEvent struct {
	EventKey         string    `json:"event_key"`
	RunKey           string    `json:"run_key,omitempty"`
	Source           string    `json:"source,omitempty"`
	SourceInstance   string    `json:"source_instance,omitempty"`
	SnapshotKey      string    `json:"snapshot_key,omitempty"`
	SourceObjectType string    `json:"source_object_type,omitempty"`
	SourceObjectID   string    `json:"source_object_id,omitempty"`
	EventType        string    `json:"event_type"`
	ObservedAt       time.Time `json:"observed_at,omitempty"`
	PayloadJSON      string    `json:"payload_json,omitempty"`
}

type Evidence struct {
	EvidenceKey    string    `json:"evidence_key"`
	RunKey         string    `json:"run_key,omitempty"`
	Source         string    `json:"source,omitempty"`
	SourceInstance string    `json:"source_instance,omitempty"`
	SnapshotKey    string    `json:"snapshot_key,omitempty"`
	SourceURL      string    `json:"source_url,omitempty"`
	TextHash       string    `json:"text_hash"`
	Summary        string    `json:"summary,omitempty"`
	QuotedText     string    `json:"quoted_text,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
}

type SourceCheckpointWrite struct {
	Source          string    `json:"source,omitempty"`
	SourceInstance  string    `json:"source_instance,omitempty"`
	Slice           string    `json:"slice,omitempty"`
	CheckpointKey   string    `json:"checkpoint_key,omitempty"`
	CheckpointValue string    `json:"checkpoint_value,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// IngestBatch is the source-neutral shape crawlers send after mapping raw
// snapshots into ontology facts. It must not contain Jira-, GitHub-, or
// Flink-specific behavior; those decisions stay in mapper code.
type IngestBatch struct {
	RunKey         string                 `json:"run_key"`
	Source         string                 `json:"source"`
	SourceInstance string                 `json:"source_instance"`
	Slice          string                 `json:"slice,omitempty"`
	MapperVersion  string                 `json:"mapper_version,omitempty"`
	SnapshotKeys   []string               `json:"snapshot_keys,omitempty"`
	ObservedAt     time.Time              `json:"observed_at,omitempty"`
	Objects        []Object               `json:"objects,omitempty"`
	Associations   []Association          `json:"associations,omitempty"`
	Evidence       []Evidence             `json:"evidence,omitempty"`
	Events         []SourceEvent          `json:"events,omitempty"`
	Checkpoint     *SourceCheckpointWrite `json:"checkpoint,omitempty"`
}

type IngestBatchResult struct {
	RunKey               string `json:"run_key"`
	ObjectsUpserted      int    `json:"objects_upserted"`
	AssociationsUpserted int    `json:"associations_upserted"`
	EvidenceUpserted     int    `json:"evidence_upserted"`
	EventsUpserted       int    `json:"events_upserted"`
	CheckpointUpdated    bool   `json:"checkpoint_updated"`
}

type IngestRunComplete struct {
	RunKey       string          `json:"run_key"`
	Status       IngestRunStatus `json:"status"`
	CompletedAt  time.Time       `json:"completed_at,omitempty"`
	ErrorCode    IngestErrorCode `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

func (c IngestRunComplete) Validate() error {
	if c.RunKey == "" {
		return fmt.Errorf("%w: run_key is required", ErrInvalidIngestComplete)
	}
	if c.Status != IngestRunCompleted && c.Status != IngestRunFailed {
		return fmt.Errorf("%w: status must be completed or failed", ErrInvalidIngestComplete)
	}
	return nil
}

type SourceStatus struct {
	Source               string             `json:"source"`
	SourceInstance       string             `json:"source_instance"`
	Slice                string             `json:"slice,omitempty"`
	Status               SourceStatusValue  `json:"status"`
	LastSuccessfulRunKey string             `json:"last_successful_run_key,omitempty"`
	LastAttemptedRunKey  string             `json:"last_attempted_run_key,omitempty"`
	LastErrorKey         string             `json:"last_error_key,omitempty"`
	NextAllowedAt        time.Time          `json:"next_allowed_at,omitempty"`
	CountsByObjectType   map[ObjectType]int `json:"counts_by_object_type,omitempty"`
}

func (b IngestBatch) WithDefaults() IngestBatch {
	if b.ObservedAt.IsZero() {
		b.ObservedAt = time.Now().UTC()
	}
	for i := range b.Objects {
		object := &b.Objects[i]
		if object.Source == "" {
			object.Source = b.Source
		}
		if object.SourceInstance == "" {
			object.SourceInstance = b.SourceInstance
		}
		if object.MapperVersion == "" {
			object.MapperVersion = b.MapperVersion
		}
		if object.Visibility == "" {
			object.Visibility = VisibilityPublic
		}
		if object.FreshnessState == "" {
			object.FreshnessState = FreshnessFresh
		}
		if object.ObservedAt.IsZero() {
			object.ObservedAt = b.ObservedAt
		}
	}
	for i := range b.Associations {
		metadata := &b.Associations[i].Metadata
		if metadata.Source == "" {
			metadata.Source = b.Source
		}
		if metadata.SourceInstance == "" {
			metadata.SourceInstance = b.SourceInstance
		}
		if metadata.MapperVersion == "" {
			metadata.MapperVersion = b.MapperVersion
		}
		if metadata.Visibility == "" {
			metadata.Visibility = VisibilityPublic
		}
		if metadata.FreshnessState == "" {
			metadata.FreshnessState = FreshnessFresh
		}
		if metadata.Confidence == 0 {
			metadata.Confidence = 1
		}
		if metadata.ObservedAt.IsZero() {
			metadata.ObservedAt = b.ObservedAt
		}
	}
	return b
}

func (b IngestBatch) Validate() error {
	if b.RunKey == "" || b.Source == "" || b.SourceInstance == "" {
		return fmt.Errorf("%w: run_key, source, and source_instance are required", ErrInvalidIngestBatch)
	}
	for _, object := range b.Objects {
		if object.ObjectType == "" || object.Key == "" {
			return fmt.Errorf("%w: every object needs object_type and key", ErrInvalidIngestBatch)
		}
	}
	for _, association := range b.Associations {
		if association.From.ObjectType == "" || association.From.Key == "" || association.To.ObjectType == "" || association.To.Key == "" {
			return fmt.Errorf("%w: every association needs from and to refs", ErrInvalidIngestBatch)
		}
		if association.AssociationType == "" {
			return fmt.Errorf("%w: every association needs an association_type", ErrInvalidIngestBatch)
		}
		if association.Metadata.EvidenceKey == "" {
			return fmt.Errorf("%w: every association needs an evidence_key", ErrInvalidIngestBatch)
		}
	}
	for _, evidence := range b.Evidence {
		if evidence.EvidenceKey == "" || evidence.TextHash == "" {
			return fmt.Errorf("%w: every evidence row needs evidence_key and text_hash", ErrInvalidIngestBatch)
		}
	}
	for _, event := range b.Events {
		if event.EventKey == "" || event.EventType == "" {
			return fmt.Errorf("%w: every source event needs event_key and event_type", ErrInvalidIngestBatch)
		}
	}
	return nil
}
