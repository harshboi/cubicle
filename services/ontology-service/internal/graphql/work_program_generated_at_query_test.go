package graphql

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/entstore"

	_ "github.com/mattn/go-sqlite3"
)

func TestWorkProgramGeneratedAtPredicateMatchesPythonUTCOffset(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ontology.db")
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 22, 7, 44, 28, 600243000, time.UTC)
	rowKey := "work-program-evidence-need:python-offset"
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey(rowKey).
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("source_authentication").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("review_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Authenticate GitHub replay before allowing absence claims.").
		SetNextExecutionStep("Refresh PR details with authenticated GitHub access.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-22T07:44:28.600243+00:00|source_coverage:auth").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}

	rawDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer rawDB.Close()
	pythonTimestamp := generatedAt.UTC().Format("2006-01-02T15:04:05.999999999") + "+00:00"
	if _, err := rawDB.ExecContext(ctx, "update work_program_evidence_needs set generated_at = ? where key = ?", pythonTimestamp, rowKey); err != nil {
		t.Fatalf("rewrite generated_at: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	gateKey := "source_coverage"
	rows, total, err := resolver.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  &source,
		workstreamKey: &workstream,
		gateKey:       &gateKey,
		generatedAt:   &generatedAt,
	}, 10)
	if err != nil {
		t.Fatalf("query evidence needs: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("evidence needs = total:%d rows:%#v, want Python +00:00 timestamp row", total, rows)
	}
}
