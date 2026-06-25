package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkResponsibilitiesReturnsTypedPersonAndSubject(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 23, 9, 30, 0, 0, time.UTC)
	person := store.Client().Person.Create().
		SetKey("person:github:owner-one").
		SetDisplayName("Owner One").
		SetGithubLogin("owner-one").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("apache/flink-kubernetes-operator#42").
		SetRepository("apache/flink-kubernetes-operator").
		SetNumber(42).
		SetTitle("Fix autoscaler reconciliation").
		SetSourceURL("https://github.com/apache/flink-kubernetes-operator/pull/42").
		SaveX(ctx)

	_, err = store.Client().WorkResponsibility.Create().
		SetKey("work-responsibility:fixture:owner-one").
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPullRequestID(pr.ID).
		SetPartyKind(workresponsibility.PartyKindPerson).
		SetPartyKey("github:owner-one").
		SetPartySource("github.pr.author").
		SetPersonID(person.ID).
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("work_responsibility:fixture").
		SetLastActivityAt(now).
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create responsibility: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	source := "fixture-source"
	party := "github:owner-one"
	rows, err := resolver.WorkResponsibilities(ctx, &limit, nil, nil, nil, &party, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("work responsibilities: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("responsibility count = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.PersonKey == nil || *row.PersonKey != person.Key {
		t.Fatalf("person key = %v, want %s", row.PersonKey, person.Key)
	}
	if row.PersonDisplayName == nil || *row.PersonDisplayName != "Owner One" {
		t.Fatalf("person display = %v, want Owner One", row.PersonDisplayName)
	}
	if row.SubjectTitle == nil || *row.SubjectTitle != "Fix autoscaler reconciliation" {
		t.Fatalf("subject title = %v, want PR title", row.SubjectTitle)
	}
	if row.SubjectURL == nil || *row.SubjectURL != "https://github.com/apache/flink-kubernetes-operator/pull/42" {
		t.Fatalf("subject url = %v, want PR URL", row.SubjectURL)
	}
	if row.ResponsibilityState != "active" || row.BasisKind != "source_native" {
		t.Fatalf("state/basis = %s/%s, want active/source_native", row.ResponsibilityState, row.BasisKind)
	}
}

func TestWorkResponsibilitiesClampNestedActionAndItemWhenResponsibilityNeedsValidation(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 23, 10, 25, 0, 0, time.UTC)
	action, responsibility := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)
	item, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:responsibility-nested").
		SetWorkActionID(action.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(action.SubjectKey).
		SetTitle("Nested responsibility gated item").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDecisionGateReason("local product action row must still honor responsibility gate").
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("work-program-item:responsibility-nested").
		SetRankScore(100).
		SetRiskScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkResponsibility.UpdateOne(responsibility).
		SetWorkProgramItemID(item.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("link responsibility item context: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	sourceArg := source
	subjectKind := "work_action"
	subjectKey := action.Key
	rows, err := resolver.WorkResponsibilities(ctx, &limit, &subjectKind, &subjectKey, nil, nil, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("work responsibilities: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("responsibility count = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.Action == nil || row.Action.ProductActionAllowed || row.Action.ClaimUse != "responsibility_validation" {
		t.Fatalf("nested action claim = %#v, want responsibility validation gate", row.Action)
	}
	if !strings.Contains(row.Action.ClaimGateReason, "generated owner hint requires validation") {
		t.Fatalf("nested action gate reason = %#v, want responsibility validation reason", row.Action.ClaimGateReason)
	}
	if row.ProgramItem == nil || row.ProgramItem.ProductActionAllowed || row.ProgramItem.ClaimUse != "responsibility_validation" {
		t.Fatalf("nested program item claim = %#v, want responsibility validation gate", row.ProgramItem)
	}
	if row.ProgramItem.Action == nil || row.ProgramItem.Action.ProductActionAllowed {
		t.Fatalf("nested program item action claim = %#v, want clamped nested action", row.ProgramItem.Action)
	}
}
