package graphstore

import (
	"context"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestWorkstreamNeighborhoodReturnsConnectedGraphWithEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler"},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation"},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate"},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java"},
		{ObjectType: ontology.ObjectBlocker, Key: "blocker:missing-review", Title: "Missing review"},
		{ObjectType: ontology.ObjectActionCandidate, Key: "action:request-review", Title: "Request review"},
	}
	for _, node := range nodes {
		if err := store.UpsertObject(ctx, node); err != nil {
			t.Fatalf("upsert node %s: %v", node.Key, err)
		}
	}

	edges := []domain.Association{
		edge("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		edge("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		edge("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
		edge("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocBlockedBy, "blocker:missing-review", ontology.ObjectBlocker, "evidence:review-gap", observedAt),
		edge("blocker:missing-review", ontology.ObjectBlocker, ontology.AssocNeedsAction, "action:request-review", ontology.ObjectActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertAssociation(ctx, edge); err != nil {
			t.Fatalf("upsert edge %s: %v", edge.Key, err)
		}
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectWorkstream,
			Key:        "workstream:flink-autoscaler",
		},
		AssociationTypes: []domain.AssociationType{
			ontology.AssocContains,
			ontology.AssocImplementedBy,
			ontology.AssocChangesFile,
			ontology.AssocBlockedBy,
			ontology.AssocNeedsAction,
		},
		Depth:          4,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand workstream: %v", err)
	}

	assertNode(t, graph, "ticket:FLINK-39743")
	assertNode(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertNode(t, graph, "file:JobVertexScaler.java")
	assertNode(t, graph, "blocker:missing-review")
	assertNode(t, graph, "action:request-review")
	assertEdge(t, graph, "workstream:flink-autoscaler", ontology.AssocContains, "ticket:FLINK-39743")
	assertEdge(t, graph, "ticket:FLINK-39743", ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertEdge(t, graph, "pr:apache/flink-kubernetes-operator#1127", ontology.AssocChangesFile, "file:JobVertexScaler.java")
	assertEdge(t, graph, "ticket:FLINK-39743", ontology.AssocBlockedBy, "blocker:missing-review")
	assertEdge(t, graph, "blocker:missing-review", ontology.AssocNeedsAction, "action:request-review")
}

func TestWorkstreamNeighborhoodRequiresBounds(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start: domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth: 2,
	})
	if err == nil {
		t.Fatal("expected missing limit to fail")
	}
}

func TestMemoryStoreUsesOpenAssociationVocabulary(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	objects := []domain.Object{
		{ObjectType: "custom_system", Key: "custom:system:atlas", Title: "Atlas"},
		{ObjectType: "custom_artifact", Key: "custom:artifact:readiness", Title: "Readiness note"},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	base := domain.Association{
		From:            objects[0].Ref(),
		To:              objects[1].Ref(),
		AssociationType: "custom_depends_on",
		Metadata: domain.AssociationMetadata{
			EvidenceKey: "evidence:first",
			Confidence:  0.7,
		},
	}
	if err := store.UpsertAssociation(ctx, base); err != nil {
		t.Fatalf("upsert custom association: %v", err)
	}
	base.Metadata.EvidenceKey = "evidence:latest"
	base.Metadata.Confidence = 0.9
	if err := store.UpsertAssociation(ctx, base); err != nil {
		t.Fatalf("upsert replacement association: %v", err)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:            objects[0].Ref(),
		AssociationTypes: []domain.AssociationType{"custom_depends_on"},
		Depth:            1,
		LimitPerObject:   10,
	})
	if err != nil {
		t.Fatalf("expand custom graph: %v", err)
	}
	if len(graph.Associations) != 1 {
		t.Fatalf("expected one logical association, got %#v", graph.Associations)
	}
	if graph.Associations[0].Metadata.EvidenceKey != "evidence:latest" {
		t.Fatalf("association evidence was not replaced: %#v", graph.Associations[0])
	}
}

func edge(fromKey string, fromKind domain.ObjectType, predicate domain.AssociationType, toKey string, toKind domain.ObjectType, evidenceKey string, observedAt time.Time) domain.Association {
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

func assertNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Objects {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("expected node %q in graph; got %#v", key, graph.Objects)
}

func assertEdge(t *testing.T, graph domain.Neighborhood, fromKey string, predicate domain.AssociationType, toKey string) {
	t.Helper()
	for _, edge := range graph.Associations {
		if edge.From.Key == fromKey && edge.AssociationType == predicate && edge.To.Key == toKey {
			if edge.Metadata.EvidenceKey == "" {
				t.Fatalf("edge %s -> %s has no evidence", fromKey, toKey)
			}
			return
		}
	}
	t.Fatalf("expected edge %q -[%s]-> %q in graph; got %#v", fromKey, predicate, toKey, graph.Associations)
}
