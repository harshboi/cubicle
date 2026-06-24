// Association:
//
//	CLI args/env/config -> command config
//	fixture manifest -> summary/load command -> JSON output
//	serve config -> HTTP server timeouts
//
// These tests keep the command layer honest before it reaches source replay or
// the local ontology service.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

// TestParseServeConfigDefaultsToLocalhost keeps the default service bind local.
func TestParseServeConfigDefaultsToLocalhost(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.DatabasePath != filepath.Join(".data", "graph.db") {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 5*time.Second {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if !cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = false, want true")
	}
}

// TestSummarizeFlinkFixtureReportsSourceAndStatusCounts checks fixture coverage before loading.
func TestSummarizeFlinkFixtureReportsSourceAndStatusCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "github"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	okBody := []byte(`{"ok":true}`)
	limitedBody := []byte(`{"message":"limited"}`)
	if err := os.WriteFile(filepath.Join(dir, "github", "ok.json"), okBody, 0o644); err != nil {
		t.Fatalf("write ok body: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "github", "limited.json"), limitedBody, 0o644); err != nil {
		t.Fatalf("write limited body: %v", err)
	}
	manifest := strings.Join([]string{
		`{"path":"github/ok.json","source":"github","source_object_type":"github_pull_request","source_object_id":"apache/flink-kubernetes-operator#1078","url":"https://api.github.test/pulls/1078","status_code":200,"body_sha256":"` + sourcecapture.HashBody(okBody) + `","bytes":11}`,
		`{"path":"github/limited.json","source":"github","source_object_type":"github_pull_request","source_object_id":"apache/flink-kubernetes-operator#1127","url":"https://api.github.test/pulls/1127","status_code":429,"body_sha256":"` + sourcecapture.HashBody(limitedBody) + `","bytes":21}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	if err := summarizeFlinkFixture(flinkFixtureSummaryConfig{Dir: dir}, &out); err != nil {
		t.Fatalf("summarizeFlinkFixture returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{`"total": 2`, `"key": "github"`, `"count": 2`, `"key": "200"`, `"key": "429"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %s: %s", want, got)
		}
	}
}

// TestLoadFlinkFixtureReportsMaterializedCounts checks the load command's graph counters.
func TestLoadFlinkFixtureReportsMaterializedCounts(t *testing.T) {
	dir := t.TempDir()
	records := []testFixtureRecord{
		{
			Path:       "jira/issues/FLINK-2.json",
			Source:     "jira",
			ObjectType: "jira_issue",
			ObjectID:   "FLINK-2",
			Status:     200,
			Body: []byte(`{
			  "key":"FLINK-2",
			  "fields":{
			    "summary":"Tune autoscaler stabilization",
			    "description":"Stabilize scaling decisions.",
			    "status":{"name":"Closed"},
			    "priority":{"name":"Major"},
			    "updated":"2026-06-10T15:04:05.000+0000"
			  }
			}`),
		},
		{
			Path:       "jira/remote-links/FLINK-2.json",
			Source:     "jira",
			ObjectType: "jira_remote_links",
			ObjectID:   "FLINK-2",
			Status:     200,
			Body:       []byte(`[{"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/20"}}]`),
		},
		{
			Path:       "github/pr-details/apache__flink-kubernetes-operator__20/pull.json",
			Source:     "github",
			ObjectType: "github_pull_request",
			ObjectID:   "apache/flink-kubernetes-operator#20",
			Status:     200,
			Body: []byte(`{
			  "html_url":"https://github.com/apache/flink-kubernetes-operator/pull/20",
			  "title":"[FLINK-2] Tune autoscaler stabilization",
			  "state":"closed",
			  "merged_at":"2026-06-10T16:00:00Z",
			  "updated_at":"2026-06-10T16:00:00Z",
			  "number":20,
			  "base":{"repo":{"full_name":"apache/flink-kubernetes-operator"}}
			}`),
		},
	}
	for _, objectType := range []string{
		"github_pull_request_files",
		"github_issue_comments",
		"github_pull_request_review_comments",
		"github_pull_request_reviews",
		"github_pull_request_commits",
	} {
		records = append(records, testFixtureRecord{
			Path:       "github/pr-details/apache__flink-kubernetes-operator__20/" + objectType + ".json",
			Source:     "github",
			ObjectType: objectType,
			ObjectID:   "apache/flink-kubernetes-operator#20",
			Status:     200,
			Body:       []byte(`[]`),
		})
	}
	records = append(records, testFixtureRecord{
		Path:       "github/pr-details/apache__flink-kubernetes-operator__21/pull.json",
		Source:     "github",
		ObjectType: "github_pull_request",
		ObjectID:   "apache/flink-kubernetes-operator#21",
		Status:     429,
		Body:       []byte(`{"message":"rate limited"}`),
	})
	writeTestFixture(t, dir, records)

	var out bytes.Buffer
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	err := loadFlinkFixture(context.Background(), flinkFixtureLoadConfig{
		Dir:          dir,
		DatabasePath: dbPath,
		StreamKey:    "test-flink-load-command",
		RunKey:       "source-sync-run:test-flink-load-command",
	}, &out)
	if err != nil {
		t.Fatalf("loadFlinkFixture returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"records_seen": 9`,
		`"records_failed": 1`,
		`"complete_pull_request_bundles": 1`,
		`"tickets": 1`,
		`"pull_requests": 1`,
		`"ticket_pull_requests": 1`,
		`"evidence": 4`,
		`"sync_issues": 1`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("load output missing %s: %s", want, got)
		}
	}
}

func TestRegisterSourceScopeCreatesNotAttemptedState(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ontology.db")

	var out bytes.Buffer
	err := registerSourceScope(ctx, sourceScopeRegisterConfig{
		DatabasePath:     dbPath,
		SourceSystem:     "jira",
		SourceInstance:   "company-jira",
		DisplayName:      "Company Jira",
		ConnectorKind:    "jira_cloud",
		ScopeKind:        "project",
		ScopeKey:         "TPM",
		ScopeDisplayName: "TPM Project",
		CrawlPolicy:      "planned_project",
		Enabled:          true,
	}, &out)
	if err != nil {
		t.Fatalf("registerSourceScope returned error: %v", err)
	}

	var payload struct {
		SourceScopeRegister sourceScopeRegisterResult `json:"sourceScopeRegister"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode register output: %v\n%s", err, out.String())
	}
	if !payload.SourceScopeRegister.ConnectionCreated || !payload.SourceScopeRegister.ScopeCreated || !payload.SourceScopeRegister.StateCreated {
		t.Fatalf("created flags = %+v, want all true", payload.SourceScopeRegister)
	}
	if payload.SourceScopeRegister.FreshnessState != "unknown" || payload.SourceScopeRegister.CoverageMode != "unknown" {
		t.Fatalf("state = %s/%s, want unknown/unknown", payload.SourceScopeRegister.FreshnessState, payload.SourceScopeRegister.CoverageMode)
	}
	if payload.SourceScopeRegister.LastAttemptedAt != "" {
		t.Fatalf("last_attempted_at = %q, want empty", payload.SourceScopeRegister.LastAttemptedAt)
	}
	if payload.SourceScopeRegister.SyncRunCount != 0 {
		t.Fatalf("sync_run_count = %d, want 0", payload.SourceScopeRegister.SyncRunCount)
	}

	store, err := entstore.Open(ctx, entstore.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	defer store.Close()
	client := store.Client()
	if got := client.SourceConnection.Query().CountX(ctx); got != 1 {
		t.Fatalf("source connections = %d, want 1", got)
	}
	if got := client.SourceScope.Query().CountX(ctx); got != 1 {
		t.Fatalf("source scopes = %d, want 1", got)
	}
	if got := client.SourceSyncRun.Query().CountX(ctx); got != 0 {
		t.Fatalf("source sync runs = %d, want 0", got)
	}
	state := client.SourceScopeState.Query().OnlyX(ctx)
	if state.FreshnessState.String() != "unknown" || state.CoverageMode.String() != "unknown" {
		t.Fatalf("persisted state = %s/%s, want unknown/unknown", state.FreshnessState, state.CoverageMode)
	}
	if !state.LastAttemptedAt.IsZero() {
		t.Fatalf("persisted last_attempted_at = %s, want zero", state.LastAttemptedAt)
	}
	if !state.LastSuccessfulAt.IsZero() || state.LastSuccessfulSyncRunID != 0 {
		t.Fatalf("persisted successful sync fields = %s/%d, want empty", state.LastSuccessfulAt, state.LastSuccessfulSyncRunID)
	}
}

// TestExportWorkProgramGraphContextWritesGraphQLPayload keeps the LLM harness input on the GraphQL product boundary.
func TestExportWorkProgramGraphContextWritesGraphQLPayload(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ontology.db")
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	queryWorkstream := "workstream:" + workstream
	generatedAt := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	_, err = store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:" + workstream + ":export").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(50).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|export").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:export:subject").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#100").
		SetTitle("Export context subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("export:subject").
		SetRankScore(91).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close ent store: %v", err)
	}

	var out bytes.Buffer
	err = exportWorkProgramGraphContext(ctx, workProgramGraphContextExportConfig{
		DatabasePath:      dbPath,
		WorkstreamKey:     queryWorkstream,
		SourceInstance:    source,
		ItemLimit:         5,
		TraversalDepth:    2,
		EvidenceLimit:     10,
		SQLiteBusyTimeout: time.Second,
	}, &out)
	if err != nil {
		t.Fatalf("exportWorkProgramGraphContext returned error: %v", err)
	}

	var payload struct {
		Data struct {
			Context struct {
				SourceInstance   *string  `json:"sourceInstance"`
				WorkstreamKey    string   `json:"workstreamKey"`
				ContextHash      string   `json:"contextHash"`
				ItemCount        int      `json:"itemCount"`
				AllowedCitations []string `json:"allowedCitations"`
			} `json:"workProgramGraphContext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode export JSON: %v\n%s", err, out.String())
	}
	got := payload.Data.Context
	if got.WorkstreamKey != queryWorkstream {
		t.Fatalf("workstreamKey = %q, want %q", got.WorkstreamKey, queryWorkstream)
	}
	if got.SourceInstance == nil || *got.SourceInstance != source {
		t.Fatalf("sourceInstance = %#v, want %q", got.SourceInstance, source)
	}
	if got.ItemCount != 1 {
		t.Fatalf("itemCount = %d, want 1; payload=%s", got.ItemCount, out.String())
	}
	if got.ContextHash == "" {
		t.Fatalf("contextHash is empty: %s", out.String())
	}
	for _, want := range []string{
		"[context:" + got.ContextHash + "]",
		"[work_program_items:work-program-item:export:subject]",
	} {
		if !containsString(got.AllowedCitations, want) {
			t.Fatalf("allowedCitations missing %s: %#v", want, got.AllowedCitations)
		}
	}
}

