package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkBlockersProductActionRequiresAcceptedGoldBlockerClaim(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	observedAt := time.Date(2026, 6, 23, 11, 15, 0, 0, time.UTC)
	_, err = store.Client().WorkBlocker.Create().
		SetKey("work-blocker:weak-product-action").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey("repo/example#weak").
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTruthLabel(workblocker.TruthLabelTruePositive).
		SetActionabilityLabel(workblocker.ActionabilityLabelNeedsOwner).
		SetLabelQuality(workblocker.LabelQualityCandidate).
		SetMeasurementEligible(false).
		SetTitle("Weak blocker product action").
		SetRecommendedAction("Validate blocker labels before treating this as a product action.").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:weak-product-action").
		SetLastActivityAt(observedAt).
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create weak blocker: %v", err)
	}
	_, err = store.Client().WorkBlocker.Create().
		SetKey("work-blocker:gold-product-action").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey("repo/example#gold").
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTruthLabel(workblocker.TruthLabelTruePositive).
		SetActionabilityLabel(workblocker.ActionabilityLabelActionable).
		SetLabelQuality(workblocker.LabelQualityGold).
		SetMeasurementEligible(true).
		SetLabelSet("blocker-gold-v1").
		SetTitle("Gold blocker product action").
		SetRecommendedAction("Drive the accepted blocker owner follow-up.").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:gold-product-action").
		SetLastActivityAt(observedAt.Add(-time.Minute)).
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create gold blocker: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	sourceArg := source
	rows, err := resolver.WorkBlockers(ctx, &limit, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("work blockers: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("work blockers count = %d, want 2: %#v", len(rows), rows)
	}
	weak := rows[0]
	if weak.Key != "work-blocker:weak-product-action" {
		t.Fatalf("first blocker = %s, want weak blocker by rank", weak.Key)
	}
	if weak.ProductActionAllowed || weak.BlockerClaimAllowed || weak.ClaimUse == "blocker_claim" {
		t.Fatalf("weak blocker claim = product:%v blocker:%v use:%s, want label-gated validation", weak.ProductActionAllowed, weak.BlockerClaimAllowed, weak.ClaimUse)
	}
	if weak.ClaimGateReason != "blocker_claim_requires_gold_measurement_label" {
		t.Fatalf("weak blocker gate = %q, want gold measurement label gate", weak.ClaimGateReason)
	}
	for _, badge := range weak.Badges {
		if badge.Key == "decision:product_action" && badge.Label == "Product action" {
			t.Fatalf("weak blocker still exposed product-action badge: %#v", weak.Badges)
		}
	}

	gold := rows[1]
	if gold.Key != "work-blocker:gold-product-action" {
		t.Fatalf("second blocker = %s, want gold blocker", gold.Key)
	}
	if !gold.ProductActionAllowed || !gold.BlockerClaimAllowed || gold.ClaimUse != "blocker_claim" {
		t.Fatalf("gold blocker claim = product:%v blocker:%v use:%s, want product blocker claim", gold.ProductActionAllowed, gold.BlockerClaimAllowed, gold.ClaimUse)
	}
	if gold.ClaimGateReason != "blocker_claim_gate_passed" {
		t.Fatalf("gold blocker gate = %q, want passed", gold.ClaimGateReason)
	}
}
