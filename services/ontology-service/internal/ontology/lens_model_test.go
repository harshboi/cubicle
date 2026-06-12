package ontology

import "testing"

func TestBuiltinWorkLensDefinitionsAreClosedAndConsistent(t *testing.T) {
	seen := make(map[WorkLensKind]bool)
	for _, definition := range BuiltinWorkLensDefinitions() {
		if definition.WorkLensKind == "" || definition.WorkAreaKind == "" || definition.LensTargetKind == "" || definition.WorkRelationKind == "" {
			t.Fatalf("work lens definition has empty required field: %#v", definition)
		}
		if seen[definition.WorkLensKind] {
			t.Fatalf("duplicate work lens kind %q", definition.WorkLensKind)
		}
		seen[definition.WorkLensKind] = true
		if err := ValidateWorkLensTargetKind(definition.WorkLensKind, definition.LensTargetKind); err != nil {
			t.Fatalf("definition should validate: %v", err)
		}
		if err := ValidateWorkLensPlacement(definition.WorkLensKind, definition.WorkAreaKind, definition.LensTargetKind); err != nil {
			t.Fatalf("definition placement should validate: %v", err)
		}
		if err := ValidateLensResult(definition.WorkLensKind, definition.LensTargetKind, definition.LensTargetKind, definition.WorkRelationKind); err != nil {
			t.Fatalf("definition link should validate: %v", err)
		}
	}
	if len(seen) != len(WorkLensKindStrings()) {
		t.Fatalf("work lens kind string list does not match definitions: %d vs %d", len(WorkLensKindStrings()), len(seen))
	}
}

func TestValidateWorkLensTargetKindRejectsMismatchedTarget(t *testing.T) {
	if err := ValidateWorkLensTargetKind(WorkLensDocumentsCommentedOn, LensTargetDocument); err != nil {
		t.Fatalf("expected document lens to accept document target: %v", err)
	}
	if err := ValidateWorkLensTargetKind(WorkLensDocumentsCommentedOn, LensTargetTicket); err == nil {
		t.Fatal("expected document lens to reject ticket target")
	}
}

func TestValidateWorkLensPlacementRejectsWrongWorkArea(t *testing.T) {
	err := ValidateWorkLensPlacement(WorkLensPullRequestsReviewed, WorkAreaDocuments, LensTargetPullRequest)
	if err == nil {
		t.Fatal("expected code lens to reject document work area")
	}
}

func TestValidateLensResultRejectsWrongRelation(t *testing.T) {
	err := ValidateLensResult(WorkLensDocumentsCreated, LensTargetDocument, LensTargetDocument, WorkRelationEdited)
	if err == nil {
		t.Fatal("expected documents_created lens to reject edited relation")
	}
}

func TestValidateLensResultRejectsWrongTargetTable(t *testing.T) {
	err := ValidateLensResult(WorkLensTicketsOwned, LensTargetTicket, LensTargetDocument, WorkRelationOwned)
	if err == nil {
		t.Fatal("expected ticket lens to reject document result table")
	}
}

func TestWorkRelationKindStringsForTarget(t *testing.T) {
	documentRelations := WorkRelationKindStringsForTarget(LensTargetDocument)
	if len(documentRelations) != 3 {
		t.Fatalf("expected document relations, got %#v", documentRelations)
	}
	ticketRelations := WorkRelationKindStringsForTarget(LensTargetTicket)
	if len(ticketRelations) != 3 {
		t.Fatalf("expected ticket relations, got %#v", ticketRelations)
	}
}
