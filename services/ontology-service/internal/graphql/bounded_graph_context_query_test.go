package graphql

import (
	"context"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/entgraph"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphcontext"
	"cubicle/services/ontology-service/internal/graphql/model"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
	"cubicle/services/ontology-service/internal/sampledata"
)

func TestBoundedGraphContextBuildsGenericTypedContext(t *testing.T) {
	ctx := context.Background()
	resolver := (&Resolver{
		GraphExpander: sampledata.NewGenericDocumentMessageTicketMemoryStore(),
	}).Query().(*queryResolver)

	got, err := resolver.BoundedGraphContext(
		ctx,
		"document",
		"doc:architecture-note",
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.ContextHash == "" || len(got.ContextHash) != 16 {
		t.Fatalf("contextHash = %q, want stable 16-char hash", got.ContextHash)
	}
	if got.ScopeMode != "bounded_graph_context" {
		t.Fatalf("scopeMode = %q, want bounded_graph_context", got.ScopeMode)
	}
	if got.Seed.ObjectType != "document" || got.Seed.Key != "doc:architecture-note" {
		t.Fatalf("seed = %#v, want document architecture note", got.Seed)
	}
	if got.Depth != 2 || got.LimitPerObject != 4 {
		t.Fatalf("bounds = depth:%d limit:%d, want 2/4 defaults", got.Depth, got.LimitPerObject)
	}
	if got.Coverage == nil || got.Coverage.CoverageState != "sparse" || got.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("coverage = %#v, want sparse with absence claims gated", got.Coverage)
	}
	if got.Coverage.Summary == nil || !strings.Contains(*got.Coverage.Summary, "server-owned") {
		t.Fatalf("coverage summary = %#v, want server-owned coverage caveat", got.Coverage.Summary)
	}
	if len(got.Objects) != 3 {
		t.Fatalf("objects = %d, want document/message/ticket", len(got.Objects))
	}
	if len(got.Associations) != 2 {
		t.Fatalf("associations = %d, want mentions and candidate follow-up", len(got.Associations))
	}
	if len(got.Evidence) != 2 {
		t.Fatalf("evidence = %d, want one row per association evidence key", len(got.Evidence))
	}

	serialized := boundedGraphContextSearchText(got)
	for _, forbidden := range []string{"flink", "tpm", "workprogram"} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("bounded context unexpectedly contains %q in %s", forbidden, serialized)
		}
	}

	mentions := associationByType(got.Associations, "mentions")
	if mentions == nil || !mentions.ClaimAllowed || mentions.ClaimGateReason != "source_evidence_full_confidence" {
		t.Fatalf("mentions association = %#v, want claimable full-confidence evidence", mentions)
	}
	if mentions.ProofState != "source_observed" || mentions.EvidenceKey == nil || *mentions.EvidenceKey != "evidence:doc-message" || pointerString(mentions.Visibility) != "public" || pointerString(mentions.FreshnessState) != "fresh" {
		t.Fatalf("mentions provenance = %#v, want source-observed public fresh evidence", mentions)
	}
	mentionsEvidence := evidenceByKey(got.Evidence, "evidence:doc-message")
	if mentionsEvidence == nil || pointerString(mentionsEvidence.Source) != "generic_sampledata" || pointerString(mentionsEvidence.SourceInstance) != "generic-doc-message-ticket" || pointerString(mentionsEvidence.LocatorKind) != "generic_relation" {
		t.Fatalf("mentions evidence = %#v, want source provenance preserved", mentionsEvidence)
	}
	candidate := associationByType(got.Associations, "possible_followup_for")
	if candidate == nil {
		t.Fatalf("missing candidate association in %#v", got.Associations)
	}
	if candidate.ClaimAllowed || candidate.ClaimGateReason != "candidate_link_requires_human_review" || candidate.ProofState != "candidate" {
		t.Fatalf("candidate association = %#v, want non-claimable candidate", candidate)
	}
	if candidate.Confidence != 0.4 {
		t.Fatalf("candidate confidence = %v, want 0.4", candidate.Confidence)
	}
	if candidate.EvidenceKey == nil || *candidate.EvidenceKey != "evidence:message-ticket-candidate" {
		t.Fatalf("candidate evidence = %#v, want candidate evidence key", candidate.EvidenceKey)
	}
}

func TestBoundedGraphCoverageModelExposesCoverageScope(t *testing.T) {
	got := boundedGraphCoverageModel(graphcontext.CoveragePolicy{
		CoverageState:                "complete",
		AbsenceClaimsAllowed:         true,
		AbsenceClaimGateReason:       "complete_relation_path_coverage",
		AbsenceClaimAssociationTypes: []string{"implemented_by", "mentions"},
		SourceSystem:                 "github",
		SourceInstance:               "apache/flink-kubernetes-operator",
		CoverageWindowStart:          "2026-06-24T00:00:00Z",
		CoverageWindowEnd:            "2026-06-24T01:00:00Z",
		Summary:                      "Complete for selected relationship paths and source window.",
	})

	if got.CoverageState != "complete" || !got.AbsenceClaimsAllowed || got.AbsenceClaimGateReason != "complete_relation_path_coverage" {
		t.Fatalf("coverage gate = %#v, want complete absence-claim gate", got)
	}
	if strings.Join(got.AbsenceClaimAssociationTypes, ",") != "implemented_by,mentions" {
		t.Fatalf("absence claim association types = %#v", got.AbsenceClaimAssociationTypes)
	}
	if pointerString(got.SourceSystem) != "github" || pointerString(got.SourceInstance) != "apache/flink-kubernetes-operator" {
		t.Fatalf("coverage source = %v/%v, want github/apache flink operator", pointerString(got.SourceSystem), pointerString(got.SourceInstance))
	}
	if pointerString(got.CoverageWindowStart) != "2026-06-24T00:00:00Z" || pointerString(got.CoverageWindowEnd) != "2026-06-24T01:00:00Z" {
		t.Fatalf("coverage window = %v..%v, want explicit start/end", pointerString(got.CoverageWindowStart), pointerString(got.CoverageWindowEnd))
	}
	if pointerString(got.Summary) != "Complete for selected relationship paths and source window." {
		t.Fatalf("summary = %#v", got.Summary)
	}
}

