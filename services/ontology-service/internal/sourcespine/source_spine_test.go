package sourcespine

import (
	"context"
	"errors"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/sourcerun"

	_ "github.com/mattn/go-sqlite3"
)

// TestValidateTargetAcceptsKnownTypedObjects proves target_kind/target_id is a
// constrained provenance pointer, not an unchecked generic graph reference.
func TestValidateTargetAcceptsKnownTypedObjects(t *testing.T) {
	ctx := context.Background()
	client := openSourceSpineTestClient(t, "source-spine-target-valid")
	defer client.Close()

	ticket := client.Ticket.Create().
		SetKey("ticket:target-valid").
		SetTitle("Target validation ticket").
		SaveX(ctx)

	if err := ValidateTarget(ctx, client, string(TargetKindTicket), ticket.ID); err != nil {
		t.Fatalf("expected ticket target to validate: %v", err)
	}
}

// TestValidateTargetRejectsUnknownAndMissingTargets proves ExternalIdentity
// target pointers cannot silently point at nonexistent or unknown objects.
func TestValidateTargetRejectsUnknownAndMissingTargets(t *testing.T) {
	ctx := context.Background()
	client := openSourceSpineTestClient(t, "source-spine-target-invalid")
	defer client.Close()

	if err := ValidateTarget(ctx, client, "made_up_kind", 1); !errors.Is(err, ErrUnknownTargetKind) {
		t.Fatalf("expected unknown kind error, got %v", err)
	}
	if err := ValidateTarget(ctx, client, string(TargetKindTicket), 404); err == nil {
		t.Fatal("expected missing typed target to fail")
	}
	if err := ValidateTarget(ctx, client, string(TargetKindTicket), 0); !errors.Is(err, ErrInvalidTargetID) {
		t.Fatalf("expected invalid target id error, got %v", err)
	}
}

// TestSourceEvidenceSpineUniquenessProvesIdentityAndAnchorDedupe proves the
// core uniqueness contracts are enforced by SQLite through Ent migrations.
func TestSourceEvidenceSpineUniquenessProvesIdentityAndAnchorDedupe(t *testing.T) {
	ctx := context.Background()
	client := openSourceSpineTestClient(t, "source-spine-uniqueness")
	defer client.Close()

	ticket := client.Ticket.Create().
		SetKey("ticket:unique").
		SetTitle("Unique source identity").
		SaveX(ctx)
	if _, err := createSourceRun(ctx, client, "run:unique", sourcerun.StatusComplete); err != nil {
		t.Fatalf("create first source run: %v", err)
	}
	if _, err := createSourceRun(ctx, client, "run:unique", sourcerun.StatusComplete); err == nil {
		t.Fatal("expected duplicate source run key to fail")
	}

	identity := createExternalIdentity(t, ctx, client, ticket.ID, "CUB-123")
	if _, err := client.ExternalIdentity.Create().
		SetTargetKind(string(TargetKindTicket)).
		SetTargetID(ticket.ID).
		SetSourceKey("jira").
		SetSourceInstance("local-dev").
		SetExternalKind("jira_issue").
		SetExternalID("CUB-123").
		Save(ctx); err == nil {
		t.Fatal("expected duplicate source identity tuple to fail")
	}

	run := client.SourceRun.Query().OnlyX(ctx)
	observation := createSourceObservation(t, ctx, client, run.ID, identity.ID, "hash:issue")
	if _, err := client.SourceObservation.Create().
		SetSourceRunID(run.ID).
		SetExternalIdentityID(identity.ID).
		SetObservedKind("jira_issue").
		SetContentHash("hash:issue-again").
		Save(ctx); err == nil {
		t.Fatal("expected duplicate run/identity observation to fail")
	}

	createEvidenceAnchor(t, ctx, client, observation.ID, "span:summary", "hash:summary")
	if _, err := client.EvidenceAnchor.Create().
		SetSourceObservationID(observation.ID).
		SetAnchorKind("ticket_description").
		SetAnchorLocator("description").
		SetSourceSpanKey("span:summary").
		SetTextHash("hash:summary").
		Save(ctx); err == nil {
		t.Fatal("expected duplicate evidence anchor tuple to fail")
	}
}

// TestCurrentEvidenceAnchorQueryRequiresPermissionFilters proves evidence
// preview reads cannot be built without explicit permission and visibility inputs.
func TestCurrentEvidenceAnchorQueryRequiresPermissionFilters(t *testing.T) {
	client := openSourceSpineTestClient(t, "source-spine-filter-required")
	defer client.Close()

	if _, err := CurrentEvidenceAnchorQuery(client, AnchorVisibilityFilter{}); !errors.Is(err, ErrMissingPermissionFilters) {
		t.Fatalf("expected missing permission filters error, got %v", err)
	}
}

