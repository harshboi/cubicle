// Association:
//
//	Plan* tests -> sourcecapture.Request identity
//	extractor tests -> Record body -> stable keys / PR refs
//
// These tests keep the Flink crawl plan deterministic before any live fetch
// or ontology materialization happens.
package sourcegraph

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

// TestPlanSeedRequests checks the first crawl fan-out: Jira, GitHub, docs.
func TestPlanSeedRequests(t *testing.T) {
	cfg := DefaultConfig(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC))
	requests, err := PlanSeedRequests(cfg)
	if err != nil {
		t.Fatalf("PlanSeedRequests returned error: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests len = %d, want 3", len(requests))
	}
	if requests[0].SourceObjectType != "jira_search_page" {
		t.Fatalf("first request = %#v", requests[0])
	}
	jiraURL, err := url.Parse(requests[0].URL)
	if err != nil {
		t.Fatalf("parse Jira URL: %v", err)
	}
	jql := jiraURL.Query().Get("jql")
	if !strings.Contains(jql, `component = "Autoscaler"`) || !strings.Contains(jql, `updated <= "2026-06-11"`) {
		t.Fatalf("unexpected JQL: %s", jql)
	}

	githubURL, err := url.Parse(requests[1].URL)
	if err != nil {
		t.Fatalf("parse GitHub URL: %v", err)
	}
	if got := githubURL.Query().Get("q"); got != "repo:apache/flink-kubernetes-operator is:pr FLINK- updated:2025-06-01..2026-06-11" {
		t.Fatalf("unexpected GitHub query: %s", got)
	}
	if requests[2].SourceObjectType != "github_docs_tree" {
		t.Fatalf("third request = %#v", requests[2])
	}
}

// TestPlanGitHubPRBundleSeparatesDetailEndpoints protects the complete-PR coverage contract.
func TestPlanGitHubPRBundleSeparatesDetailEndpoints(t *testing.T) {
	cfg := DefaultConfig(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC))
	requests, err := PlanGitHubPRBundle(cfg, PullRequestRef{Repo: "apache/flink-kubernetes-operator", Number: 1127})
	if err != nil {
		t.Fatalf("PlanGitHubPRBundle returned error: %v", err)
	}
	if len(requests) != 6 {
		t.Fatalf("requests len = %d, want 6", len(requests))
	}
	wantTypes := []string{
		"github_pull_request",
		"github_pull_request_files",
		"github_issue_comments",
		"github_pull_request_review_comments",
		"github_pull_request_reviews",
		"github_pull_request_commits",
	}
	for idx, want := range wantTypes {
		if requests[idx].SourceObjectType != want {
			t.Fatalf("request %d type = %s, want %s", idx, requests[idx].SourceObjectType, want)
		}
		if !strings.Contains(requests[idx].URL, "/1127") {
			t.Fatalf("request %d URL does not target PR 1127: %s", idx, requests[idx].URL)
		}
	}
}

// TestExtractors checks snapshot records become keys and PR refs, not product rows.
func TestExtractors(t *testing.T) {
	jiraSearch := sourcecapture.Record{
		SnapshotKey: "snapshot:jira",
		Body: []byte(`{
		  "issues": [
		    {"key": "FLINK-39743"},
		    {"key": "FLINK-30574"},
		    {"key": "NOT-FLINK"}
		  ]
		}`),
	}
	keys, err := IssueKeysFromJiraSearch(jiraSearch)
	if err != nil {
		t.Fatalf("IssueKeysFromJiraSearch returned error: %v", err)
	}
	if strings.Join(keys, ",") != "FLINK-30574,FLINK-39743" {
		t.Fatalf("keys = %#v", keys)
	}

	remoteLinks := sourcecapture.Record{
		SnapshotKey: "snapshot:remote-links",
		Body: []byte(`[
		  {"object": {"url": "https://github.com/apache/flink-kubernetes-operator/pull/1127"}},
		  {"object": {"url": "https://example.com/not-a-pr"}}
		]`),
	}
	prURLs, err := PRURLsFromJiraRemoteLinks(remoteLinks)
	if err != nil {
		t.Fatalf("PRURLsFromJiraRemoteLinks returned error: %v", err)
	}
	if len(prURLs) != 1 || prURLs[0] != "https://github.com/apache/flink-kubernetes-operator/pull/1127" {
		t.Fatalf("prURLs = %#v", prURLs)
	}

	search := sourcecapture.Record{
		SnapshotKey: "snapshot:github-search",
		Body: []byte(`{
		  "items": [
		    {"html_url": "https://github.com/apache/flink-kubernetes-operator/pull/1127"},
		    {"html_url": "https://github.com/apache/flink-kubernetes-operator/pull/1127"}
		  ]
		}`),
	}
	refs, err := PullRequestsFromGitHubSearch(search)
	if err != nil {
		t.Fatalf("PullRequestsFromGitHubSearch returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].Repo != "apache/flink-kubernetes-operator" || refs[0].Number != 1127 {
		t.Fatalf("refs = %#v", refs)
	}
}

// TestMarkdownDocsFromTree keeps docs discovery scoped to markdown docs content.
func TestMarkdownDocsFromTree(t *testing.T) {
	record := sourcecapture.Record{
		SnapshotKey: "snapshot:docs-tree",
		Body: []byte(`{
		  "tree": [
		    {"path": "docs/content/docs/custom-resource/sourcegraph.md", "type": "blob"},
		    {"path": "docs/content/docs/custom-resource/sourcegraph.png", "type": "blob"},
		    {"path": "README.md", "type": "blob"}
		  ]
		}`),
	}
	paths, err := MarkdownDocsFromTree(record)
	if err != nil {
		t.Fatalf("MarkdownDocsFromTree returned error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "docs/content/docs/custom-resource/sourcegraph.md" {
		t.Fatalf("paths = %#v", paths)
	}
}

// TestExtractIssueKeys keeps free-text FLINK references stable and deduped.
func TestExtractIssueKeys(t *testing.T) {
	keys := ExtractIssueKeys("[FLINK-39743] commit message links FLINK-30574 and flink-39743")
	if strings.Join(keys, ",") != "FLINK-30574,FLINK-39743" {
		t.Fatalf("keys = %#v", keys)
	}
}

// TestReadFixtureManifestMapsSourceInstances checks capture source names map into Cubicle scopes.
func TestReadFixtureManifestMapsSourceInstances(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	body := []byte("# autoscaler")
	bodyHash := sourcecapture.HashBody(body)
	if err := os.WriteFile(filepath.Join(dir, "docs", "sourcegraph.md"), body, 0o644); err != nil {
		t.Fatalf("write fixture body: %v", err)
	}
	manifest := `{"path":"docs/sourcegraph.md","source":"github_docs","source_object_type":"github_markdown_doc","source_object_id":"apache/flink-kubernetes-operator:docs/content/docs/custom-resource/sourcegraph.md","url":"https://raw.githubusercontent.test/apache/flink-kubernetes-operator/main/docs/content/docs/custom-resource/sourcegraph.md","status_code":200,"body_sha256":"` + bodyHash + `","bytes":12}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}

	records, err := ReadFixtureManifest(dir)
	if err != nil {
		t.Fatalf("ReadFixtureManifest returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].SourceInstance != SourceInstanceDocs {
		t.Fatalf("source instance = %q, want %q", records[0].SourceInstance, SourceInstanceDocs)
	}
}