func TestBoundedGraphContextUsesSeedObservationCoverageWhenNoSourceScopeState(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	observedAt := time.Date(2026, 6, 24, 12, 34, 56, 0, time.UTC)
	_, err := store.Client().Ticket.Create().
		SetKey("ticket:SPARSE-OBSERVED").
		SetTitle("Sparse observed ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("apache-jira").
		SetExternalKind("jira_issue").
		SetExternalID("SPARSE-OBSERVED").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create unscoped ticket: %v", err)
	}

	depth := 0
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:SPARSE-OBSERVED", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "sparse" || got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_coverage_gate" {
		t.Fatalf("coverage = %#v, want sparse seed observation with absence claims gated", got.Coverage)
	}
	if pointerString(got.Coverage.SourceSystem) != "jira" || pointerString(got.Coverage.SourceInstance) != "apache-jira" {
		t.Fatalf("coverage source = %v/%v, want seed source identity", pointerString(got.Coverage.SourceSystem), pointerString(got.Coverage.SourceInstance))
	}
	if pointerString(got.Coverage.CoverageWindowStart) != "2026-06-24T12:34:56Z" || pointerString(got.Coverage.CoverageWindowEnd) != "2026-06-24T12:34:56Z" {
		t.Fatalf("coverage observation window = %v..%v, want seed last_confirmed_at point window", pointerString(got.Coverage.CoverageWindowStart), pointerString(got.Coverage.CoverageWindowEnd))
	}
	if got.Coverage.Summary == nil || !strings.Contains(*got.Coverage.Summary, "Sparse seed observation") {
		t.Fatalf("coverage summary = %#v, want sparse observation caveat", got.Coverage.Summary)
	}
}

func TestBoundedGraphContextUsesSourceScopeCoverageForAbsenceClaims(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-1",
		crawlPolicy:   "bounded_graph_absence=implemented_by",
		withRunWindow: true,
	})
	queryCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey:                 "principal:coverage-owner",
		AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
		CoverageCompleteForPrincipal: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(queryCtx, string(ontology.ObjectTicket), "ticket:COVERAGE-1", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" || !got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "complete_relation_path_coverage" {
		t.Fatalf("coverage = %#v, want complete relation/path/source/time coverage", got.Coverage)
	}
	if strings.Join(got.Coverage.AbsenceClaimAssociationTypes, ",") != string(ontology.AssocImplementedBy) {
		t.Fatalf("absence association types = %#v, want implemented_by", got.Coverage.AbsenceClaimAssociationTypes)
	}
	if pointerString(got.Coverage.SourceSystem) != "jira" || pointerString(got.Coverage.SourceInstance) != "company" {
		t.Fatalf("coverage source = %v/%v, want jira/company", pointerString(got.Coverage.SourceSystem), pointerString(got.Coverage.SourceInstance))
	}
	if pointerString(got.Coverage.CoverageWindowStart) != "2026-06-24T10:00:00Z" || pointerString(got.Coverage.CoverageWindowEnd) != "2026-06-24T11:00:00Z" {
		t.Fatalf("coverage window = %v..%v, want explicit source run window", pointerString(got.Coverage.CoverageWindowStart), pointerString(got.Coverage.CoverageWindowEnd))
	}
}

func TestBoundedGraphContextKeepsAbsenceClaimsGatedWithoutPrincipalCoverage(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-NO-PRINCIPAL",
		crawlPolicy:   "bounded_graph_absence=implemented_by",
		withRunWindow: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:COVERAGE-NO-PRINCIPAL", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" {
		t.Fatalf("coverage state = %q, want complete source scope", got.Coverage.CoverageState)
	}
	if got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "principal_coverage_required" {
		t.Fatalf("coverage = %#v, want principal coverage gate", got.Coverage)
	}
}

func TestBoundedGraphContextKeepsAbsenceClaimsGatedWithoutRelationCoverage(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-NO-RELATION",
		crawlPolicy:   "fixture_replay",
		withRunWindow: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:COVERAGE-NO-RELATION", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" {
		t.Fatalf("coverage state = %q, want complete source scope", got.Coverage.CoverageState)
	}
	if got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_scope_relation_coverage_required" {
		t.Fatalf("coverage = %#v, want relation coverage gate", got.Coverage)
	}
}

func TestBoundedGraphContextKeepsAbsenceClaimsGatedForUnfilteredTraversal(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-UNFILTERED",
		crawlPolicy:   "bounded_graph_absence=implemented_by",
		withRunWindow: true,
	})
	queryCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey:                 "principal:coverage-owner",
		AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
		CoverageCompleteForPrincipal: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(queryCtx, string(ontology.ObjectTicket), "ticket:COVERAGE-UNFILTERED", nil, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" {
		t.Fatalf("coverage state = %q, want complete source scope", got.Coverage.CoverageState)
	}
	if got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "relation_path_coverage_required" {
		t.Fatalf("coverage = %#v, want relation/path gate for unfiltered traversal", got.Coverage)
	}
}

func TestBoundedGraphContextKeepsAbsenceClaimsGatedWithoutCoverageWindow(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-NO-WINDOW",
		crawlPolicy:   "bounded_graph_absence=implemented_by",
		withRunWindow: false,
	})
	queryCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey:                 "principal:coverage-owner",
		AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
		CoverageCompleteForPrincipal: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(queryCtx, string(ontology.ObjectTicket), "ticket:COVERAGE-NO-WINDOW", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" {
		t.Fatalf("coverage state = %q, want complete source scope", got.Coverage.CoverageState)
	}
	if got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_time_window_required" {
		t.Fatalf("coverage = %#v, want source time-window gate", got.Coverage)
	}
}

func TestBoundedGraphContextSourceSyncIssueOverridesCompleteCoverage(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	fixture := seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), boundedGraphCoverageFixture{
		ticketKey:     "ticket:COVERAGE-FORBIDDEN",
		crawlPolicy:   "bounded_graph_absence=implemented_by",
		withRunWindow: true,
	})
	_, err := store.Client().SourceSyncIssue.Create().
		SetScope(fixture.scope).
		SetSyncRun(fixture.run).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_forbidden").
		SetMessage("source snapshot returned status 403; body retained for replay coverage only").
		SetSourceSystem("jira").
		SetSourceInstance("company").
		SetExternalKind("jira_issue").
		SetExternalID("COVERAGE-FORBIDDEN").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source sync issue: %v", err)
	}

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:COVERAGE-FORBIDDEN", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "limited" || got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_auth_or_rate_limit" {
		t.Fatalf("coverage = %#v, want auth/rate-limit issue to gate absence claims", got.Coverage)
	}
	if got.Coverage.Summary == nil || !strings.Contains(*got.Coverage.Summary, "coverage evidence only") {
		t.Fatalf("coverage summary = %#v, want source issue evidence-only caveat", got.Coverage.Summary)
	}
}

