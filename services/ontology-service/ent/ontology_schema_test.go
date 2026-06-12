package ent_test

import (
	"testing"

	ontologyschema "cubicle/services/ontology-service/ent/schema"

	coreent "entgo.io/ent"
)

// TestPersonServingGraphAvoidsDirectHighCardinalityEdges proves person pages
// must cross bounded serving parents before loading work items.
func TestPersonServingGraphAvoidsDirectHighCardinalityEdges(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.Person{}.Edges(), []string{"work_areas", "identities"})
}

// assertSchemaEdges fails if a handwritten schema exposes unexpected edge names.
func assertSchemaEdges(t *testing.T, edges []coreent.Edge, names []string) {
	t.Helper()
	if len(edges) != len(names) {
		t.Fatalf("schema edge count = %d, want %#v", len(edges), names)
	}
	for i, edge := range edges {
		descriptor := edge.Descriptor()
		if descriptor.Name != names[i] {
			t.Fatalf("schema edge %d = %q, want %q", i, descriptor.Name, names[i])
		}
		if descriptor.Through != nil {
			t.Fatalf("schema edge %q must not be a direct Through edge", descriptor.Name)
		}
	}
}
