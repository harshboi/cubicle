package sampledata

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestFakeFlinkAutoscalerMemoryStoreExpandsKnownWorkstream(t *testing.T) {
	store := NewFakeFlinkAutoscalerMemoryStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          2,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand fake sample graph: %v", err)
	}
	if len(graph.Objects) < 4 {
		t.Fatalf("expected connected fake sample graph, got %d objects: %#v", len(graph.Objects), graph.Objects)
	}
	if len(graph.Associations) < 3 {
		t.Fatalf("expected evidence-backed fake sample associations, got %d associations: %#v", len(graph.Associations), graph.Associations)
	}
	for _, association := range graph.Associations {
		if association.Metadata.EvidenceKey == "" {
			t.Fatalf("association %s has empty evidence key", association.Key)
		}
	}
}

func TestGenericDocumentMessageTicketMemoryStoreExpandsWithoutWorkProgram(t *testing.T) {
	store := NewGenericDocumentMessageTicketMemoryStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectDocument, Key: "doc:architecture-note"},
		Depth:          2,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand generic sample graph: %v", err)
	}
	assertSampleObject(t, graph.Objects, "message:standup-1")
	assertSampleObject(t, graph.Objects, "ticket:SUP-101")
	assertSampleAssociation(t, graph.Associations, "mentions", 1)
	assertSampleAssociation(t, graph.Associations, "possible_followup_for", 0.4)
	for _, object := range graph.Objects {
		if object.ObjectType == ontology.ObjectWorkstream || object.Key == "workstream:flink-autoscaler" {
			t.Fatalf("generic sample graph leaked workstream object: %#v", object)
		}
	}
}

func TestCustomerIncidentRunbookMemoryStoreUsesOpenGraphTypes(t *testing.T) {
	store := NewCustomerIncidentRunbookMemoryStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: domain.ObjectType("customer_account"),
			Key:        "customer-account:acme",
		},
		AssociationTypes: []domain.AssociationType{
			domain.AssociationType("reported_incident"),
			domain.AssociationType("has_update"),
			domain.AssociationType("has_runbook"),
		},
		Depth:          2,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand customer incident graph: %v", err)
	}
	assertSampleObject(t, graph.Objects, "incident:payments-latency")
	assertSampleObject(t, graph.Objects, "slack-message:payments-update-1")
	assertSampleObject(t, graph.Objects, "runbook:payments-latency")
	assertSampleAssociation(t, graph.Associations, "reported_incident", 1)
	assertSampleAssociation(t, graph.Associations, "has_update", 1)
	assertSampleAssociation(t, graph.Associations, "has_runbook", 1)
	for _, forbidden := range []string{"incident:finance-export", "runbook:finance-export", "slack-channel:customer-incidents"} {
		for _, object := range graph.Objects {
			if object.Key == forbidden {
				t.Fatalf("filtered incident graph leaked %s: %#v", forbidden, graph.Objects)
			}
		}
	}
}

func assertSampleObject(t *testing.T, objects []domain.Object, key string) {
	t.Helper()
	for _, object := range objects {
		if object.Key == key {
			return
		}
	}
	t.Fatalf("sample graph missing object %q: %#v", key, objects)
}

func assertSampleAssociation(t *testing.T, associations []domain.Association, associationType domain.AssociationType, confidence float64) {
	t.Helper()
	for _, association := range associations {
		if association.AssociationType == associationType {
			if association.Metadata.EvidenceKey == "" {
				t.Fatalf("association %s has no evidence key: %#v", associationType, association)
			}
			if association.Metadata.Confidence != confidence {
				t.Fatalf("association %s confidence = %f, want %f", associationType, association.Metadata.Confidence, confidence)
			}
			return
		}
	}
	t.Fatalf("sample graph missing association %q: %#v", associationType, associations)
}