func TestBoundedGraphContextSourceScopedRowStillSeesStreamScopeSourceIssue(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedStart(t, ctx, store.Client(), boundedGraphCoverageStartFixture{
		objectType:        ontology.ObjectPullRequest,
		key:               "pull-request:company/app#42",
		association:       domain.AssociationType("reviewer"),
		withRunWindow:     true,
		stateCoverageMode: sourcescopestate.CoverageModePartialScope,
	})
	streamConnection, err := store.Client().SourceConnection.Create().
		SetKey("source-connection:fixture-workstream:company-app").
		SetSourceSystem("fixture").
		SetSourceInstance("company-app-workstream").
		SetDisplayName("fixture workstream").
		Save(ctx)
	if err != nil {
		t.Fatalf("create stream source connection: %v", err)
	}
	streamScope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:fixture-workstream:company-app").
		SetConnection(streamConnection).
		SetScopeKind("workstream").
		SetScopeKey("company-app").
		SetDisplayName("company app fixture stream").
		SetCrawlPolicy("fixture_replay").
		Save(ctx)
	if err != nil {
		t.Fatalf("create stream source scope: %v", err)
	}
	streamRun, err := store.Client().SourceSyncRun.Create().
		SetScope(streamScope).
		SetRunKey("source-sync-run:fixture-workstream:company-app").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusPartial).
		SetStartedAt(time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)).
		SetCompletedAt(time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stream source sync run: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(streamScope).
		SetSyncRun(streamRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_rate_limited").
		SetMessage("review endpoint returned 429; raw body retained for replay coverage only").
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request_reviews").
		SetExternalID("company/app#42").
		Save(ctx)
	if err != nil {
		t.Fatalf("create stream source sync issue: %v", err)
	}

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectPullRequest), "pull-request:company/app#42", []string{"reviewer"}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "limited" || got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_auth_or_rate_limit" {
		t.Fatalf("coverage = %#v, want stream-scope source issue to gate source-scoped PR row", got.Coverage)
	}
}

func TestBoundedGraphContextSourceCoverageMatrixGatesAbsenceClaims(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name              string
		ticketKey         string
		fixture           boundedGraphCoverageFixture
		principalComplete bool
		associationTypes  []string
		sourceSyncIssue   bool
		wantState         string
		wantAllowed       bool
		wantReason        string
		wantWindowStart   string
		wantWindowEnd     string
	}{
		{
			name:              "complete scoped source relation time and principal coverage",
			ticketKey:         "ticket:MATRIX-COMPLETE",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			wantState:         "complete",
			wantAllowed:       true,
			wantReason:        "complete_relation_path_coverage",
		},
		{
			name:              "wrong source instance",
			ticketKey:         "ticket:MATRIX-WRONG-SOURCE",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true, rowSourceInstance: "other-company"},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			wantState:         "limited",
			wantReason:        "source_scope_identity_mismatch",
		},
		{
			name:              "stale source scope",
			ticketKey:         "ticket:MATRIX-STALE",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true, freshnessState: sourcescopestate.FreshnessStateStale},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			wantState:         "limited",
			wantReason:        "source_scope_not_fresh",
		},
		{
			name:              "partial source scope",
			ticketKey:         "ticket:MATRIX-PARTIAL",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true, coverageMode: sourcescopestate.CoverageModePartialScope},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			wantState:         "limited",
			wantReason:        "source_scope_not_exact",
			wantWindowStart:   "2026-06-24T11:00:00Z",
			wantWindowEnd:     "2026-06-24T11:00:00Z",
		},
		{
			name:             "missing principal coverage",
			ticketKey:        "ticket:MATRIX-NO-PRINCIPAL",
			fixture:          boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true},
			associationTypes: []string{string(ontology.AssocImplementedBy)},
			wantState:        "complete",
			wantReason:       "principal_coverage_required",
		},
		{
			name:              "unfiltered traversal",
			ticketKey:         "ticket:MATRIX-UNFILTERED",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true},
			principalComplete: true,
			wantState:         "complete",
			wantReason:        "relation_path_coverage_required",
		},
		{
			name:              "missing source time window",
			ticketKey:         "ticket:MATRIX-NO-WINDOW",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: false},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			wantState:         "complete",
			wantReason:        "source_time_window_required",
		},
		{
			name:              "auth or rate limited source issue",
			ticketKey:         "ticket:MATRIX-RATE-LIMIT",
			fixture:           boundedGraphCoverageFixture{crawlPolicy: "bounded_graph_absence=implemented_by", withRunWindow: true},
			principalComplete: true,
			associationTypes:  []string{string(ontology.AssocImplementedBy)},
			sourceSyncIssue:   true,
			wantState:         "limited",
			wantReason:        "source_auth_or_rate_limit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, resolver := newBoundedGraphEntResolver(t, ctx)
			defer store.Close()
			tc.fixture.ticketKey = tc.ticketKey
			rows := seedBoundedGraphCoverageScopedTicket(t, ctx, store.Client(), tc.fixture)
			if tc.sourceSyncIssue {
				_, err := store.Client().SourceSyncIssue.Create().
					SetScope(rows.scope).
					SetSyncRun(rows.run).
					SetSeverity(sourcesyncissue.SeverityWarning).
					SetIssueCode("github_rate_limited_429").
					SetMessage("raw status 429 body token=secret must stay replay-only").
					SetSourceSystem("jira").
					SetSourceInstance("company").
					SetExternalKind("jira_issue").
					SetExternalID(strings.TrimPrefix(tc.ticketKey, "ticket:")).
					Save(ctx)
				if err != nil {
					t.Fatalf("create source sync issue: %v", err)
				}
			}

			queryCtx := ctx
			if tc.principalComplete {
				queryCtx = WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
					PrincipalKey:                 "principal:coverage-owner",
					AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
					CoverageCompleteForPrincipal: true,
				})
			}
			depth := 1
			limit := 4
			got, err := resolver.BoundedGraphContext(queryCtx, string(ontology.ObjectTicket), tc.ticketKey, tc.associationTypes, &depth, &limit)
			if err != nil {
				t.Fatalf("boundedGraphContext: %v", err)
			}
			if got.Coverage.CoverageState != tc.wantState || got.Coverage.AbsenceClaimsAllowed != tc.wantAllowed || got.Coverage.AbsenceClaimGateReason != tc.wantReason {
				t.Fatalf("coverage = %#v, want state=%s allowed=%v reason=%s", got.Coverage, tc.wantState, tc.wantAllowed, tc.wantReason)
			}
			if tc.wantWindowStart != "" || tc.wantWindowEnd != "" {
				if pointerString(got.Coverage.CoverageWindowStart) != tc.wantWindowStart || pointerString(got.Coverage.CoverageWindowEnd) != tc.wantWindowEnd {
					t.Fatalf("coverage window = %v..%v, want %s..%s", pointerString(got.Coverage.CoverageWindowStart), pointerString(got.Coverage.CoverageWindowEnd), tc.wantWindowStart, tc.wantWindowEnd)
				}
			}
			if tc.sourceSyncIssue {
				summary := pointerString(got.Coverage.Summary)
				for _, forbidden := range []string{"429", "token=secret", "raw status"} {
					if strings.Contains(summary, forbidden) {
						t.Fatalf("coverage summary leaked raw source issue detail %q: %q", forbidden, summary)
					}
				}
				if !strings.Contains(summary, "coverage evidence only") {
					t.Fatalf("coverage summary = %q, want replay/coverage evidence caveat", summary)
				}
			}
		})
	}
}

