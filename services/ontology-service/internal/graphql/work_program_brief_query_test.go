package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramBriefClampsAutomationReadinessForResponsibilityValidation(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 9, 55, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	queryWorkstream := "workstream:flink-kubernetes-operator"
	sourceArg := source
	brief, err := resolver.WorkProgramBrief(ctx, &queryWorkstream, &sourceArg)
	if err != nil {
		t.Fatalf("work program brief: %v", err)
	}

	if brief.AutomationReadiness == nil {
		t.Fatalf("brief automation readiness missing")
	}
	if brief.AutomationReadiness.AutonomousActionReady || !brief.AutomationReadiness.HumanReviewRequired {
		t.Fatalf("brief readiness flags = autonomous:%v human:%v, want false/true", brief.AutomationReadiness.AutonomousActionReady, brief.AutomationReadiness.HumanReviewRequired)
	}
	if brief.AutomationReadiness.ReadinessState != "human_review_required" {
		t.Fatalf("brief readiness state = %q, want human_review_required", brief.AutomationReadiness.ReadinessState)
	}
	assertContainsString(t, brief.AutomationReadiness.HumanRequiredAreas, "responsibility_validation")
	assertContainsString(t, brief.AutomationReadiness.RequiredEvidence, "validated_accountable_owner")
}
