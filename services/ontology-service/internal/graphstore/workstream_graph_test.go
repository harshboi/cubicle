package graphstore

import (
	"context"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestWorkstreamNeighborhoodReturnsConnectedGraphWithEvidence(t *testing.T) {
	// ctx is the request context passed through the store API. The in-memory
	// implementation does not use it yet, but Ent-backed stores will.
	ctx := context.Background()

	// store is the concrete graph implementation under test for this scaffold
	// PR. Later PRs add Ent while keeping the same public behavior.
	store := NewMemoryStore()

	// observedAt is fixed so association metadata is deterministic in tests.
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	// objects is the minimal fake Flink workstream sample. It models a connected
	// slice from workstream to ticket, PR, changed file, blocker, and action.
	objects := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler"},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation"},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate"},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java"},
		{ObjectType: ontology.ObjectBlocker, Key: "blocker:missing-review", Title: "Missing review"},
		{ObjectType: ontology.ObjectActionCandidate, Key: "action:request-review", Title: "Request review"},
	}

	// object is each fake sample entity being inserted before associations. The
	// store requires endpoints to exist before relationships are written.
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	// associations is the evidence-backed fake sample relationship set that makes the
	// workstream traversable.
	associations := []domain.Association{
		association("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		association("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		association("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
		association("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocBlockedBy, "blocker:missing-review", ontology.ObjectBlocker, "evidence:review-gap", observedAt),
		association("blocker:missing-review", ontology.ObjectBlocker, ontology.AssocNeedsAction, "action:request-review", ontology.ObjectActionCandidate, "evidence:action-rule", observedAt),
	}

	// association is each relationship inserted after both endpoint objects are
	// present in the store.
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	// graph is the bounded neighborhood returned to HTTP and future Swift query
	// surfaces.
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

	assertObject(t, graph, "ticket:FLINK-39743")
	assertObject(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertObject(t, graph, "file:JobVertexScaler.java")
	assertObject(t, graph, "blocker:missing-review")
	assertObject(t, graph, "action:request-review")
	assertAssociation(t, graph, "workstream:flink-autoscaler", ontology.AssocContains, "ticket:FLINK-39743")
	assertAssociation(t, graph, "ticket:FLINK-39743", ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertAssociation(t, graph, "pr:apache/flink-kubernetes-operator#1127", ontology.AssocChangesFile, "file:JobVertexScaler.java")
	assertAssociation(t, graph, "ticket:FLINK-39743", ontology.AssocBlockedBy, "blocker:missing-review")
	assertAssociation(t, graph, "blocker:missing-review", ontology.AssocNeedsAction, "action:request-review")
}

func TestWorkstreamNeighborhoodRequiresBounds(t *testing.T) {
	// store is intentionally empty because this test checks request validation
	// before traversal work starts.
	store := NewMemoryStore()

	// err should be non-nil because LimitPerObject is required for every graph
	// expansion request.
	_, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start: domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth: 2,
	})
	if err == nil {
		t.Fatal("expected missing limit to fail")
	}
}

func TestMemoryStoreUsesOpenAssociationVocabulary(t *testing.T) {
	// store proves the graph layer accepts custom terms even when they are not in
	// the built-in ontology registry.
	store := NewMemoryStore()

	// ctx is passed through the same write/read API shape Ent will implement.
	ctx := context.Background()

	// objects uses custom object types to prove ObjectType is an open string.
	objects := []domain.Object{
		{ObjectType: "custom_system", Key: "custom:system:atlas", Title: "Atlas"},
		{ObjectType: "custom_artifact", Key: "custom:artifact:readiness", Title: "Readiness note"},
	}

	// object is each custom object inserted before the custom association.
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	// base is the first version of a custom association. Rewriting it below
	// proves upsert semantics replace the logical relationship.
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

	// base.Metadata is updated in place to simulate a newer mapper observation
	// for the same logical association.
	base.Metadata.EvidenceKey = "evidence:latest"
	base.Metadata.Confidence = 0.9
	if err := store.UpsertAssociation(ctx, base); err != nil {
		t.Fatalf("upsert replacement association: %v", err)
	}

	// graph is the custom-term neighborhood used to assert open vocabulary
	// traversal and replacement behavior.
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

// association builds a fake sample relationship with evidence metadata.
//
// fromKey/fromObjectType describe the directed source object, associationType
// names the relationship, toKey/toObjectType describe the directed target
// object, and evidenceKey/observedAt fill the provenance metadata.
func association(fromKey string, fromObjectType domain.ObjectType, associationType domain.AssociationType, toKey string, toObjectType domain.ObjectType, evidenceKey string, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            domain.ObjectRef{ObjectType: fromObjectType, Key: fromKey},
		To:              domain.ObjectRef{ObjectType: toObjectType, Key: toKey},
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

// assertObject checks that a neighborhood contains an object with key.
func assertObject(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()

	// object is each returned graph object being compared against the expected
	// key.
	for _, object := range graph.Objects {
		if object.Key == key {
			return
		}
	}
	t.Fatalf("expected object %q in graph; got %#v", key, graph.Objects)
}

// assertAssociation checks that a neighborhood contains a directed relationship.
func assertAssociation(t *testing.T, graph domain.Neighborhood, fromKey string, associationType domain.AssociationType, toKey string) {
	t.Helper()

	// association is each returned graph relationship being compared against the
	// expected source, type, and target.
	for _, association := range graph.Associations {
		if association.From.Key == fromKey && association.AssociationType == associationType && association.To.Key == toKey {
			if association.Metadata.EvidenceKey == "" {
				t.Fatalf("association %s -> %s has no evidence", fromKey, toKey)
			}
			return
		}
	}
	t.Fatalf("expected association %q -[%s]-> %q in graph; got %#v", fromKey, associationType, toKey, graph.Associations)
}