func TestBoundedGraphContextUsesSourceScopeCoverageForNonTicketStarts(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name            string
		objectType      domain.ObjectType
		key             string
		associationType domain.AssociationType
	}{
		{
			name:            "pull request author",
			objectType:      ontology.ObjectPullRequest,
			key:             "pull-request:company/app#42",
			associationType: domain.AssociationType("author"),
		},
		{
			name:            "document links to",
			objectType:      ontology.ObjectDocument,
			key:             "document:launch-plan",
			associationType: domain.AssociationType("links_to"),
		},
		{
			name:            "message discussed in",
			objectType:      ontology.ObjectMessage,
			key:             "message:launch-thread",
			associationType: ontology.AssocDiscussedIn,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, resolver := newBoundedGraphEntResolver(t, ctx)
			defer store.Close()
			seedBoundedGraphCoverageScopedStart(t, ctx, store.Client(), boundedGraphCoverageStartFixture{
				objectType:    tc.objectType,
				key:           tc.key,
				association:   tc.associationType,
				withRunWindow: true,
			})
			queryCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
				PrincipalKey:                 "principal:coverage-owner",
				AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
				CoverageCompleteForPrincipal: true,
			})

			depth := 1
			limit := 4
			got, err := resolver.BoundedGraphContext(queryCtx, string(tc.objectType), tc.key, []string{string(tc.associationType)}, &depth, &limit)
			if err != nil {
				t.Fatalf("boundedGraphContext: %v", err)
			}

			if got.Seed.ObjectType != string(tc.objectType) || got.Seed.Key != tc.key {
				t.Fatalf("seed = %#v, want %s %s", got.Seed, tc.objectType, tc.key)
			}
			if got.Coverage.CoverageState != "complete" || !got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "complete_relation_path_coverage" {
				t.Fatalf("coverage = %#v, want complete scoped coverage for %s", got.Coverage, tc.objectType)
			}
			if strings.Join(got.Coverage.AbsenceClaimAssociationTypes, ",") != string(tc.associationType) {
				t.Fatalf("association coverage = %#v, want %s", got.Coverage.AbsenceClaimAssociationTypes, tc.associationType)
			}
			if pointerString(got.Coverage.SourceSystem) != sourceSystemForCoverageStart(tc.objectType) || pointerString(got.Coverage.SourceInstance) != sourceInstanceForCoverageStart(tc.objectType) {
				t.Fatalf("coverage source = %v/%v, want start-specific source namespace", pointerString(got.Coverage.SourceSystem), pointerString(got.Coverage.SourceInstance))
			}
			serialized := boundedGraphContextSearchText(got)
			for _, forbidden := range []string{"source_scope", "source_sync_run", "source_sync_issue", "SourceScopeState"} {
				if strings.Contains(serialized, forbidden) {
					t.Fatalf("bounded context leaked source diagnostic %q in %s", forbidden, serialized)
				}
			}
		})
	}
}

func TestBoundedGraphContextDoesNotLetGitHubPRScopeProveJiraTicketLinks(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()
	seedBoundedGraphCoverageScopedStart(t, ctx, store.Client(), boundedGraphCoverageStartFixture{
		objectType:    ontology.ObjectPullRequest,
		key:           "pull-request:company/app#43",
		association:   ontology.AssocImplementedBy,
		withRunWindow: true,
	})
	queryCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey:                 "principal:coverage-owner",
		AllowedVisibilityClasses:     []string{"private", "restricted", "team"},
		CoverageCompleteForPrincipal: true,
	})

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(queryCtx, string(ontology.ObjectPullRequest), "pull-request:company/app#43", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	if got.Coverage.CoverageState != "complete" {
		t.Fatalf("coverage state = %q, want complete GitHub PR scope", got.Coverage.CoverageState)
	}
	if got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_scope_relation_coverage_unsupported" {
		t.Fatalf("coverage = %#v, want GitHub PR scope unable to prove Jira ticket-link absence", got.Coverage)
	}
	if len(got.Coverage.AbsenceClaimAssociationTypes) != 0 {
		t.Fatalf("association coverage = %#v, want unsupported implemented_by filtered out", got.Coverage.AbsenceClaimAssociationTypes)
	}
}

func TestBoundedGraphContextAssociationTypeFilterPreventsHubBleed(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	for _, object := range []domain.Object{
		{
			ObjectType:     "ticket",
			Key:            "ticket:COMP-101",
			Title:          "Launch checklist",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "pull_request",
			Key:            "pull-request:company/app#42",
			Title:          "Launch checklist API",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "person",
			Key:            "person:alice",
			Title:          "Alice",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "ticket",
			Key:            "ticket:COMP-999",
			Title:          "Unrelated finance export",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	for _, association := range []domain.Association{
		{
			From:            domain.ObjectRef{ObjectType: "ticket", Key: "ticket:COMP-101"},
			To:              domain.ObjectRef{ObjectType: "pull_request", Key: "pull-request:company/app#42"},
			AssociationType: "implemented_by",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:launch-ticket-pr",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "implemented_by",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               domain.VisibilityPublic,
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			From:            domain.ObjectRef{ObjectType: "ticket", Key: "ticket:COMP-101"},
			To:              domain.ObjectRef{ObjectType: "person", Key: "person:alice"},
			AssociationType: "assignee",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:launch-assignee",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "assignee",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               domain.VisibilityPublic,
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			From:            domain.ObjectRef{ObjectType: "person", Key: "person:alice"},
			To:              domain.ObjectRef{ObjectType: "ticket", Key: "ticket:COMP-999"},
			AssociationType: "assignee",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:finance-assignee",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "assignee",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               domain.VisibilityPublic,
				FreshnessState:           domain.FreshnessFresh,
			},
		},
	} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.AssociationType, err)
		}
	}

	depth := 2
	limit := 8
	resolver := (&Resolver{GraphExpander: store}).Query().(*queryResolver)
	unfiltered, err := resolver.BoundedGraphContext(ctx, "ticket", "ticket:COMP-101", nil, &depth, &limit)
	if err != nil {
		t.Fatalf("unfiltered boundedGraphContext: %v", err)
	}
	if !strings.Contains(boundedGraphContextSearchText(unfiltered), "ticket:COMP-999") {
		t.Fatalf("unfiltered context = %s, want high-degree hub to demonstrate bleed risk", boundedGraphContextSearchText(unfiltered))
	}

	filtered, err := resolver.BoundedGraphContext(ctx, "ticket", "ticket:COMP-101", []string{"implemented_by"}, &depth, &limit)
	if err != nil {
		t.Fatalf("filtered boundedGraphContext: %v", err)
	}
	serialized := boundedGraphContextSearchText(filtered)
	if !strings.Contains(serialized, "pull-request:company/app#42") || !strings.Contains(serialized, "implemented_by") {
		t.Fatalf("filtered context = %s, want implemented_by PR path", serialized)
	}
	for _, forbidden := range []string{"person:alice", "ticket:COMP-999", "assignee", "finance"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("filtered context leaked %q in %s", forbidden, serialized)
		}
	}
}

