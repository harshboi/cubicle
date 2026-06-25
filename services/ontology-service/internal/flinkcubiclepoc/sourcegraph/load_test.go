// Association:
//
//	fixture snapshots -> LoadFixture -> typed rows + Evidence + SourceSyncIssue
//	failed PR bundle -> SourceSyncIssue, not fake PullRequest truth
//
// These tests protect the split between product graph facts and source coverage.
package sourcegraph

import (
	"context"
	"strconv"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"

	_ "github.com/mattn/go-sqlite3"
)

// TestLoadFixtureMaterializesTypedGraphAndCoverage proves product truth and coverage rows split correctly.
func TestLoadFixtureMaterializesTypedGraphAndCoverage(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:flink-fixture-load?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	records := []sourcecapture.Record{
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_issue", "FLINK-1", []byte(`{
		  "key":"FLINK-1",
		  "fields":{
		    "summary":"Fix autoscaler target utilization",
		    "description":"The autoscaler should converge.",
		    "status":{"name":"Closed"},
		    "priority":{"name":"Major"},
		    "updated":"2026-06-10T15:04:05.000+0000"
		  }
		}`)),
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_remote_links", "FLINK-1", []byte(`[
		  {"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/10"}},
		  {"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/11"}}
		]`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pr_search", "apache/flink-kubernetes-operator:page-1", []byte(`{
		  "items":[
		    {"html_url":"https://github.com/apache/flink-kubernetes-operator/pull/10"},
		    {"html_url":"https://github.com/apache/flink-kubernetes-operator/pull/11"},
		    {"html_url":"https://github.com/apache/flink-kubernetes-operator/pull/12"}
		  ]
		}`)),
	}
	records = append(records, completePRBundleRecords(10)...)
	records = append(records, failedPRBundleRecords(11, 429)...)

	result, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-stream",
		DisplayName: "Test Flink stream",
		RunKey:      "source-sync-run:test-flink-stream",
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("LoadFixture returned error: %v", err)
	}

	if result.RecordsSeen != 15 || result.Records200 != 9 || result.RecordsFailed != 6 {
		t.Fatalf("record coverage = %#v", result)
	}
	if result.DiscoveredPullRequests != 3 {
		t.Fatalf("DiscoveredPullRequests = %d, want 3", result.DiscoveredPullRequests)
	}
	if result.CompletePullRequestBundles != 1 {
		t.Fatalf("CompletePullRequestBundles = %d, want 1", result.CompletePullRequestBundles)
	}
	if result.Tickets != 1 || result.PullRequests != 2 || result.TicketPullRequests != 2 {
		t.Fatalf("product counts = %#v", result)
	}
	if result.Evidence != 6 || result.SyncIssues != 6 || result.WorkLensWindows != 2 {
		t.Fatalf("evidence/coverage/window counts = %#v", result)
	}

	hydratedPR := client.PullRequest.Query().
		Where(pullrequest.KeyEQ("pull-request:github:apache/flink-kubernetes-operator#10")).
		OnlyX(ctx)
	if hydratedPR.Additions == nil || *hydratedPR.Additions != 12 {
		t.Fatalf("hydrated PR additions = %v, want 12", hydratedPR.Additions)
	}
	if hydratedPR.Deletions == nil || *hydratedPR.Deletions != 3 {
		t.Fatalf("hydrated PR deletions = %v, want 3", hydratedPR.Deletions)
	}
	if hydratedPR.ChangedFilesCount == nil || *hydratedPR.ChangedFilesCount != 2 {
		t.Fatalf("hydrated PR changed files = %v, want 2", hydratedPR.ChangedFilesCount)
	}
	if hydratedPR.CommitCount == nil || *hydratedPR.CommitCount != 4 {
		t.Fatalf("hydrated PR commit count = %v, want 4", hydratedPR.CommitCount)
	}
	if hydratedPR.IssueCommentCount == nil || *hydratedPR.IssueCommentCount != 5 {
		t.Fatalf("hydrated PR issue comment count = %v, want 5", hydratedPR.IssueCommentCount)
	}
	if hydratedPR.ReviewCommentCount == nil || *hydratedPR.ReviewCommentCount != 6 {
		t.Fatalf("hydrated PR review comment count = %v, want 6", hydratedPR.ReviewCommentCount)
	}
	if hydratedPR.IsDraft == nil || *hydratedPR.IsDraft {
		t.Fatalf("hydrated PR draft = %v, want false", hydratedPR.IsDraft)
	}
	if hydratedPR.IsMergeable == nil || !*hydratedPR.IsMergeable {
		t.Fatalf("hydrated PR mergeable = %v, want true", hydratedPR.IsMergeable)
	}
	hydratedRel := client.TicketPullRequest.Query().
		Where(ticketpullrequest.HasPullRequestWith(pullrequest.KeyEQ("pull-request:github:apache/flink-kubernetes-operator#10"))).
		OnlyX(ctx)
	if hydratedRel.EvidenceCount != 2 {
		t.Fatalf("hydrated relationship evidence count = %d, want 2 independent evidence rows", hydratedRel.EvidenceCount)
	}

	assertCount(t, "tickets", client.Ticket.Query().CountX(ctx), 1)
	assertCount(t, "pull requests", client.PullRequest.Query().CountX(ctx), 2)
	assertCount(t, "ticket pull requests", client.TicketPullRequest.Query().CountX(ctx), 2)
	assertCount(t, "evidence", client.Evidence.Query().CountX(ctx), 6)
	assertCount(t, "sync issues", client.SourceSyncIssue.Query().CountX(ctx), 6)
	assertCount(t, "work lens windows", client.WorkLensWindow.Query().CountX(ctx), 2)
	assertCount(t, "pull request lens results", client.PullRequestLensResult.Query().CountX(ctx), 2)
	assertCount(t, "ticket lens results", client.TicketLensResult.Query().CountX(ctx), 1)
	assertCount(t, "source connections", client.SourceConnection.Query().CountX(ctx), 3)
	assertCount(t, "source scopes", client.SourceScope.Query().CountX(ctx), 3)
	assertCount(t, "source scope states", client.SourceScopeState.Query().CountX(ctx), 2)

	loadedTicket := client.Ticket.Query().
		Where(ticket.KeyEQ("ticket:jira:FLINK-1")).
		OnlyX(ctx)
	if loadedTicket.SourceScopeStateID == 0 {
		t.Fatalf("ticket source scope state ID = 0, want Jira source scope provenance")
	}
	assertSourceScopeState(t, ctx, client, loadedTicket.SourceScopeStateID, SourceJira, SourceInstanceJira, now)
	if hydratedPR.SourceScopeStateID == 0 {
		t.Fatalf("hydrated PR source scope state ID = 0, want GitHub source scope provenance")
	}
	assertSourceScopeState(t, ctx, client, hydratedPR.SourceScopeStateID, SourceGitHub, SourceInstanceGitHub, now)
	if hydratedRel.SourceScopeStateID == 0 {
		t.Fatalf("ticket-PR relationship source scope state ID = 0, want source-specific relationship provenance")
	}
	assertSourceScopeState(t, ctx, client, hydratedRel.SourceScopeStateID, SourceJira, SourceInstanceJira, now)

	remoteOnlyPR := client.PullRequest.Query().
		Where(pullrequest.KeyEQ("pull-request:github:apache/flink-kubernetes-operator#11")).
		OnlyX(ctx)
	if remoteOnlyPR.State != pullrequest.StateUnknown {
		t.Fatalf("remote-only PR state = %s, want unknown", remoteOnlyPR.State)
	}
	if remoteOnlyPR.FreshnessState != pullrequest.FreshnessStatePartial {
		t.Fatalf("remote-only PR freshness = %s, want partial", remoteOnlyPR.FreshnessState)
	}
	if remoteOnlyPR.Title != "apache/flink-kubernetes-operator#11" {
		t.Fatalf("remote-only PR title = %q", remoteOnlyPR.Title)
	}
	if remoteOnlyPR.Additions != nil || remoteOnlyPR.IssueCommentCount != nil || remoteOnlyPR.IsMergeable != nil {
		t.Fatalf("remote-only PR metrics = additions %v comments %v mergeable %v, want unset", remoteOnlyPR.Additions, remoteOnlyPR.IssueCommentCount, remoteOnlyPR.IsMergeable)
	}
	if remoteOnlyPR.SourceScopeStateID == 0 {
		t.Fatalf("remote-only PR source scope state ID = 0, want attempted GitHub source scope provenance")
	}
	assertSourceScopeState(t, ctx, client, remoteOnlyPR.SourceScopeStateID, SourceGitHub, SourceInstanceGitHub, now)

	if exists, err := client.PullRequest.Query().
		Where(pullrequest.KeyEQ("pull-request:github:apache/flink-kubernetes-operator#12")).
		Exist(ctx); err != nil || exists {
		t.Fatalf("search-only PR materialized = %v, err = %v; want no product row", exists, err)
	}

	run := client.SourceSyncRun.Query().OnlyX(ctx)
	if run.Status.String() != "partial" || run.CoverageMode.String() != "partial_scope" {
		t.Fatalf("sync run coverage = status %s mode %s", run.Status, run.CoverageMode)
	}
	if run.ObjectsSeenCount != 4 {
		t.Fatalf("ObjectsSeenCount = %d, want discovered PRs plus tickets (4)", run.ObjectsSeenCount)
	}
	if run.ObjectsCreatedCount != 3 || run.RelationshipsCreatedCount != 5 || run.EvidenceCreatedCount != 6 || run.IssuesCreatedCount != 6 {
		t.Fatalf("sync run counters = %#v", run)
	}
	if run.CoverageStartAt.IsZero() || run.CoverageEndAt.IsZero() {
		t.Fatalf("sync run coverage window = %s..%s, want replay fetch window", run.CoverageStartAt, run.CoverageEndAt)
	}
}

func TestLoadFixtureRepeatedReplayDoesNotInflateCreatedCounters(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:flink-fixture-load-idempotent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	records := []sourcecapture.Record{
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_issue", "FLINK-1", []byte(`{
		  "key":"FLINK-1",
		  "fields":{
		    "summary":"Fix autoscaler target utilization",
		    "description":"The autoscaler should converge.",
		    "status":{"name":"Closed"},
		    "priority":{"name":"Major"},
		    "updated":"2026-06-10T15:04:05.000+0000"
		  }
		}`)),
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_remote_links", "FLINK-1", []byte(`[
		  {"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/10"}}
		]`)),
	}
	records = append(records, completePRBundleRecords(10)...)

	if _, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-idempotent",
		DisplayName: "Test Flink idempotent",
		RunKey:      "source-sync-run:test-flink-idempotent:first",
		Now:         func() time.Time { return now },
	}); err != nil {
		t.Fatalf("first LoadFixture returned error: %v", err)
	}
	evidenceCount := client.Evidence.Query().CountX(ctx)

	if _, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-idempotent",
		DisplayName: "Test Flink idempotent",
		RunKey:      "source-sync-run:test-flink-idempotent:second",
		Now:         func() time.Time { return now.Add(time.Hour) },
	}); err != nil {
		t.Fatalf("second LoadFixture returned error: %v", err)
	}

	assertCount(t, "ticket", client.Ticket.Query().CountX(ctx), 1)
	assertCount(t, "pull_request", client.PullRequest.Query().CountX(ctx), 1)
	assertCount(t, "ticket_pull_request", client.TicketPullRequest.Query().CountX(ctx), 1)
	assertCount(t, "evidence after replay", client.Evidence.Query().CountX(ctx), evidenceCount)

	secondRun := client.SourceSyncRun.Query().
		Where(sourcesyncrun.RunKeyEQ("source-sync-run:test-flink-idempotent:second")).
		OnlyX(ctx)
	if secondRun.ObjectsCreatedCount != 0 ||
		secondRun.RelationshipsCreatedCount != 0 ||
		secondRun.EvidenceCreatedCount != 0 ||
		secondRun.IssuesCreatedCount != 0 {
		t.Fatalf("second run created counters = objects:%d relationships:%d evidence:%d issues:%d, want all zero",
			secondRun.ObjectsCreatedCount,
			secondRun.RelationshipsCreatedCount,
			secondRun.EvidenceCreatedCount,
			secondRun.IssuesCreatedCount,
		)
	}
}

// TestLoadFixtureClearsUnknownPullRequestMetricsOnRefresh proves a later 200
// detail payload can return source metrics to unknown instead of retaining stale
// values from a previous successful snapshot.
func TestLoadFixtureClearsUnknownPullRequestMetricsOnRefresh(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:flink-fixture-load-refresh?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	if _, err := LoadFixture(ctx, client, completePRBundleRecords(10), LoadOptions{
		StreamKey:   "test-flink-refresh",
		DisplayName: "Test Flink refresh",
		RunKey:      "source-sync-run:test-flink-refresh:first",
		Now:         func() time.Time { return now },
	}); err != nil {
		t.Fatalf("first LoadFixture returned error: %v", err)
	}

	records := completePRBundleRecords(10)
	objectID := "apache/flink-kubernetes-operator#10"
	records[0] = testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request", objectID, []byte(`{
	  "html_url":"https://github.com/apache/flink-kubernetes-operator/pull/10",
	  "title":"[FLINK-1] Fix autoscaler target utilization",
	  "state":"closed",
	  "merged_at":"2026-06-10T16:00:00Z",
	  "updated_at":"2026-06-10T16:00:00Z",
	  "number":10,
	  "mergeable":null,
	  "base":{"repo":{"full_name":"apache/flink-kubernetes-operator"}}
	}`))
	if _, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-refresh",
		DisplayName: "Test Flink refresh",
		RunKey:      "source-sync-run:test-flink-refresh:second",
		Now:         func() time.Time { return now.Add(time.Hour) },
	}); err != nil {
		t.Fatalf("second LoadFixture returned error: %v", err)
	}

	refreshedPR := client.PullRequest.Query().
		Where(pullrequest.KeyEQ("pull-request:github:apache/flink-kubernetes-operator#10")).
		OnlyX(ctx)
	if refreshedPR.Additions != nil ||
		refreshedPR.Deletions != nil ||
		refreshedPR.ChangedFilesCount != nil ||
		refreshedPR.CommitCount != nil ||
		refreshedPR.IssueCommentCount != nil ||
		refreshedPR.ReviewCommentCount != nil ||
		refreshedPR.IsDraft != nil ||
		refreshedPR.IsMergeable != nil {
		t.Fatalf("refreshed PR metrics were not cleared: %#v", refreshedPR)
	}
}

func TestLoadFixtureMaterializesCorrelationJiraSnapshots(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:flink-fixture-load-correlation?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Date(2026, 6, 22, 5, 0, 0, 0, time.UTC)
	records := []sourcecapture.Record{
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_correlation_issue", "FLINK-2", []byte(`{
		  "key":"FLINK-2",
		  "fields":{
		    "summary":"Correlate related Jira ownership",
		    "description":"A same-developer correlation ticket.",
		    "status":{"name":"Open"},
		    "priority":{"name":"Major"},
		    "updated":"2026-06-21T15:04:05.000+0000"
		  }
		}`)),
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_correlation_remote_links", "FLINK-2", []byte(`[
		  {"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/10"}}
		]`)),
	}
	records = append(records, completePRBundleRecords(10)...)

	result, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-correlation",
		DisplayName: "Test Flink correlation",
		RunKey:      "source-sync-run:test-flink-correlation",
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("LoadFixture returned error: %v", err)
	}

	if result.Tickets != 1 || result.PullRequests != 1 || result.TicketPullRequests != 1 {
		t.Fatalf("product counts = %#v", result)
	}
	if result.DiscoveredPullRequests != 1 || result.CompletePullRequestBundles != 1 {
		t.Fatalf("discovery/bundle counts = %#v", result)
	}
	assertCount(t, "ticket", client.Ticket.Query().CountX(ctx), 1)
	assertCount(t, "pull_request", client.PullRequest.Query().CountX(ctx), 1)
	assertCount(t, "ticket_pull_request", client.TicketPullRequest.Query().CountX(ctx), 1)
}

func TestLoadFixtureTracksMissingPullRequestBundleSnapshots(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:flink-fixture-load-missing-bundle?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	records := []sourcecapture.Record{
		testSnapshot(200, SourceJira, SourceInstanceJira, "jira_issue", "FLINK-1", []byte(`{
		  "key":"FLINK-1",
		  "fields":{"summary":"Partial PR detail","status":{"name":"Open"},"updated":"2026-06-21T15:04:05.000+0000"}
		}`)),
		completePRBundleRecords(10)[0],
	}

	result, err := LoadFixture(ctx, client, records, LoadOptions{
		StreamKey:   "test-flink-missing-bundle",
		DisplayName: "Test Flink missing bundle",
		RunKey:      "source-sync-run:test-flink-missing-bundle",
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("LoadFixture returned error: %v", err)
	}

	if result.MissingPRBundleSnapshots != 5 || result.SyncIssues != 5 {
		t.Fatalf("missing bundle coverage = %#v", result)
	}
	if result.CompletePullRequestBundles != 0 || result.PullRequests != 1 {
		t.Fatalf("PR counts = %#v", result)
	}
	assertCount(t, "source_sync_issue", client.SourceSyncIssue.Query().CountX(ctx), 5)
}

// completePRBundleRecords creates the six successful snapshots required for full PR trust.
func completePRBundleRecords(number int) []sourcecapture.Record {
	objectID := "apache/flink-kubernetes-operator#" + strconv.Itoa(number)
	return []sourcecapture.Record{
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request", objectID, []byte(`{
		  "html_url":"https://github.com/apache/flink-kubernetes-operator/pull/10",
		  "title":"[FLINK-1] Fix autoscaler target utilization",
		  "state":"closed",
		  "merged_at":"2026-06-10T16:00:00Z",
		  "updated_at":"2026-06-10T16:00:00Z",
		  "number":10,
		  "additions":12,
		  "deletions":3,
		  "changed_files":2,
		  "commits":4,
		  "comments":5,
		  "review_comments":6,
		  "draft":false,
		  "mergeable":true,
		  "base":{"repo":{"full_name":"apache/flink-kubernetes-operator"}}
		}`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request_files", objectID, []byte(`[]`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_issue_comments", objectID, []byte(`[]`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request_review_comments", objectID, []byte(`[]`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request_reviews", objectID, []byte(`[]`)),
		testSnapshot(200, SourceGitHub, SourceInstanceGitHub, "github_pull_request_commits", objectID, []byte(`[]`)),
	}
}

// failedPRBundleRecords creates non-200 PR snapshots that should become sync issues only.
func failedPRBundleRecords(number int, status int) []sourcecapture.Record {
	objectID := "apache/flink-kubernetes-operator#" + strconv.Itoa(number)
	body := []byte(`{"message":"rate limited"}`)
	records := make([]sourcecapture.Record, 0, len(requiredPRBundleTypes))
	for _, objectType := range requiredPRBundleTypes {
		records = append(records, testSnapshot(status, SourceGitHub, SourceInstanceGitHub, objectType, objectID, body))
	}
	return records
}

// testSnapshot builds a replay record with source identity and body hash.
func testSnapshot(status int, sourceKey string, sourceInstance string, objectType string, objectID string, body []byte) sourcecapture.Record {
	sourceURL := "https://example.test/" + objectType + "/" + objectID
	return sourcecapture.Record{
		SnapshotKey:      "snapshot:" + objectType + ":" + objectID + ":" + strconv.Itoa(status),
		SourceKey:        sourceKey,
		SourceInstance:   sourceInstance,
		SourceObjectType: objectType,
		SourceObjectID:   objectID,
		SourceURL:        sourceURL,
		BodySHA256:       sourcecapture.HashBody(body),
		Request: sourcecapture.RequestMetadata{
			Method: "GET",
			URL:    sourceURL,
		},
		Response: sourcecapture.ResponseMetadata{
			StatusCode: status,
			FetchedAt:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
		},
		Body: body,
	}
}

func assertSourceScopeState(t *testing.T, ctx context.Context, client *genent.Client, id int, sourceSystem string, sourceInstance string, attemptedAt time.Time) {
	t.Helper()
	state := client.SourceScopeState.GetX(ctx, id)
	if state.FreshnessState != sourcescopestate.FreshnessStateFresh || state.CoverageMode != sourcescopestate.CoverageModePartialScope {
		t.Fatalf("source scope state %d = freshness %s coverage %s, want fresh partial_scope", id, state.FreshnessState, state.CoverageMode)
	}
	if !state.LastAttemptedAt.Equal(attemptedAt) {
		t.Fatalf("source scope state %d last_attempted_at = %s, want %s", id, state.LastAttemptedAt, attemptedAt)
	}
	if state.LastSuccessfulSyncRunID != 0 || !state.LastSuccessfulAt.IsZero() {
		t.Fatalf("source scope state %d successful run = %d at %s, want unset for partial fixture source scope", id, state.LastSuccessfulSyncRunID, state.LastSuccessfulAt)
	}
	scope := state.QueryScope().OnlyX(ctx)
	if scope.ScopeKind != "source_instance" {
		t.Fatalf("source scope %d kind = %q, want source_instance", scope.ID, scope.ScopeKind)
	}
	conn := scope.QueryConnection().OnlyX(ctx)
	if conn.SourceSystem != sourceSystem || conn.SourceInstance != sourceInstance {
		t.Fatalf("source connection for state %d = %s/%s, want %s/%s", id, conn.SourceSystem, conn.SourceInstance, sourceSystem, sourceInstance)
	}
}

// assertCount names materialized row-count failures in graph assertions.
func assertCount(t *testing.T, name string, got int, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s count = %d, want %d", name, got, want)
	}
}

var _ *genent.Client
