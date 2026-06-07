package fixtures

import (
	"context"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
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

	nodes := []domain.Node{
		{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindBlocker, Key: "blocker:missing-review", Title: "Missing review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindActionCandidate, Key: "action:request-review", Title: "Request review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, node := range nodes {
		if err := store.UpsertNode(ctx, node); err != nil {
			return err
		}
	}

	edges := []domain.Edge{
		fixtureEdge("workstream:flink-autoscaler", domain.KindWorkstream, domain.PredicateContains, "ticket:FLINK-39743", domain.KindTicket, "evidence:jira-component", observedAt),
		fixtureEdge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateImplementedBy, "pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, "evidence:jira-remote-link", observedAt),
		fixtureEdge("pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, domain.PredicateChangesFile, "file:JobVertexScaler.java", domain.KindCodeFile, "evidence:github-files", observedAt),
		fixtureEdge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateBlockedBy, "blocker:missing-review", domain.KindBlocker, "evidence:review-gap", observedAt),
		fixtureEdge("blocker:missing-review", domain.KindBlocker, domain.PredicateNeedsAction, "action:request-review", domain.KindActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertEdge(ctx, edge); err != nil {
			return err
		}
	}
	return nil
}

func fixtureEdge(fromKey string, fromKind domain.Kind, predicate domain.Predicate, toKey string, toKind domain.Kind, evidenceKey string, observedAt time.Time) domain.Edge {
	return domain.Edge{
		From: domain.NodeRef{Kind: fromKind, Key: fromKey},
		To:   domain.NodeRef{Kind: toKind, Key: toKey},
		Metadata: domain.EdgeMetadata{
			Predicate:      predicate,
			EvidenceKey:    evidenceKey,
			Source:         "fixture",
			Confidence:     1,
			Visibility:     "public",
			FreshnessState: "fresh",
			ObservedAt:     observedAt,
		},
	}
}