func TestBoundedGraphContextUsesPrincipalReadFilterBeforeTraversal(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	for _, object := range []domain.Object{
		{
			ObjectType:     "document",
			Key:            "doc:public-seed",
			Title:          "Public seed",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "message",
			Key:            "message:public-direct",
			Title:          "Public direct message",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "person",
			Key:            "person:secret-alice",
			Title:          "Secret Alice",
			Visibility:     "private",
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "ticket",
			Key:            "ticket:public-through-private-hub",
			Title:          "Public descendant through private hub",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	for _, association := range []domain.Association{
		{
			Key:             "a-private-hub",
			From:            domain.ObjectRef{ObjectType: "document", Key: "doc:public-seed"},
			To:              domain.ObjectRef{ObjectType: "person", Key: "person:secret-alice"},
			AssociationType: "mentions",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:private-hub",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "mentions",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               "private",
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			Key:             "z-public-direct",
			From:            domain.ObjectRef{ObjectType: "document", Key: "doc:public-seed"},
			To:              domain.ObjectRef{ObjectType: "message", Key: "message:public-direct"},
			AssociationType: "mentions",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:public-direct",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "mentions",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               domain.VisibilityPublic,
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			Key:             "private-hub-to-public-descendant",
			From:            domain.ObjectRef{ObjectType: "person", Key: "person:secret-alice"},
			To:              domain.ObjectRef{ObjectType: "ticket", Key: "ticket:public-through-private-hub"},
			AssociationType: "assignee",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:private-descendant",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "assignee",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               "private",
				FreshnessState:           domain.FreshnessFresh,
			},
		},
	} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	depth := 2
	limit := 1
	resolver := (&Resolver{GraphExpander: store}).Query().(*queryResolver)
	publicOnlyCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey: "user:bob",
	})
	publicOnly, err := resolver.BoundedGraphContext(publicOnlyCtx, "document", "doc:public-seed", nil, &depth, &limit)
	if err != nil {
		t.Fatalf("public-only boundedGraphContext: %v", err)
	}
	publicText := boundedGraphContextSearchText(publicOnly)
	if !strings.Contains(publicText, "message:public-direct") || !strings.Contains(publicText, "z-public-direct") {
		t.Fatalf("public-only context = %s, want public edge after private edge is skipped", publicText)
	}
	for _, forbidden := range []string{"person:secret-alice", "ticket:public-through-private-hub", "Secret Alice", "private-hub"} {
		if strings.Contains(publicText, forbidden) {
			t.Fatalf("public-only context leaked %q in %s", forbidden, publicText)
		}
	}

	privateAllowedCtx := WithBoundedGraphPrincipalAccess(ctx, BoundedGraphPrincipalAccess{
		PrincipalKey:             "user:alice",
		GroupKeys:                []string{"group:launch"},
		AllowedVisibilityClasses: []string{"private"},
	})
	privateAllowed, err := resolver.BoundedGraphContext(privateAllowedCtx, "document", "doc:public-seed", nil, &depth, &limit)
	if err != nil {
		t.Fatalf("private-allowed boundedGraphContext: %v", err)
	}
	privateText := boundedGraphContextSearchText(privateAllowed)
	for _, expected := range []string{"person:secret-alice", "ticket:public-through-private-hub", "a-private-hub", "private-hub-to-public-descendant"} {
		if !strings.Contains(privateText, expected) {
			t.Fatalf("private-allowed context missing %q in %s", expected, privateText)
		}
	}
}