// TestCurrentEvidenceAnchorQueryFiltersDeletionVisibilityAndPartialRuns proves
// the helper encodes the legal evidence text read path.
func TestCurrentEvidenceAnchorQueryFiltersDeletionVisibilityAndPartialRuns(t *testing.T) {
	ctx := context.Background()
	client := openSourceSpineTestClient(t, "source-spine-anchor-filter")
	defer client.Close()

	completeVisible := createAnchoredObservation(t, ctx, client, anchoredObservationInput{
		RunKey:              "run:complete-visible",
		RunStatus:           sourcerun.StatusComplete,
		ExternalID:          "CUB-1",
		ContentHash:         "hash:complete-visible",
		SourceSpanKey:       "span:complete-visible",
		TextHash:            "text:complete-visible",
		PermissionPolicyKey: "local_dev_open",
		VisibilityHash:      "visible",
		IsDeleted:           false,
	})
	partialVisible := createAnchoredObservation(t, ctx, client, anchoredObservationInput{
		RunKey:              "run:partial-visible",
		RunStatus:           sourcerun.StatusPartial,
		ExternalID:          "CUB-2",
		ContentHash:         "hash:partial-visible",
		SourceSpanKey:       "span:partial-visible",
		TextHash:            "text:partial-visible",
		PermissionPolicyKey: "local_dev_open",
		VisibilityHash:      "visible",
		IsDeleted:           false,
	})
	createAnchoredObservation(t, ctx, client, anchoredObservationInput{
		RunKey:              "run:deleted",
		RunStatus:           sourcerun.StatusComplete,
		ExternalID:          "CUB-3",
		ContentHash:         "hash:deleted",
		SourceSpanKey:       "span:deleted",
		TextHash:            "text:deleted",
		PermissionPolicyKey: "local_dev_open",
		VisibilityHash:      "visible",
		IsDeleted:           true,
	})
	createAnchoredObservation(t, ctx, client, anchoredObservationInput{
		RunKey:              "run:hidden",
		RunStatus:           sourcerun.StatusComplete,
		ExternalID:          "CUB-4",
		ContentHash:         "hash:hidden",
		SourceSpanKey:       "span:hidden",
		TextHash:            "text:hidden",
		PermissionPolicyKey: "local_dev_open",
		VisibilityHash:      "hidden",
		IsDeleted:           false,
	})

	query, err := CurrentEvidenceAnchorQuery(client, AnchorVisibilityFilter{
		PermissionPolicyKeys: []string{"local_dev_open"},
		VisibilityHashes:     []string{"visible"},
		IncludePartialRuns:   false,
	})
	if err != nil {
		t.Fatalf("build complete-only anchor query: %v", err)
	}
	completeOnly := query.AllX(ctx)
	if len(completeOnly) != 1 || completeOnly[0].ID != completeVisible.ID {
		t.Fatalf("expected only complete visible current anchor %d, got %#v", completeVisible.ID, completeOnly)
	}

	query, err = CurrentEvidenceAnchorQuery(client, AnchorVisibilityFilter{
		PermissionPolicyKeys: []string{"local_dev_open"},
		VisibilityHashes:     []string{"visible"},
		IncludePartialRuns:   true,
	})
	if err != nil {
		t.Fatalf("build partial-inclusive anchor query: %v", err)
	}
	withPartial := query.AllX(ctx)
	if len(withPartial) != 2 {
		t.Fatalf("expected complete plus partial anchors, got %#v", withPartial)
	}
	ids := map[int]bool{withPartial[0].ID: true, withPartial[1].ID: true}
	if !ids[completeVisible.ID] || !ids[partialVisible.ID] {
		t.Fatalf("expected anchors %d and %d, got %#v", completeVisible.ID, partialVisible.ID, withPartial)
	}
}

