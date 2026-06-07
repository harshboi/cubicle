package domain

import (
	"errors"
	"testing"
	"time"
)

func TestIngestBatchDefaultsSourceFactMetadata(t *testing.T) {
	observedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	batch := IngestBatch{
		RunKey:         "run:flink:fixture:1",
		Source:         "jira",
		SourceInstance: "apache-jira",
		Slice:          "flink-autoscaler",
		MapperVersion:  "flink-fixture/v1",
		ObservedAt:     observedAt,
		Objects: []Object{{
			ObjectType: "ticket",
			Key:        "ticket:FLINK-39743",
			Title:      "Autoscaler bug",
		}},
		Associations: []Association{{
			From:            ObjectRef{ObjectType: "workstream", Key: "workstream:flink-autoscaler"},
			To:              ObjectRef{ObjectType: "ticket", Key: "ticket:FLINK-39743"},
			AssociationType: "contains",
			Metadata: AssociationMetadata{
				EvidenceKey: "evidence:jira:FLINK-39743",
			},
		}},
	}

	defaulted := batch.WithDefaults()

	object := defaulted.Objects[0]
	if object.Source != "jira" || object.SourceInstance != "apache-jira" {
		t.Fatalf("object source metadata = %q/%q", object.Source, object.SourceInstance)
	}
	if object.Visibility != VisibilityPublic {
		t.Fatalf("object visibility = %q", object.Visibility)
	}
	if object.FreshnessState != FreshnessFresh {
		t.Fatalf("object freshness = %q", object.FreshnessState)
	}
	if !object.ObservedAt.Equal(observedAt) {
		t.Fatalf("object observed_at = %s", object.ObservedAt)
	}

	metadata := defaulted.Associations[0].Metadata
	if metadata.Source != "jira" || metadata.SourceInstance != "apache-jira" {
		t.Fatalf("association source metadata = %q/%q", metadata.Source, metadata.SourceInstance)
	}
	if metadata.MapperVersion != "flink-fixture/v1" {
		t.Fatalf("association mapper_version = %q", metadata.MapperVersion)
	}
	if metadata.Visibility != VisibilityPublic {
		t.Fatalf("association visibility = %q", metadata.Visibility)
	}
	if metadata.FreshnessState != FreshnessFresh {
		t.Fatalf("association freshness = %q", metadata.FreshnessState)
	}
	if metadata.Confidence != 1 {
		t.Fatalf("association confidence = %v", metadata.Confidence)
	}
	if !metadata.ObservedAt.Equal(observedAt) {
		t.Fatalf("association observed_at = %s", metadata.ObservedAt)
	}
}

func TestIngestBatchValidateRejectsMissingRequiredFields(t *testing.T) {
	batch := IngestBatch{
		RunKey:         "run:flink:fixture:1",
		Source:         "jira",
		SourceInstance: "apache-jira",
		Objects: []Object{{
			ObjectType: "ticket",
			Key:        "ticket:FLINK-39743",
		}},
		Associations: []Association{{
			From:            ObjectRef{ObjectType: "workstream", Key: "workstream:flink-autoscaler"},
			To:              ObjectRef{ObjectType: "ticket", Key: "ticket:FLINK-39743"},
			AssociationType: "contains",
			Metadata:        AssociationMetadata{},
		}},
	}

	if err := batch.Validate(); !errors.Is(err, ErrInvalidIngestBatch) {
		t.Fatalf("expected invalid ingest batch error, got %v", err)
	}
}

func TestSourceStatusJSONFieldsStayStable(t *testing.T) {
	status := SourceStatus{
		Source:               "jira",
		SourceInstance:       "apache-jira",
		Slice:                "flink-autoscaler",
		Status:               SourceStatusHealthy,
		LastSuccessfulRunKey: "run:flink:fixture:1",
		LastAttemptedRunKey:  "run:flink:fixture:1",
		LastErrorKey:         "",
		NextAllowedAt:        time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC),
		CountsByObjectType: map[ObjectType]int{
			"ticket": 1,
		},
	}

	if status.Source != "jira" || status.CountsByObjectType["ticket"] != 1 {
		t.Fatalf("unexpected source status: %#v", status)
	}
}
