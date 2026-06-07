package sampledata

import (
	"context"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

// NewFakeFlinkAutoscalerMemoryStore returns a memory store seeded with sample data.
//
// This helper is test/dev scaffolding only. It gives tests a stable graph shape
// without implying that Flink data is part of the generic service contract.
func NewFakeFlinkAutoscalerMemoryStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	if err := SeedFakeFlinkAutoscalerWorkstream(ctx, store); err != nil {
		panic(err)
	}
	return store
}

// SeedFakeFlinkAutoscalerWorkstream writes a tiny fake workstream graph.
//
// The data is deliberately small and deterministic. Use it for tests, examples,
// and explicitly enabled local demo seeding; do not treat it as authoritative
// source data for the generic ontology service.
func SeedFakeFlinkAutoscalerWorkstream(ctx context.Context, store graphstore.Writer) error {
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	objects := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "fake_sampledata", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "fake_sampledata", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectBlocker, Key: "blocker:missing-review", Title: "Missing review", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectActionCandidate, Key: "action:request-review", Title: "Request review", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			return err
		}
	}

	associations := []domain.Association{
		fakeAssociation("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		fakeAssociation("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		fakeAssociation("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
		fakeAssociation("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocBlockedBy, "blocker:missing-review", ontology.ObjectBlocker, "evidence:review-gap", observedAt),
		fakeAssociation("blocker:missing-review", ontology.ObjectBlocker, ontology.AssocNeedsAction, "action:request-review", ontology.ObjectActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			return err
		}
	}
	return nil
}

// fakeAssociation builds one evidence-backed association in the sample graph.
func fakeAssociation(fromKey string, fromKind domain.ObjectType, associationType domain.AssociationType, toKey string, toKind domain.ObjectType, evidenceKey string, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            domain.ObjectRef{ObjectType: fromKind, Key: fromKey},
		To:              domain.ObjectRef{ObjectType: toKind, Key: toKey},
		AssociationType: associationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    evidenceKey,
			Source:         "fake_sampledata",
			Confidence:     1,
			Visibility:     "public",
			FreshnessState: "fresh",
			ObservedAt:     observedAt,
		},
	}
}