func TestBoundedGraphContextQuarantinesNonProductWriters(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: t.TempDir() + "/ontology.db",
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	seedBoundedGraphNonProductWriterRows(t, ctx, store.Client())

	depth := 1
	limit := 4
	resolver := (&Resolver{
		GraphExpander: entgraph.NewProductExpander(store.Client()),
	}).Query().(*queryResolver)
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:RAW-1", []string{string(ontology.AssocDiscussedIn)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	serialized := boundedGraphContextSearchText(got)
	for _, forbidden := range []string{
		"RAW_COMMENT_SHOULD_NOT_REACH_PROMPT",
		"GENERATED_BRIEF_SHOULD_NOT_REACH_PROMPT",
		"work-insight:generated-summary-sentinel",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("bounded context leaked %q in %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "message:raw-comment-1") || !strings.Contains(serialized, "ticket:RAW-1") {
		t.Fatalf("bounded context = %s, want stable ticket/message keys", serialized)
	}
	for _, object := range got.Objects {
		if object.ObjectType == string(ontology.ObjectMessage) && object.Title != "message:raw-comment-1" {
			t.Fatalf("message object title = %q, want stable key", object.Title)
		}
	}
}

func TestBoundedGraphContextGatesPartialEndpointRelationship(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: t.TempDir() + "/ontology.db",
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	seedBoundedGraphPartialPullRequestRows(t, ctx, store.Client())

	depth := 1
	limit := 4
	resolver := (&Resolver{
		GraphExpander: entgraph.NewProductExpander(store.Client()),
	}).Query().(*queryResolver)
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:PARTIAL-1", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	implementedBy := associationByType(got.Associations, string(ontology.AssocImplementedBy))
	if implementedBy == nil {
		t.Fatalf("associations = %#v, want implemented_by relationship visible for validation", got.Associations)
	}
	if implementedBy.ClaimAllowed || implementedBy.ClaimGateReason != "relationship_endpoint_partial_requires_hydration" {
		t.Fatalf("implemented_by claim = allowed:%v gate:%q, want endpoint hydration gate", implementedBy.ClaimAllowed, implementedBy.ClaimGateReason)
	}
	if implementedBy.ProofState != "candidate" {
		t.Fatalf("implemented_by proofState = %q, want candidate", implementedBy.ProofState)
	}
	prObject := objectByKey(got.Objects, "pull-request:repo/example#101")
	if prObject == nil {
		t.Fatalf("objects = %#v, want partial PR object visible for validation", got.Objects)
	}
	if prObject.ClaimAllowed || prObject.ClaimGateReason != "object_partial_requires_hydration" {
		t.Fatalf("partial PR object claim = allowed:%v gate:%q, want hydration gate", prObject.ClaimAllowed, prObject.ClaimGateReason)
	}
}

func TestBoundedGraphContextGatesLatestEvidenceWhenOlderEvidenceIsHidden(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: t.TempDir() + "/ontology.db",
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	seedBoundedGraphMultiEvidenceRelationshipRows(t, ctx, store.Client())

	depth := 1
	limit := 4
	resolver := (&Resolver{
		GraphExpander: entgraph.NewProductExpander(store.Client()),
	}).Query().(*queryResolver)
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:MULTI-EVIDENCE-1", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	implementedBy := associationByType(got.Associations, string(ontology.AssocImplementedBy))
	if implementedBy == nil {
		t.Fatalf("associations = %#v, want implemented_by relationship visible for validation", got.Associations)
	}
	if implementedBy.ClaimAllowed || implementedBy.ClaimGateReason != "relationship_multi_evidence_requires_review" {
		t.Fatalf("implemented_by claim = allowed:%v gate:%q, want multi-evidence review gate", implementedBy.ClaimAllowed, implementedBy.ClaimGateReason)
	}
	if implementedBy.ProofState != "candidate" {
		t.Fatalf("implemented_by proofState = %q, want candidate", implementedBy.ProofState)
	}
}

func TestBoundedGraphContextRequiresAuthoritativeEvidenceLocatorKind(t *testing.T) {
	ctx := context.Background()
	store, resolver := newBoundedGraphEntResolver(t, ctx)
	defer store.Close()

	seedBoundedGraphUnauthoritativeEvidenceRelationshipRows(t, ctx, store.Client())

	depth := 1
	limit := 4
	got, err := resolver.BoundedGraphContext(ctx, string(ontology.ObjectTicket), "ticket:AUTHORITY-1", []string{string(ontology.AssocImplementedBy)}, &depth, &limit)
	if err != nil {
		t.Fatalf("boundedGraphContext: %v", err)
	}

	implementedBy := associationByType(got.Associations, string(ontology.AssocImplementedBy))
	if implementedBy == nil {
		t.Fatalf("associations = %#v, want implemented_by relationship visible for validation", got.Associations)
	}
	if implementedBy.ClaimAllowed || implementedBy.ClaimGateReason != "relationship_locator_not_authoritative_for_presence" {
		t.Fatalf("implemented_by claim = allowed:%v gate:%q, want locator-authority gate", implementedBy.ClaimAllowed, implementedBy.ClaimGateReason)
	}
	if implementedBy.ProofState != "candidate" {
		t.Fatalf("implemented_by proofState = %q, want candidate", implementedBy.ProofState)
	}
	relationshipEvidence := evidenceByKey(got.Evidence, "evidence:authority-mismatch")
	if relationshipEvidence == nil || pointerString(relationshipEvidence.Source) != "jira" || pointerString(relationshipEvidence.LocatorKind) != "chat_message" {
		t.Fatalf("relationship evidence = %#v, want resolved jira evidence with chat_message locator", relationshipEvidence)
	}
}

func TestBoundedGraphContextRequiresConfiguredExpander(t *testing.T) {
	ctx := context.Background()
	resolver := (&Resolver{}).Query().(*queryResolver)

	got, err := resolver.BoundedGraphContext(
		ctx,
		"document",
		"doc:architecture-note",
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("boundedGraphContext returned graph %#v, want configuration error", got)
	}
	if !strings.Contains(err.Error(), "configured graph expander") {
		t.Fatalf("error = %q, want configured graph expander", err.Error())
	}
}

type boundedGraphCoverageFixture struct {
	ticketKey         string
	crawlPolicy       string
	withRunWindow     bool
	rowSourceSystem   string
	rowSourceInstance string
	freshnessState    sourcescopestate.FreshnessState
	coverageMode      sourcescopestate.CoverageMode
}

type boundedGraphCoverageStartFixture struct {
	objectType        domain.ObjectType
	key               string
	association       domain.AssociationType
	crawlPolicy       string
	withRunWindow     bool
	rowSourceSystem   string
	rowSourceInstance string
	stateFreshness    sourcescopestate.FreshnessState
	stateCoverageMode sourcescopestate.CoverageMode
}

type boundedGraphCoverageRows struct {
	scope *genent.SourceScope
	run   *genent.SourceSyncRun
}

func newBoundedGraphEntResolver(t *testing.T, ctx context.Context) (*entstore.Store, *queryResolver) {
	t.Helper()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: t.TempDir() + "/ontology.db",
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	resolver := (&Resolver{
		EntClient:     store.Client(),
		GraphExpander: entgraph.NewProductExpander(store.Client()),
	}).Query().(*queryResolver)
	return store, resolver
}

func seedBoundedGraphCoverageScopedTicket(t *testing.T, ctx context.Context, client *genent.Client, fixture boundedGraphCoverageFixture) boundedGraphCoverageRows {
	return seedBoundedGraphCoverageScopedStart(t, ctx, client, boundedGraphCoverageStartFixture{
		objectType:        ontology.ObjectTicket,
		key:               fixture.ticketKey,
		association:       ontology.AssocImplementedBy,
		crawlPolicy:       fixture.crawlPolicy,
		withRunWindow:     fixture.withRunWindow,
		rowSourceSystem:   fixture.rowSourceSystem,
		rowSourceInstance: fixture.rowSourceInstance,
		stateFreshness:    fixture.freshnessState,
		stateCoverageMode: fixture.coverageMode,
	})
}

func seedBoundedGraphCoverageScopedStart(t *testing.T, ctx context.Context, client *genent.Client, fixture boundedGraphCoverageStartFixture) boundedGraphCoverageRows {
	t.Helper()
	coverageStart := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	coverageEnd := time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC)
	sourceSystem := sourceSystemForCoverageStart(fixture.objectType)
	sourceInstance := sourceInstanceForCoverageStart(fixture.objectType)
	rowSourceSystem := fixture.rowSourceSystem
	if rowSourceSystem == "" {
		rowSourceSystem = sourceSystem
	}
	rowSourceInstance := fixture.rowSourceInstance
	if rowSourceInstance == "" {
		rowSourceInstance = sourceInstance
	}
	stateFreshness := fixture.stateFreshness
	if stateFreshness == "" {
		stateFreshness = sourcescopestate.FreshnessStateFresh
	}
	stateCoverageMode := fixture.stateCoverageMode
	if stateCoverageMode == "" {
		stateCoverageMode = sourcescopestate.CoverageModeExactScope
	}
	crawlPolicy := fixture.crawlPolicy
	if crawlPolicy == "" {
		crawlPolicy = "bounded_graph_absence=" + string(fixture.association)
	}
	connection, err := client.SourceConnection.Create().
		SetKey("source-connection:" + fixture.key).
		SetSourceSystem(sourceSystem).
		SetSourceInstance(sourceInstance).
		SetDisplayName(sourceSystem + " fixture").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	scope, err := client.SourceScope.Create().
		SetKey("source-scope:" + fixture.key).
		SetConnection(connection).
		SetScopeKind(scopeKindForCoverageStart(fixture.objectType)).
		SetScopeKey(scopeKeyForCoverageStart(fixture.objectType)).
		SetDisplayName(scopeKeyForCoverageStart(fixture.objectType)).
		SetCrawlPolicy(crawlPolicy).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source scope: %v", err)
	}
	runBuilder := client.SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("source-sync-run:" + fixture.key).
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModeExactScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(coverageStart.Add(-time.Minute)).
		SetCompletedAt(coverageEnd.Add(time.Minute))
	if fixture.withRunWindow {
		runBuilder.SetCoverageStartAt(coverageStart).SetCoverageEndAt(coverageEnd)
	}
	run, err := runBuilder.Save(ctx)
	if err != nil {
		t.Fatalf("create source sync run: %v", err)
	}
	state, err := client.SourceScopeState.Create().
		SetScope(scope).
		SetFreshnessState(stateFreshness).
		SetCoverageMode(stateCoverageMode).
		SetLastSuccessfulSyncRun(run).
		SetLastSuccessfulAt(coverageEnd).
		SetLastAttemptedAt(coverageEnd).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source scope state: %v", err)
	}
	seedBoundedGraphCoverageStartObject(t, ctx, client, fixture.objectType, fixture.key, rowSourceSystem, rowSourceInstance, state.ID, coverageEnd)
	return boundedGraphCoverageRows{scope: scope, run: run}
}

