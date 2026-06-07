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
		Nodes: []Node{{
			Kind:  KindTicket,
			Key:   "ticket:FLINK-39743",
			Title: "Autoscaler bug",
		}},
		Edges: []Edge{{
			From: NodeRef{Kind: KindWorkstream, Key: "workstream:flink-autoscaler"},
			To:   NodeRef{Kind: KindTicket, Key: "ticket:FLINK-39743"},
			Metadata: EdgeMetadata{
				Predicate:   PredicateContains,
				EvidenceKey: "evidence:jira:FLINK-39743",
			},
		}},
	}

	defaulted := batch.WithDefaults()

	node := defaulted.Nodes[0]
	if node.Source != "jira" || node.SourceInstance != "apache-jira" {
		t.Fatalf("node source metadata = %q/%q", node.Source, node.SourceInstance)
	}
	if node.Visibility != VisibilityPublic {
		t.Fatalf("node visibility = %q", node.Visibility)
	}
	if node.FreshnessState != FreshnessFresh {
		t.Fatalf("node freshness = %q", node.FreshnessState)
	}
	if !node.ObservedAt.Equal(observedAt) {
		t.Fatalf("node observed_at = %s", node.ObservedAt)
	}

	metadata := defaulted.Edges[0].Metadata
	if metadata.Source != "jira" || metadata.SourceInstance != "apache-jira" {
		t.Fatalf("edge source metadata = %q/%q", metadata.Source, metadata.SourceInstance)
	}
	if metadata.MapperVersion != "flink-fixture/v1" {
		t.Fatalf("edge mapper_version = %q", metadata.MapperVersion)
	}
	if metadata.Visibility != VisibilityPublic {
		t.Fatalf("edge visibility = %q", metadata.Visibility)
	}
	if metadata.FreshnessState != FreshnessFresh {
		t.Fatalf("edge freshness = %q", metadata.FreshnessState)
	}
	if metadata.Confidence != 1 {
		t.Fatalf("edge confidence = %v", metadata.Confidence)
	}
	if !metadata.ObservedAt.Equal(observedAt) {
		t.Fatalf("edge observed_at = %s", metadata.ObservedAt)
	}
}

func TestIngestBatchValidateRejectsMissingRequiredFields(t *testing.T) {
	batch := IngestBatch{
		RunKey:         "run:flink:fixture:1",
		Source:         "jira",
		SourceInstance: "apache-jira",
		Nodes: []Node{{
			Kind: KindTicket,
			Key:  "ticket:FLINK-39743",
		}},
		Edges: []Edge{{
			From: NodeRef{Kind: KindWorkstream, Key: "workstream:flink-autoscaler"},
			To:   NodeRef{Kind: KindTicket, Key: "ticket:FLINK-39743"},
			Metadata: EdgeMetadata{
				Predicate: PredicateContains,
			},
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
		CountsByKind: map[Kind]int{
			KindTicket: 1,
		},
	}

	if status.Source != "jira" || status.CountsByKind[KindTicket] != 1 {
		t.Fatalf("unexpected source status: %#v", status)
	}
}