// TestEvidenceCanPointToEvidenceAnchor proves existing graph evidence can cite
// the new source-neutral anchor without replacing through-edge latest evidence.
func TestEvidenceCanPointToEvidenceAnchor(t *testing.T) {
	ctx := context.Background()
	client := openSourceSpineTestClient(t, "source-spine-evidence-link")
	defer client.Close()

	anchor := createAnchoredObservation(t, ctx, client, anchoredObservationInput{
		RunKey:              "run:evidence-link",
		RunStatus:           sourcerun.StatusComplete,
		ExternalID:          "CUB-5",
		ContentHash:         "hash:evidence-link",
		SourceSpanKey:       "span:evidence-link",
		TextHash:            "text:evidence-link",
		PermissionPolicyKey: "local_dev_open",
		VisibilityHash:      "visible",
		IsDeleted:           false,
	})
	evidence := client.Evidence.Create().
		SetKey("evidence:anchor-link").
		SetEvidenceAnchorID(anchor.ID).
		SaveX(ctx)

	loadedAnchor := evidence.QueryEvidenceAnchor().OnlyX(ctx)
	if loadedAnchor.ID != anchor.ID {
		t.Fatalf("expected evidence to resolve anchor %d, got %d", anchor.ID, loadedAnchor.ID)
	}
}

type anchoredObservationInput struct {
	RunKey              string
	RunStatus           sourcerun.Status
	ExternalID          string
	ContentHash         string
	SourceSpanKey       string
	TextHash            string
	PermissionPolicyKey string
	VisibilityHash      string
	IsDeleted           bool
}

func openSourceSpineTestClient(t *testing.T, name string) *genent.Client {
	t.Helper()
	return enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
}

func createAnchoredObservation(t *testing.T, ctx context.Context, client *genent.Client, input anchoredObservationInput) *genent.EvidenceAnchor {
	t.Helper()
	ticket := client.Ticket.Create().
		SetKey("ticket:" + input.ExternalID).
		SetTitle("Ticket " + input.ExternalID).
		SaveX(ctx)
	run, err := createSourceRun(ctx, client, input.RunKey, input.RunStatus)
	if err != nil {
		t.Fatalf("create source run: %v", err)
	}
	identity := createExternalIdentity(t, ctx, client, ticket.ID, input.ExternalID)
	observation := createSourceObservation(t, ctx, client, run.ID, identity.ID, input.ContentHash)
	if input.PermissionPolicyKey != "" || input.VisibilityHash != "" || input.IsDeleted {
		update := client.SourceObservation.UpdateOneID(observation.ID)
		if input.PermissionPolicyKey != "" {
			update.SetPermissionPolicyKey(input.PermissionPolicyKey)
		}
		if input.VisibilityHash != "" {
			update.SetVisibilityHash(input.VisibilityHash)
		}
		if input.IsDeleted {
			update.SetIsDeleted(true)
		}
		observation = update.SaveX(ctx)
	}
	return createEvidenceAnchor(t, ctx, client, observation.ID, input.SourceSpanKey, input.TextHash)
}

func createSourceRun(ctx context.Context, client *genent.Client, runKey string, status sourcerun.Status) (*genent.SourceRun, error) {
	return client.SourceRun.Create().
		SetRunKey(runKey).
		SetSourceKey("jira").
		SetSourceInstance("local-dev").
		SetScopeKind("project").
		SetScopeKey("CUB").
		SetStatus(status).
		SetStartedAt(time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)).
		Save(ctx)
}

func createExternalIdentity(t *testing.T, ctx context.Context, client *genent.Client, ticketID int, externalID string) *genent.ExternalIdentity {
	t.Helper()
	return client.ExternalIdentity.Create().
		SetTargetKind(string(TargetKindTicket)).
		SetTargetID(ticketID).
		SetSourceKey("jira").
		SetSourceInstance("local-dev").
		SetExternalKind("jira_issue").
		SetExternalID(externalID).
		SaveX(ctx)
}

func createSourceObservation(t *testing.T, ctx context.Context, client *genent.Client, runID int, identityID int, contentHash string) *genent.SourceObservation {
	t.Helper()
	return client.SourceObservation.Create().
		SetSourceRunID(runID).
		SetExternalIdentityID(identityID).
		SetObservedKind("jira_issue").
		SetPermissionPolicyKey("local_dev_open").
		SetVisibilityHash("visible").
		SetContentHash(contentHash).
		SaveX(ctx)
}

func createEvidenceAnchor(t *testing.T, ctx context.Context, client *genent.Client, observationID int, sourceSpanKey string, textHash string) *genent.EvidenceAnchor {
	t.Helper()
	return client.EvidenceAnchor.Create().
		SetSourceObservationID(observationID).
		SetAnchorKind("ticket_description").
		SetAnchorLocator("description").
		SetSourceSpanKey(sourceSpanKey).
		SetTextHash(textHash).
		SetTextPreview("launch is blocked until the source evidence spine lands").
		SetLexicalFingerprint("launch blocked source evidence spine").
		SaveX(ctx)
}