func seedBoundedGraphCoverageStartObject(t *testing.T, ctx context.Context, client *genent.Client, objectType domain.ObjectType, key string, sourceSystem string, sourceInstance string, sourceScopeStateID int, observedAt time.Time) {
	t.Helper()
	switch objectType {
	case ontology.ObjectTicket:
		externalID := strings.TrimPrefix(key, "ticket:")
		_, err := client.Ticket.Create().
			SetKey(key).
			SetTitle("Coverage-scoped ticket").
			SetStatus(ticket.StatusOpen).
			SetSourceSystem(sourceSystem).
			SetSourceInstance(sourceInstance).
			SetExternalKind("jira_issue").
			SetExternalID(externalID).
			SetFreshnessState(ticket.FreshnessStateFresh).
			SetVisibility(ticket.VisibilityPublic).
			SetSourceScopeStateID(sourceScopeStateID).
			SetLastConfirmedAt(observedAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create ticket: %v", err)
		}
	case ontology.ObjectPullRequest:
		_, err := client.PullRequest.Create().
			SetKey(key).
			SetRepository("company/app").
			SetNumber(42).
			SetTitle("Coverage-scoped pull request").
			SetState(pullrequest.StateOpen).
			SetSourceSystem(sourceSystem).
			SetSourceInstance(sourceInstance).
			SetExternalKind("github_pull_request").
			SetExternalID("company/app#42").
			SetFreshnessState(pullrequest.FreshnessStateFresh).
			SetVisibility(pullrequest.VisibilityPublic).
			SetSourceScopeStateID(sourceScopeStateID).
			SetLastConfirmedAt(observedAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create pull request: %v", err)
		}
	case ontology.ObjectDocument:
		_, err := client.Document.Create().
			SetKey(key).
			SetTitle("Coverage-scoped document").
			SetDocumentKind(document.DocumentKindMarkdown).
			SetSourceSystem(sourceSystem).
			SetSourceInstance(sourceInstance).
			SetExternalKind("markdown").
			SetExternalID(strings.TrimPrefix(key, "document:") + ".md").
			SetFreshnessState(document.FreshnessStateFresh).
			SetVisibility(document.VisibilityPublic).
			SetSourceScopeStateID(sourceScopeStateID).
			SetLastConfirmedAt(observedAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create document: %v", err)
		}
	case ontology.ObjectMessage:
		_, err := client.Message.Create().
			SetKey(key).
			SetSummary("Coverage-scoped message").
			SetBody("Coverage-scoped launch message.").
			SetChannelKey("launch").
			SetThreadKey(strings.TrimPrefix(key, "message:")).
			SetSourceSystem(sourceSystem).
			SetSourceInstance(sourceInstance).
			SetExternalKind("chat_message").
			SetExternalID(strings.TrimPrefix(key, "message:")).
			SetFreshnessState(message.FreshnessStateFresh).
			SetVisibility(message.VisibilityPublic).
			SetSourceScopeStateID(sourceScopeStateID).
			SetLastConfirmedAt(observedAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create message: %v", err)
		}
	default:
		t.Fatalf("unsupported coverage start type %s", objectType)
	}
}

func sourceSystemForCoverageStart(objectType domain.ObjectType) string {
	switch objectType {
	case ontology.ObjectPullRequest:
		return "github"
	case ontology.ObjectDocument:
		return "docs"
	case ontology.ObjectMessage:
		return "chat"
	default:
		return "jira"
	}
}

func sourceInstanceForCoverageStart(objectType domain.ObjectType) string {
	switch objectType {
	case ontology.ObjectPullRequest:
		return "company/app"
	case ontology.ObjectDocument:
		return "company-docs"
	case ontology.ObjectMessage:
		return "company-chat"
	default:
		return "company"
	}
}

func scopeKindForCoverageStart(objectType domain.ObjectType) string {
	switch objectType {
	case ontology.ObjectPullRequest:
		return "repository"
	case ontology.ObjectDocument:
		return "folder"
	case ontology.ObjectMessage:
		return "channel"
	default:
		return "project"
	}
}

func scopeKeyForCoverageStart(objectType domain.ObjectType) string {
	switch objectType {
	case ontology.ObjectPullRequest:
		return "company/app"
	case ontology.ObjectDocument:
		return "launch-docs"
	case ontology.ObjectMessage:
		return "launch"
	default:
		return "COMP"
	}
}

