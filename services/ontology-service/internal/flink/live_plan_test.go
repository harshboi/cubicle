package flink

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPlanInitialLiveRequestsBuildsBoundedPublicRequests(t *testing.T) {
	cfg := testLiveConfig()
	requests, err := PlanInitialLiveRequests(cfg)
	if err != nil {
		t.Fatalf("plan initial requests: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("request count = %d", len(requests))
	}

	jira := requests[0]
	if jira.Source != SourceJira || jira.SourceInstance != SourceInstanceJira {
		t.Fatalf("jira source identity = %s/%s", jira.Source, jira.SourceInstance)
	}
	if jira.SnapshotKey != "snapshot:jira:search:20260608T120000Z:start-0" {
		t.Fatalf("jira snapshot key = %s", jira.SnapshotKey)
	}
	parsedJira, err := url.Parse(jira.URL)
	if err != nil {
		t.Fatalf("parse jira url: %v", err)
	}
	jql := parsedJira.Query().Get("jql")
	for _, required := range []string{
		`project = FLINK`,
		`component = "Autoscaler"`,
		`updated >= "2023-01-01 00:00"`,
		`updated <= "2026-06-08 12:00"`,
		`ORDER BY updated ASC, key ASC`,
	} {
		if !strings.Contains(jql, required) {
			t.Fatalf("JQL %q missing %q", jql, required)
		}
	}
	if parsedJira.Query().Get("maxResults") != "50" || parsedJira.Query().Get("startAt") != "0" {
		t.Fatalf("jira pagination = %s", parsedJira.RawQuery)
	}

	github := requests[1]
	if github.Source != SourceGitHub || github.SourceObjectKind != "github_pr_search" {
		t.Fatalf("github request = %#v", github)
	}
	parsedGitHub, err := url.Parse(github.URL)
	if err != nil {
		t.Fatalf("parse github url: %v", err)
	}
	if parsedGitHub.Path != "/search/issues" {
		t.Fatalf("github path = %s", parsedGitHub.Path)
	}
	if query := parsedGitHub.Query().Get("q"); !strings.Contains(query, "repo:apache/flink-kubernetes-operator") || !strings.Contains(query, "updated:>=2025-06-01") {
		t.Fatalf("github query = %q", query)
	}

	docs := requests[2]
	if docs.Source != SourceDocs || docs.SourceInstance != SourceInstanceDocs {
		t.Fatalf("docs source identity = %s/%s", docs.Source, docs.SourceInstance)
	}
	if docs.SourceObjectID != "docs/content/docs/custom-resource/autoscaler.md" {
		t.Fatalf("docs source object = %s", docs.SourceObjectID)
	}
}

func TestPlanInitialLiveRequestsRequiresCrawlBounds(t *testing.T) {
	cfg := testLiveConfig()
	cfg.CrawlStartedAt = time.Time{}
	_, err := PlanInitialLiveRequests(cfg)
	if !errors.Is(err, ErrInvalidLivePlan) {
		t.Fatalf("expected ErrInvalidLivePlan for missing crawl start, got %T: %v", err, err)
	}

	cfg = testLiveConfig()
	cfg.JiraSince = time.Time{}
	_, err = PlanInitialLiveRequests(cfg)
	if !errors.Is(err, ErrInvalidLivePlan) {
		t.Fatalf("expected ErrInvalidLivePlan for missing jira since, got %T: %v", err, err)
	}

	cfg = testLiveConfig()
	cfg.JiraBaseURL = "://bad-url"
	_, err = PlanInitialLiveRequests(cfg)
	if !errors.Is(err, ErrInvalidLivePlan) {
		t.Fatalf("expected ErrInvalidLivePlan for invalid base url, got %T: %v", err, err)
	}
}

func TestPlanJiraRemoteLinkRequestsNormalizesIssueKeys(t *testing.T) {
	cfg := testLiveConfig()
	cfg.IssueLimit = 2
	requests, err := PlanJiraRemoteLinkRequests(cfg, []string{" flink-39743 ", "FLINK-39743", "FLINK-1", "NOT-1", "FLINK-2"})
	if err != nil {
		t.Fatalf("plan remote links: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	if requests[0].SourceObjectID != "FLINK-1" || requests[1].SourceObjectID != "FLINK-2" {
		t.Fatalf("issue order/limit = %#v", requests)
	}
	if !strings.HasSuffix(requests[0].URL, "/rest/api/2/issue/FLINK-1/remotelink") {
		t.Fatalf("remote link url = %s", requests[0].URL)
	}
	if requests[0].SnapshotKey != "snapshot:jira:remote-links:FLINK-1:20260608T120000Z" {
		t.Fatalf("snapshot key = %s", requests[0].SnapshotKey)
	}
}

func TestPlanGitHubPRDetailRequestsNormalizesRemoteLinks(t *testing.T) {
	cfg := testLiveConfig()
	cfg.PRDetailLimit = 2
	requests, err := PlanGitHubPRDetailRequests(cfg, []string{
		"https://github.com/apache/flink-kubernetes-operator/pull/1234",
		"https://github.com/apache/flink-kubernetes-operator/pull/1234",
		"https://github.com/apache/flink-kubernetes-operator/pull/99",
		"https://github.com/apache/other/pull/1",
	})
	if err != nil {
		t.Fatalf("plan github details: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	if requests[0].SourceObjectID != "apache/flink-kubernetes-operator#99" || requests[1].SourceObjectID != "apache/flink-kubernetes-operator#1234" {
		t.Fatalf("PR order/limit = %#v", requests)
	}
	if requests[0].URL != "https://api.github.com/repos/apache/flink-kubernetes-operator/pulls/99" {
		t.Fatalf("github detail url = %s", requests[0].URL)
	}
	if requests[0].SnapshotKey != "snapshot:github:pr:apache-flink-kubernetes-operator:99:20260608T120000Z" {
		t.Fatalf("snapshot key = %s", requests[0].SnapshotKey)
	}
}

func testLiveConfig() LiveConfig {
	cfg := DefaultLiveConfig()
	cfg.CrawlStartedAt = time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	cfg.JiraSince = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg.GitHubSeedSince = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	return cfg
}
