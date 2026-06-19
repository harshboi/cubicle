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
	if result.Evidence != 5 || result.SyncIssues != 6 || result.WorkLensWindows != 1 {
		t.Fatalf("evidence/coverage/window counts = %#v", result)
	}

	assertCount(t, "tickets", client.Ticket.Query().CountX(ctx), 1)
	assertCount(t, "pull requests", client.PullRequest.Query().CountX(ctx), 2)
	assertCount(t, "ticket pull requests", client.TicketPullRequest.Query().CountX(ctx), 2)
	assertCount(t, "evidence", client.Evidence.Query().CountX(ctx), 5)
	assertCount(t, "sync issues", client.SourceSyncIssue.Query().CountX(ctx), 6)
	assertCount(t, "work lens windows", client.WorkLensWindow.Query().CountX(ctx), 1)

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
	if run.ObjectsCreatedCount != 3 || run.RelationshipsCreatedCount != 2 || run.EvidenceCreatedCount != 5 || run.IssuesCreatedCount != 6 {
		t.Fatalf("sync run counters = %#v", run)
	}
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

// assertCount names materialized row-count failures in graph assertions.
func assertCount(t *testing.T, name string, got int, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s count = %d, want %d", name, got, want)
	}
}

var _ *genent.Client