func seedBoundedGraphMultiEvidenceRelationshipRows(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 16, 30, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:MULTI-EVIDENCE-1").
		SetTitle("Multi-evidence ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("MULTI-EVIDENCE-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#303").
		SetRepository("repo/example").
		SetNumber(303).
		SetTitle("Current PR endpoint").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("github.com/repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#303").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetConfidence(1).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	rel := client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("MULTI-EVIDENCE-1->repo/example#303").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	currentEvidence := client.Evidence.Create().
		SetKey("evidence:multi-current-ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetClaimTargetID(rel.ID).
		SetRelationshipKind(string(ontology.AssocImplementedBy)).
		SetRelationshipID(rel.ID).
		SetLocatorKind("jira_remote_link").
		SetLocator("MULTI-EVIDENCE-1 current remote link").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("MULTI-EVIDENCE-1->repo/example#303/current").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.Evidence.Create().
		SetKey("evidence:multi-stale-ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetClaimTargetID(rel.ID).
		SetRelationshipKind(string(ontology.AssocImplementedBy)).
		SetRelationshipID(rel.ID).
		SetLocatorKind("jira_remote_link").
		SetLocator("MULTI-EVIDENCE-1 stale remote link").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("MULTI-EVIDENCE-1->repo/example#303/stale").
		SetProofState(evidence.ProofStateStale).
		SetFreshnessState(evidence.FreshnessStateStale).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt.Add(-24 * time.Hour)).
		SaveX(ctx)
	rel.Update().
		SetLatestEvidence(currentEvidence).
		SetEvidenceCount(2).
		SaveX(ctx)
}

func seedBoundedGraphUnauthoritativeEvidenceRelationshipRows(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 16, 45, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:AUTHORITY-1").
		SetTitle("Source authority ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("AUTHORITY-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#404").
		SetRepository("repo/example").
		SetNumber(404).
		SetTitle("Source authority PR endpoint").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("github.com/repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#404").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetConfidence(1).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	rel := client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("AUTHORITY-1->repo/example#404").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	unauthoritativeEvidence := client.Evidence.Create().
		SetKey("evidence:authority-mismatch").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetClaimTargetID(rel.ID).
		SetRelationshipKind(string(ontology.AssocImplementedBy)).
		SetRelationshipID(rel.ID).
		SetLocatorKind("chat_message").
		SetLocator("AUTHORITY-1 chat mention").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("chat_message").
		SetExternalID("AUTHORITY-1 chat mention").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	rel.Update().
		SetLatestEvidence(unauthoritativeEvidence).
		SetEvidenceCount(1).
		SaveX(ctx)
}

func seedBoundedGraphPartialPullRequestRows(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 16, 0, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:PARTIAL-1").
		SetTitle("Partial PR endpoint ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("PARTIAL-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#101").
		SetRepository("repo/example").
		SetNumber(101).
		SetTitle("repo/example#101").
		SetState(pullrequest.StateUnknown).
		SetSourceSystem("github").
		SetSourceInstance("github.com/repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#101").
		SetFreshnessState(pullrequest.FreshnessStatePartial).
		SetVisibility(pullrequest.VisibilityPublic).
		SetConfidence(0.6).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	relationshipEvidence := client.Evidence.Create().
		SetKey("evidence:partial-ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetRelationshipKind(string(ontology.AssocImplementedBy)).
		SetLocatorKind("jira_remote_link").
		SetLocator("PARTIAL-1 remote link").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("PARTIAL-1->repo/example#101").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("PARTIAL-1->repo/example#101").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(relationshipEvidence).
		SaveX(ctx)
}

func seedBoundedGraphNonProductWriterRows(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 15, 0, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:RAW-1").
		SetTitle("Raw comment quarantine ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("RAW-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	messageRow := client.Message.Create().
		SetKey("message:raw-comment-1").
		SetBody("RAW_COMMENT_SHOULD_NOT_REACH_PROMPT body").
		SetSummary("RAW_COMMENT_SHOULD_NOT_REACH_PROMPT summary").
		SetChannelKey("jira-comments").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_comment").
		SetExternalID("RAW-1/comment-1").
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	messageEvidence := client.Evidence.Create().
		SetKey("evidence:raw-ticket-message").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_message").
		SetRelationshipKind(string(ontology.AssocDiscussedIn)).
		SetLocatorKind("jira_comment").
		SetLocator("RAW-1/comment-1").
		SetExcerpt("RAW_COMMENT_SHOULD_NOT_REACH_PROMPT evidence excerpt").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_comment").
		SetExternalID("RAW-1/comment-1").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketMessage.Create().
		SetTicket(ticketRow).
		SetMessage(messageRow).
		SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_comment").
		SetExternalID("RAW-1->comment-1").
		SetFreshnessState(ticketmessage.FreshnessStateFresh).
		SetVisibility(ticketmessage.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(messageEvidence).
		SaveX(ctx)
	client.WorkInsight.Create().
		SetKey("work-insight:generated-summary-sentinel").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("ticket:RAW-1").
		SetTitle("GENERATED_BRIEF_SHOULD_NOT_REACH_PROMPT title").
		SetDetails("GENERATED_BRIEF_SHOULD_NOT_REACH_PROMPT details").
		SetRecommendedAction("GENERATED_BRIEF_SHOULD_NOT_REACH_PROMPT action").
		SetModelMethod("bounded_graph_context_to_cited_brief:generic").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("example").
		SetExternalKind("tpm_insight").
		SetExternalID("generated-summary-sentinel").
		SetSourceURL("cubicle://graph-brief/generic/sentinel").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetVisibility(workinsight.VisibilityPublic).
		SetRankScore(1000).
		SetLastActivityAt(observedAt).
		SaveX(ctx)
}

func associationByType(values []*model.BoundedGraphAssociation, associationType string) *model.BoundedGraphAssociation {
	for _, value := range values {
		if value.AssociationType == associationType {
			return value
		}
	}
	return nil
}

func evidenceByKey(values []*model.BoundedGraphEvidence, key string) *model.BoundedGraphEvidence {
	for _, value := range values {
		if value.Key == key {
			return value
		}
	}
	return nil
}

func objectByKey(values []*model.BoundedGraphObject, key string) *model.BoundedGraphObject {
	for _, value := range values {
		if value.Key == key {
			return value
		}
	}
	return nil
}

func boundedGraphContextSearchText(value *model.BoundedGraphContext) string {
	var b strings.Builder
	b.WriteString(value.ScopeMode)
	b.WriteString(" ")
	for _, object := range value.Objects {
		b.WriteString(object.ObjectType)
		b.WriteString(" ")
		b.WriteString(object.Key)
		b.WriteString(" ")
		b.WriteString(object.Title)
		b.WriteString(" ")
	}
	for _, association := range value.Associations {
		b.WriteString(association.AssociationType)
		b.WriteString(" ")
		b.WriteString(association.Key)
		b.WriteString(" ")
	}
	return b.String()
}
