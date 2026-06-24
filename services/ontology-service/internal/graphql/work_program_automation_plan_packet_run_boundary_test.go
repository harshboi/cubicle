package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramAutomationPlanActionEvidenceNeedsUseExecutionRun(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	actionKey := "tpm-action:owner-a"
	targetKey := "apache/flink-kubernetes-operator#100"
	oldRunAt := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)

	for _, row := range []struct {
		key         string
		generatedAt time.Time
		action      string
		target      string
		rank        float64
	}{
		{key: "work-program-evidence-need:plan-action-stale", generatedAt: oldRunAt, action: actionKey, target: targetKey, rank: 100},
		{key: "work-program-evidence-need:plan-action-current", generatedAt: newRunAt, action: actionKey, target: targetKey, rank: 90},
	} {
		_, err = store.Client().WorkProgramEvidenceNeed.Create().
			SetKey(row.key).
			SetWorkstreamKey("flink-kubernetes-operator").
			SetGeneratedAt(row.generatedAt).
			SetGateKey("blocker_clearance").
			SetEvidenceKind("blocker_clearance").
			SetPriority(workprogramevidenceneed.PriorityHigh).
			SetTargetKind("pull_request").
			SetTargetKey(row.target).
			SetActionKey(row.action).
			SetActionState("open").
			SetExecutionState("action_open").
			SetCurrentCount(0).
			SetRequiredCount(1).
			SetMissingCount(1).
			SetRecommendedAction("Confirm blocker clearance for the action.").
			SetNextExecutionStep("Review current blocker evidence.").
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_program_evidence_need").
			SetExternalID(row.key).
			SetRankScore(row.rank).
			Save(ctx)
		if err != nil {
			t.Fatalf("create evidence need %s: %v", row.key, err)
		}
	}

	generatedAt := newRunAt.Format(time.RFC3339Nano)
	execution := &model.WorkProgramExecutionPacket{
		SourceInstance: stringPointer(source),
		GeneratedAt:    &generatedAt,
		WorkstreamKey:  workstream,
		Actions: []*model.WorkAction{
			{Key: actionKey, SubjectKey: targetKey},
		},
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rows, err := resolver.workProgramAutomationPlanActionEvidenceNeeds(ctx, execution)
	if err != nil {
		t.Fatalf("plan action evidence needs: %v", err)
	}
	if len(rows) != 1 || rows[0].Key != "plan-action-current" {
		t.Fatalf("action evidence needs = %#v, want only current execution run", rows)
	}
}
