package fixtures

import (
	"context"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

// NewFlinkAutoscalerStore returns a tiny but realistic workstream graph.
//
// Fixtures are deliberately code, not JSON, in this early slice. Keeping them
// typed makes refactors cheap while the graph contract is still moving. Once
// ingestion exists, similar data should move into snapshots and replay tests.
func NewFlinkAutoscalerStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	if err := SeedFlinkAutoscaler(ctx, store); err != nil {
		panic(err)
	}
	return store
}

// SeedFlinkAutoscaler writes a tiny but realistic workstream graph.
//
// The same fixture can now feed MemoryStore tests, Ent-backed server startup,
// and future crawler replay tests. Treat it as a typed seed dataset: useful for
// validating graph shape, not a substitute for ingestion coverage.
func SeedFlinkAutoscaler(ctx context.Context, store graphstore.Writer) error {
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectBlocker, Key: "blocker:missing-review", Title: "Missing review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectActionCandidate, Key: "action:request-review", Title: "Request review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, node := range nodes {
		if err := store.UpsertObject(ctx, node); err != nil {
			return err
		}
	}

	edges := []domain.Association{
		fixtureEdge("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		fixtureEdge("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		fixtureEdge("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
		fixtureEdge("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocBlockedBy, "blocker:missing-review", ontology.ObjectBlocker, "evidence:review-gap", observedAt),
		fixtureEdge("blocker:missing-review", ontology.ObjectBlocker, ontology.AssocNeedsAction, "action:request-review", ontology.ObjectActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertAssociation(ctx, edge); err != nil {
			return err
		}
	}
	return nil
}

func fixtureEdge(fromKey string, fromKind domain.ObjectType, predicate domain.AssociationType, toKey string, toKind domain.ObjectType, evidenceKey string, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            domain.ObjectRef{ObjectType: fromKind, Key: fromKey},
		To:              domain.ObjectRef{ObjectType: toKind, Key: toKey},
		AssociationType: predicate,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    evidenceKey,
			Source:         "fixture",
			Confidence:     1,
			Visibility:     "public",
			FreshnessState: "fresh",
			ObservedAt:     observedAt,
		},
	}
}