// TestExportBoundedGraphContextWritesGenericPayload keeps the generic AI path out of WorkProgram fixtures.
func TestExportBoundedGraphContextWritesGenericPayload(t *testing.T) {
	var out bytes.Buffer
	err := exportBoundedGraphContext(context.Background(), boundedGraphContextExportConfig{
		Fixture:                "generic-doc-message-ticket",
		Depth:                  2,
		LimitPerObject:         4,
		CoverageState:          "sparse",
		AbsenceClaimsAllowed:   false,
		AbsenceClaimGateReason: "partial_message_history",
		CoverageSummary:        "Only selected document and message rows were loaded.",
	}, &out)
	if err != nil {
		t.Fatalf("exportBoundedGraphContext returned error: %v", err)
	}

	var payload struct {
		Context struct {
			ContextHash string `json:"contextHash"`
			Seed        struct {
				ObjectType string `json:"objectType"`
				Key        string `json:"key"`
			} `json:"seed"`
			Coverage struct {
				CoverageState        string `json:"coverageState"`
				AbsenceClaimsAllowed bool   `json:"absenceClaimsAllowed"`
			} `json:"coverage"`
			Guardrails []string `json:"guardrails"`
			Objects    []struct {
				ObjectType   string `json:"objectType"`
				Key          string `json:"key"`
				ClaimAllowed bool   `json:"claimAllowed"`
			} `json:"objects"`
			Associations []struct {
				AssociationType string `json:"associationType"`
				ClaimAllowed    bool   `json:"claimAllowed"`
				ClaimGateReason string `json:"claimGateReason"`
			} `json:"associations"`
		} `json:"boundedGraphContext"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode bounded graph export JSON: %v\n%s", err, out.String())
	}
	got := payload.Context
	if got.ContextHash == "" {
		t.Fatalf("contextHash is empty: %s", out.String())
	}
	if got.Seed.ObjectType != "document" || got.Seed.Key != "doc:architecture-note" {
		t.Fatalf("seed = %#v, want architecture document", got.Seed)
	}
	if got.Coverage.CoverageState != "sparse" || got.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("coverage = %#v, want sparse and absence-gated", got.Coverage)
	}
	if !containsString(got.Guardrails, "Source coverage gates absence claims; missing neighbors are unknown, not absent.") {
		t.Fatalf("missing sparse coverage guardrail: %#v", got.Guardrails)
	}
	if len(got.Objects) != 3 {
		t.Fatalf("objects = %#v, want three generic graph objects", got.Objects)
	}
	candidateFound := false
	for _, association := range got.Associations {
		if association.AssociationType == "possible_followup_for" {
			candidateFound = true
			if association.ClaimAllowed || association.ClaimGateReason != "candidate_link_requires_human_review" {
				t.Fatalf("candidate association promoted incorrectly: %#v", association)
			}
		}
	}
	if !candidateFound {
		t.Fatalf("candidate association missing: %#v", got.Associations)
	}
	for _, forbidden := range []string{"WorkProgram", "tpm_", "flink"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("generic bounded graph export leaked %q: %s", forbidden, out.String())
		}
	}
}

// TestExportBoundedGraphContextWritesCustomerIncidentRunbookPayload keeps the generic path off ticket/PR-shaped fixtures.
func TestExportBoundedGraphContextWritesCustomerIncidentRunbookPayload(t *testing.T) {
	var out bytes.Buffer
	err := exportBoundedGraphContext(context.Background(), boundedGraphContextExportConfig{
		Fixture: "customer-incident-runbook",
		AssociationTypes: []domain.AssociationType{
			domain.AssociationType("reported_incident"),
			domain.AssociationType("has_update"),
			domain.AssociationType("has_runbook"),
		},
		Depth:                  2,
		LimitPerObject:         4,
		CoverageState:          "sparse",
		AbsenceClaimsAllowed:   false,
		AbsenceClaimGateReason: "partial_incident_sources",
		CoverageSummary:        "Only selected customer incident rows were loaded.",
	}, &out)
	if err != nil {
		t.Fatalf("exportBoundedGraphContext returned error: %v", err)
	}

	var payload boundedGraphExportPayload
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode incident bounded graph export JSON: %v\n%s", err, out.String())
	}
	got := payload.Context
	if got.Seed.ObjectType != "customer_account" || got.Seed.Key != "customer-account:acme" {
		t.Fatalf("seed = %#v, want Acme customer account", got.Seed)
	}
	for _, want := range []string{
		"incident:payments-latency",
		"slack-message:payments-update-1",
		"runbook:payments-latency",
	} {
		if !boundedGraphObjectKeyExists(got.Objects, want) {
			t.Fatalf("incident context missing object %s: %#v", want, got.Objects)
		}
	}
	for _, forbidden := range []string{
		"slack-channel:customer-incidents",
		"incident:finance-export",
		"runbook:finance-export",
		"ticket:",
		"pull-request:",
	} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("incident context leaked %q: %s", forbidden, out.String())
		}
	}
	for _, want := range []string{"reported_incident", "has_update", "has_runbook"} {
		if !boundedGraphAssociationTypeExists(got.Associations, want) {
			t.Fatalf("incident context missing association %s: %#v", want, got.Associations)
		}
	}
	for _, forbidden := range []string{"WorkProgram", "TPM", "Flink", "implemented_by", "documented_by"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("incident bounded graph export leaked product vocabulary %q: %s", forbidden, out.String())
		}
	}
}

// TestExportBoundedGraphContextWritesEntCompanyFixture proves the generic AI path can read typed Ent product rows.
func TestExportBoundedGraphContextWritesEntCompanyFixture(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "company-ai-first.db")

	var documentOut bytes.Buffer
	err := exportBoundedGraphContext(ctx, boundedGraphContextExportConfig{
		DatabasePath:      dbPath,
		SQLiteBusyTimeout: time.Second,
		Fixture:           "company-ai-first-minimum",
		SeedFixture:       true,
		ResetDatabase:     true,
		Depth:             2,
		LimitPerObject:    8,
	}, &documentOut)
	if err != nil {
		t.Fatalf("export document Ent bounded graph context: %v", err)
	}

	var documentPayload boundedGraphExportPayload
	if err := json.Unmarshal(documentOut.Bytes(), &documentPayload); err != nil {
		t.Fatalf("decode document bounded graph export JSON: %v\n%s", err, documentOut.String())
	}
	documentContext := documentPayload.Context
	if documentContext.Seed.ObjectType != "document" || documentContext.Seed.Key != "document:company-plan" {
		t.Fatalf("document seed = %#v, want company plan document", documentContext.Seed)
	}
	if documentContext.Coverage.CoverageState != "limited" || documentContext.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("document coverage = %#v, want limited and absence-gated", documentContext.Coverage)
	}
	if documentContext.Coverage.AbsenceClaimGateReason != "source_auth_or_rate_limit" {
		t.Fatalf("coverage gate reason = %q, want auth/rate limit", documentContext.Coverage.AbsenceClaimGateReason)
	}
	for _, want := range []string{
		"document:api-reference",
		"ticket:COMP-101",
	} {
		if !boundedGraphObjectKeyExists(documentContext.Objects, want) {
			t.Fatalf("document context missing object %s: %#v", want, documentContext.Objects)
		}
	}
	for _, forbidden := range companyAIFirstDistractorKeys() {
		if boundedGraphObjectKeyExists(documentContext.Objects, forbidden) {
			t.Fatalf("document context leaked distractor object %s: %#v", forbidden, documentContext.Objects)
		}
	}
	for _, want := range []string{"documented_by", "links_to"} {
		if !boundedGraphAssociationTypeExists(documentContext.Associations, want) {
			t.Fatalf("document context missing association %s: %#v", want, documentContext.Associations)
		}
	}
	for _, forbidden := range []string{"WorkProgram", "tpm_", "flink"} {
		if strings.Contains(documentOut.String(), forbidden) {
			t.Fatalf("Ent bounded graph export leaked %q: %s", forbidden, documentOut.String())
		}
	}

	var personOut bytes.Buffer
	err = exportBoundedGraphContext(ctx, boundedGraphContextExportConfig{
		DatabasePath:      dbPath,
		SQLiteBusyTimeout: time.Second,
		Fixture:           "company-ai-first-minimum",
		StartObjectType:   "person",
		StartKey:          "person:alice",
		Depth:             1,
		LimitPerObject:    8,
	}, &personOut)
	if err != nil {
		t.Fatalf("export person Ent bounded graph context: %v", err)
	}
	var personPayload boundedGraphExportPayload
	if err := json.Unmarshal(personOut.Bytes(), &personPayload); err != nil {
		t.Fatalf("decode person bounded graph export JSON: %v\n%s", err, personOut.String())
	}
	personContext := personPayload.Context
	if personContext.Coverage.CoverageState != "sparse" || personContext.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("person coverage = %#v, want sparse and absence-gated", personContext.Coverage)
	}
	for _, want := range []string{
		"ticket:COMP-101",
		"pull-request:company/app#42",
	} {
		if !boundedGraphObjectKeyExists(personContext.Objects, want) {
			t.Fatalf("person context missing object %s: %#v", want, personContext.Objects)
		}
	}
	for _, forbidden := range companyAIFirstDistractorKeys() {
		if boundedGraphObjectKeyExists(personContext.Objects, forbidden) {
			t.Fatalf("person context leaked distractor object %s: %#v", forbidden, personContext.Objects)
		}
	}
	for _, want := range []string{"assignee", "author"} {
		if !boundedGraphAssociationTypeExists(personContext.Associations, want) {
			t.Fatalf("person context missing association %s: %#v", want, personContext.Associations)
		}
	}

	var seedOnlyOut bytes.Buffer
	err = exportBoundedGraphContext(ctx, boundedGraphContextExportConfig{
		DatabasePath:      dbPath,
		SQLiteBusyTimeout: time.Second,
		Fixture:           "company-ai-first-minimum",
		StartObjectType:   "document",
		StartKey:          "document:company-plan",
		Depth:             0,
		LimitPerObject:    8,
	}, &seedOnlyOut)
	if err != nil {
		t.Fatalf("export seed-only Ent bounded graph context: %v", err)
	}
	var seedOnlyPayload boundedGraphExportPayload
	if err := json.Unmarshal(seedOnlyOut.Bytes(), &seedOnlyPayload); err != nil {
		t.Fatalf("decode seed-only bounded graph export JSON: %v\n%s", err, seedOnlyOut.String())
	}
	seedOnlyContext := seedOnlyPayload.Context
	if len(seedOnlyContext.Objects) != 1 || seedOnlyContext.Objects[0].Key != "document:company-plan" {
		t.Fatalf("seed-only objects = %#v, want only company plan", seedOnlyContext.Objects)
	}
	if len(seedOnlyContext.Associations) != 0 {
		t.Fatalf("seed-only associations = %#v, want none", seedOnlyContext.Associations)
	}
	for _, forbidden := range companyAIFirstDistractorKeys() {
		if boundedGraphObjectKeyExists(seedOnlyContext.Objects, forbidden) {
			t.Fatalf("seed-only context leaked distractor object %s: %#v", forbidden, seedOnlyContext.Objects)
		}
	}
}

// TestOpenGraphFixtureLoadFeedsEntBoundedGraphExport proves the generic AI path can start from data-backed open graph rows.
func TestOpenGraphFixtureLoadFeedsEntBoundedGraphExport(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "open-graph.db")
	fixturePath := filepath.Join(dir, "open_graph_fixture.json")
	sourceAuthorityPath := filepath.Join(dir, "source_authority.json")
	if err := os.WriteFile(fixturePath, []byte(`{
	  "sourceInstance": "test-open-graph-fixture",
	  "observedAt": "2026-06-24T12:00:00Z",
	  "objects": [
	    {
	      "objectType": "customer_account",
	      "key": "customer:acme",
	      "title": "Acme",
	      "sourceSystem": "crm",
	      "externalKind": "account",
	      "externalID": "acme",
	      "visibility": "public",
	      "freshnessState": "fresh",
	      "rankScore": 10
	    },
	    {
	      "objectType": "incident",
	      "key": "incident:sev-42",
	      "title": "Checkout outage",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident",
	      "externalID": "sev-42",
	      "visibility": "public",
	      "freshnessState": "fresh",
	      "rankScore": 9
	    },
	    {
	      "objectType": "incident",
	      "key": "incident:hidden-private",
	      "title": "Private executive-impact outage",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident",
	      "externalID": "hidden-private",
	      "visibility": "private",
	      "freshnessState": "fresh",
	      "rankScore": 20
	    },
	    {
	      "objectType": "incident",
	      "key": "incident:missing-freshness",
	      "title": "Public incident with missing freshness",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident",
	      "externalID": "missing-freshness",
	      "visibility": "public",
	      "rankScore": 8
	    }
	  ],
	  "associations": [
	    {
	      "from": {"objectType": "customer_account", "key": "customer:acme"},
	      "to": {"objectType": "incident", "key": "incident:hidden-private"},
	      "associationType": "affected_by",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident_link",
	      "externalID": "acme:hidden-private",
	      "locatorKind": "incident_link",
	      "locator": "pagerduty://incidents/hidden-private#customer-acme",
	      "visibility": "private",
	      "freshnessState": "fresh",
	      "rankScore": 20
	    },
	    {
	      "from": {"objectType": "customer_account", "key": "customer:acme"},
	      "to": {"objectType": "incident", "key": "incident:sev-42"},
	      "associationType": "affected_by",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident_link",
	      "externalID": "acme:sev-42",
	      "locatorKind": "incident_link",
	      "locator": "pagerduty://incidents/sev-42#customer-acme",
	      "visibility": "public",
	      "freshnessState": "fresh",
	      "rankScore": 9
	    },
	    {
	      "from": {"objectType": "customer_account", "key": "customer:acme"},
	      "to": {"objectType": "incident", "key": "incident:missing-freshness"},
	      "associationType": "affected_by",
	      "sourceSystem": "pagerduty",
	      "externalKind": "incident_link",
	      "externalID": "acme:missing-freshness",
	      "locatorKind": "incident_link",
	      "locator": "pagerduty://incidents/missing-freshness#customer-acme",
	      "visibility": "public",
	      "rankScore": 8
	    }
	  ]
	}`), 0o644); err != nil {
		t.Fatalf("write open graph fixture: %v", err)
	}
	if err := os.WriteFile(sourceAuthorityPath, []byte(`{
	  "relationship_authority": {
	    "affected_by": {
	      "presence_sources": ["pagerduty"],
	      "presence_locator_kinds": {
	        "pagerduty": ["incident_link"]
	      }
	    }
	  }
	}`), 0o644); err != nil {
		t.Fatalf("write source authority policy: %v", err)
	}

	var loadOut bytes.Buffer
	if err := loadOpenGraphFixture(ctx, openGraphFixtureLoadConfig{
		FixturePath:       fixturePath,
		DatabasePath:      dbPath,
		SQLiteBusyTimeout: time.Second,
		ResetDatabase:     true,
	}, &loadOut); err != nil {
		t.Fatalf("loadOpenGraphFixture returned error: %v", err)
	}
	for _, want := range []string{
		`"objectCount": 4`,
		`"associationCount": 3`,
		`"evidenceCount": 3`,
	} {
		if !strings.Contains(loadOut.String(), want) {
			t.Fatalf("load output missing %s: %s", want, loadOut.String())
		}
	}

	var exportOut bytes.Buffer
	if err := exportBoundedGraphContext(ctx, boundedGraphContextExportConfig{
		DatabasePath:         dbPath,
		SQLiteBusyTimeout:    time.Second,
		Fixture:              openGraphFixtureName,
		SourceAuthorityPath:  sourceAuthorityPath,
		StartObjectType:      "customer_account",
		StartKey:             "customer:acme",
		AssociationTypes:     []domain.AssociationType{domain.AssociationType("affected_by")},
		Depth:                1,
		LimitPerObject:       4,
		AbsenceClaimsAllowed: false,
	}, &exportOut); err != nil {
		t.Fatalf("exportBoundedGraphContext returned error: %v", err)
	}

	var payload boundedGraphExportPayload
	if err := json.Unmarshal(exportOut.Bytes(), &payload); err != nil {
		t.Fatalf("decode open graph bounded graph export JSON: %v\n%s", err, exportOut.String())
	}
	got := payload.Context
	if got.Seed.ObjectType != "customer_account" || got.Seed.Key != "customer:acme" {
		t.Fatalf("seed = %#v, want Acme customer account", got.Seed)
	}
	if !boundedGraphObjectKeyExists(got.Objects, "incident:sev-42") {
		t.Fatalf("open graph context missing incident object: %#v", got.Objects)
	}
	publicIncident := boundedGraphObjectByKey(got.Objects, "incident:sev-42")
	if publicIncident == nil {
		t.Fatalf("open graph context missing public incident: %#v", got.Objects)
	}
	if publicIncident.ClaimAllowed || publicIncident.ClaimGateReason != "open_graph_object_context_only" {
		t.Fatalf("open graph object promoted incorrectly: %#v", publicIncident)
	}
	if boundedGraphObjectKeyExists(got.Objects, "incident:hidden-private") {
		t.Fatalf("open graph context leaked private incident for public export: %#v", got.Objects)
	}
	if !boundedGraphAssociationTypeExists(got.Associations, "affected_by") {
		t.Fatalf("open graph context missing claimable affected_by association: %#v", got.Associations)
	}
	missingFreshnessAssociation := boundedGraphAssociationToKey(got.Associations, "incident:missing-freshness")
	if missingFreshnessAssociation == nil {
		t.Fatalf("open graph context missing freshness regression association: %#v", got.Associations)
	}
	if missingFreshnessAssociation.ClaimAllowed || missingFreshnessAssociation.ClaimGateReason != "relationship_endpoint_freshness_unknown" {
		t.Fatalf("missing freshness association promoted incorrectly: %#v", missingFreshnessAssociation)
	}
	for _, forbidden := range []string{"WorkProgram", "tpm_", "flink", "ticket:", "pull-request:"} {
		if strings.Contains(exportOut.String(), forbidden) {
			t.Fatalf("open graph export leaked product vocabulary %q: %s", forbidden, exportOut.String())
		}
	}

	var privateAllowedOut bytes.Buffer
	if err := exportBoundedGraphContext(ctx, boundedGraphContextExportConfig{
		DatabasePath:              dbPath,
		SQLiteBusyTimeout:         time.Second,
		Fixture:                   openGraphFixtureName,
		SourceAuthorityPath:       sourceAuthorityPath,
		PrincipalKey:              "principal:private-reader",
		AllowedVisibilityClasses:  []string{"private"},
		StartObjectType:           "customer_account",
		StartKey:                  "customer:acme",
		AssociationTypes:          []domain.AssociationType{domain.AssociationType("affected_by")},
		Depth:                     1,
		LimitPerObject:            1,
		AbsenceClaimsAllowed:      false,
		AbsenceClaimGateReason:    "test_acl_probe",
		PrincipalCoverageComplete: true,
	}, &privateAllowedOut); err != nil {
		t.Fatalf("export private-allowed bounded graph context returned error: %v", err)
	}
	var privateAllowedPayload boundedGraphExportPayload
	if err := json.Unmarshal(privateAllowedOut.Bytes(), &privateAllowedPayload); err != nil {
		t.Fatalf("decode private-allowed bounded graph export JSON: %v\n%s", err, privateAllowedOut.String())
	}
	privateAllowed := privateAllowedPayload.Context
	if !boundedGraphObjectKeyExists(privateAllowed.Objects, "incident:hidden-private") {
		t.Fatalf("private-allowed context missing private incident: %#v", privateAllowed.Objects)
	}
	if boundedGraphObjectKeyExists(privateAllowed.Objects, "incident:sev-42") {
		t.Fatalf("private-allowed context should spend fanout on higher-ranked private edge first: %#v", privateAllowed.Objects)
	}
}

// TestParseServeConfigUsesEnvironmentDefaults checks env vars feed the serve runtime.
func TestParseServeConfigUsesEnvironmentDefaults(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":                "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_DATABASE_PATH":              "/tmp/cubicle-ontology/env.db",
		"CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT":        "1200ms",
		"CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED": "false",
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("parse env serve config: %v", err)
	}

	if cfg.Listen != "127.0.0.1:49090" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/env.db" {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 1200*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigLoadsHOCONFile checks file-backed runtime defaults.
func TestParseServeConfigLoadsHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/cubicle-config-file.db"
graphql.playground_enabled = false
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve", "--config", path}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse config file: %v", err)
	}

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.Listen != "127.0.0.1:49300" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-config-file.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigLoadsHOCONPathFromEnv checks env can select the config file.
func TestParseServeConfigLoadsHOCONPathFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49350"
storage.database_path = "/tmp/cubicle-config-env-file.db"
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_CONFIG_PATH": path,
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("parse config file from env: %v", err)
	}

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.Listen != "127.0.0.1:49350" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-config-env-file.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

// TestParseServeConfigFlagsOverrideHOCONFile keeps explicit CLI flags highest priority.
func TestParseServeConfigFlagsOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/from-file.db"
graphql.playground_enabled = true
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{
		"serve",
		"--config", path,
		"--listen", "127.0.0.1:49400",
		"--database", "/tmp/from-flag.db",
		"--sqlite-busy-timeout", "1500ms",
		"--graphql-playground=false",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse config file: %v", err)
	}

	if cfg.Listen != "127.0.0.1:49400" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/from-flag.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 1500*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigRejectsPublicBindWithoutFlag protects local-only default serving.
func TestParseServeConfigRejectsPublicBindWithoutFlag(t *testing.T) {
	_, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080"}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected public bind without --allow-public-bind to fail")
	}
}

// TestParseServeConfigAllowsPublicBindWithFlag keeps public bind opt-in and explicit.
func TestParseServeConfigAllowsPublicBindWithFlag(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080", "--allow-public-bind"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse public serve config: %v", err)
	}
	if cfg.Listen != "0.0.0.0:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
}

// TestHTTPServerUsesTimeouts checks the service has fixed HTTP timeout bounds.
func TestHTTPServerUsesTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer(serveConfig{Listen: "127.0.0.1:48080"}, handler)

	if server.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != serverReadTimeout {
		t.Fatalf("ReadTimeout = %s", server.ReadTimeout)
	}
	if server.WriteTimeout != serverWriteTimeout {
		t.Fatalf("WriteTimeout = %s", server.WriteTimeout)
	}
	if server.IdleTimeout != serverIdleTimeout {
		t.Fatalf("IdleTimeout = %s", server.IdleTimeout)
	}
}

// testFixtureRecord is one raw capture row plus body bytes for CLI fixture tests.
type testFixtureRecord struct {
	Path       string
	Source     string
	ObjectType string
	ObjectID   string
	Status     int
	Body       []byte
}

type boundedGraphExportPayload struct {
	Context struct {
		ContextHash string `json:"contextHash"`
		Seed        struct {
			ObjectType string `json:"objectType"`
			Key        string `json:"key"`
		} `json:"seed"`
		Coverage struct {
			CoverageState          string `json:"coverageState"`
			AbsenceClaimsAllowed   bool   `json:"absenceClaimsAllowed"`
			AbsenceClaimGateReason string `json:"absenceClaimGateReason"`
		} `json:"coverage"`
		Objects      []boundedGraphObjectPayload      `json:"objects"`
		Associations []boundedGraphAssociationPayload `json:"associations"`
	} `json:"boundedGraphContext"`
}

type boundedGraphObjectPayload struct {
	ObjectType      string `json:"objectType"`
	Key             string `json:"key"`
	ClaimAllowed    bool   `json:"claimAllowed"`
	ClaimGateReason string `json:"claimGateReason"`
}

type boundedGraphRefPayload struct {
	ObjectType string `json:"objectType"`
	Key        string `json:"key"`
}

type boundedGraphAssociationPayload struct {
	Key             string                 `json:"key"`
	AssociationType string                 `json:"associationType"`
	To              boundedGraphRefPayload `json:"to"`
	ClaimAllowed    bool                   `json:"claimAllowed"`
	ClaimGateReason string                 `json:"claimGateReason"`
}

// writeTestFixture writes body files and manifest rows for command-level replay tests.
func writeTestFixture(t *testing.T, dir string, records []testFixtureRecord) {
	t.Helper()
	lines := make([]string, 0, len(records)+1)
	for _, record := range records {
		path := filepath.Join(dir, record.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture path: %v", err)
		}
		if err := os.WriteFile(path, record.Body, 0o644); err != nil {
			t.Fatalf("write fixture body: %v", err)
		}
		lines = append(lines, fmt.Sprintf(
			`{"path":%q,"source":%q,"source_object_type":%q,"source_object_id":%q,"url":%q,"status_code":%d,"body_sha256":%q,"bytes":%d}`,
			record.Path,
			record.Source,
			record.ObjectType,
			record.ObjectID,
			"https://example.test/"+record.ObjectType+"/"+record.ObjectID,
			record.Status,
			sourcecapture.HashBody(record.Body),
			len(record.Body),
		))
	}
	lines = append(lines, "")
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}
}

func boundedGraphObjectKeyExists(values []boundedGraphObjectPayload, want string) bool {
	for _, value := range values {
		if value.Key == want {
			return true
		}
	}
	return false
}

func boundedGraphObjectByKey(values []boundedGraphObjectPayload, want string) *boundedGraphObjectPayload {
	for index := range values {
		if values[index].Key == want {
			return &values[index]
		}
	}
	return nil
}

func boundedGraphAssociationTypeExists(values []boundedGraphAssociationPayload, want string) bool {
	for _, value := range values {
		if value.AssociationType == want && value.ClaimAllowed {
			return true
		}
	}
	return false
}

func boundedGraphAssociationToKey(values []boundedGraphAssociationPayload, want string) *boundedGraphAssociationPayload {
	for index := range values {
		if values[index].To.Key == want {
			return &values[index]
		}
	}
	return nil
}

func companyAIFirstDistractorKeys() []string {
	return []string{
		"ticket:COMP-999",
		"pull-request:company/app#99",
		"person:mallory",
		"document:unrelated-roadmap",
		"message:finance-thread",
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
