package ontology

import "testing"

func TestBuiltinPaneDefinitionsAreClosedAndConsistent(t *testing.T) {
	seen := make(map[PaneKind]bool)
	for _, definition := range BuiltinPaneDefinitions() {
		if definition.PaneKind == "" || definition.SurfaceKind == "" || definition.TargetKind == "" || definition.RelationKind == "" {
			t.Fatalf("pane definition has empty required field: %#v", definition)
		}
		if seen[definition.PaneKind] {
			t.Fatalf("duplicate pane kind %q", definition.PaneKind)
		}
		seen[definition.PaneKind] = true
		if err := ValidatePaneTargetKind(definition.PaneKind, definition.TargetKind); err != nil {
			t.Fatalf("definition should validate: %v", err)
		}
		if err := ValidatePanePlacement(definition.PaneKind, definition.SurfaceKind, definition.TargetKind); err != nil {
			t.Fatalf("definition placement should validate: %v", err)
		}
		if err := ValidatePaneLink(definition.PaneKind, definition.TargetKind, definition.TargetKind, definition.RelationKind); err != nil {
			t.Fatalf("definition link should validate: %v", err)
		}
	}
	if len(seen) != len(PaneKindStrings()) {
		t.Fatalf("pane kind string list does not match definitions: %d vs %d", len(PaneKindStrings()), len(seen))
	}
}

func TestValidatePaneTargetKindRejectsMismatchedTarget(t *testing.T) {
	if err := ValidatePaneTargetKind(PaneDocumentsCommentedOn, TargetDocument); err != nil {
		t.Fatalf("expected document pane to accept document target: %v", err)
	}
	if err := ValidatePaneTargetKind(PaneDocumentsCommentedOn, TargetTicket); err == nil {
		t.Fatal("expected document pane to reject ticket target")
	}
}

func TestValidatePanePlacementRejectsWrongSurface(t *testing.T) {
	err := ValidatePanePlacement(PanePullRequestsReviewed, SurfaceDocuments, TargetPullRequest)
	if err == nil {
		t.Fatal("expected code pane to reject document surface")
	}
}

func TestValidatePaneLinkRejectsWrongRelation(t *testing.T) {
	err := ValidatePaneLink(PaneDocumentsCreated, TargetDocument, TargetDocument, RelationEdited)
	if err == nil {
		t.Fatal("expected documents_created pane to reject edited relation")
	}
}

func TestValidatePaneLinkRejectsWrongTargetTable(t *testing.T) {
	err := ValidatePaneLink(PaneTicketsOwned, TargetTicket, TargetDocument, RelationOwned)
	if err == nil {
		t.Fatal("expected ticket pane to reject document link table")
	}
}

func TestRelationKindStringsForTarget(t *testing.T) {
	documentRelations := RelationKindStringsForTarget(TargetDocument)
	if len(documentRelations) != 3 {
		t.Fatalf("expected document relations, got %#v", documentRelations)
	}
	ticketRelations := RelationKindStringsForTarget(TargetTicket)
	if len(ticketRelations) != 3 {
		t.Fatalf("expected ticket relations, got %#v", ticketRelations)
	}
}
