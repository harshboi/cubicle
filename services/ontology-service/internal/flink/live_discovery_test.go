package flink

import (
	"strings"
	"testing"
)

func TestPlanFollowUpLiveRequestsDiscoversJiraRemoteLinksFromSearchSnapshots(t *testing.T) {
	cfg := testLiveConfig()
	cfg.IssueLimit = 2
	requests, err := PlanFollowUpLiveRequests(cfg, []SnapshotRecord{{
		SnapshotKey:      "snapshot:jira:search:test",
		Source:           SourceJira,
		SourceInstance:   SourceInstanceJira,
		SourceObjectType: "jira_search_page",
		Body: []byte(`{
			"issues": [
				{"key":"FLINK-39743","fields":{"summary":"A","status":{"name":"Open"}}},
				{"key":"FLINK-1","fields":{"summary":"B","status":{"name":"Open"}}},
				{"key":"FLINK-2","fields":{"summary":"C","status":{"name":"Open"}}},
				{"key":"FLINK-1","fields":{"summary":"duplicate","status":{"name":"Open"}}}
			]
		}`),
	}})
	if err != nil {
		t.Fatalf("plan follow-up requests: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	if requests[0].SourceObjectType != "jira_remote_links" || requests[0].SourceObjectID != "FLINK-1" {
		t.Fatalf("first request = %#v", requests[0])
	}
	if requests[1].SourceObjectType != "jira_remote_links" || requests[1].SourceObjectID != "FLINK-2" {
		t.Fatalf("second request = %#v", requests[1])
	}
}

func TestPlanFollowUpLiveRequestsDiscoversGitHubPRDetails(t *testing.T) {
	cfg := testLiveConfig()
	requests, err := PlanFollowUpLiveRequests(cfg, []SnapshotRecord{
		{
			SnapshotKey:      "snapshot:jira:remote-links:test",
			Source:           SourceJira,
			SourceInstance:   SourceInstanceJira,
			SourceObjectType: "jira_remote_links",
			Body: []byte(`{
				"issueKey": "FLINK-39743",
				"links": [
					{"title":"PR 1234","url":"https://github.com/apache/flink-kubernetes-operator/pull/1234"},
					{"title":"other repo","url":"https://github.com/apache/other/pull/7"}
				]
			}`),
		},
		{
			SnapshotKey:      "snapshot:github:search:test",
			Source:           SourceGitHub,
			SourceInstance:   SourceInstanceGitHub,
			SourceObjectType: "github_pr_search",
			Body: []byte(`{
				"items": [
					{"html_url":"https://github.com/apache/flink-kubernetes-operator/pull/99"},
					{"html_url":"https://github.com/apache/flink-kubernetes-operator/pull/1234"}
				]
			}`),
		},
	})
	if err != nil {
		t.Fatalf("plan follow-up requests: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	if requests[0].SourceObjectID != "apache/flink-kubernetes-operator#99" {
		t.Fatalf("first PR request = %#v", requests[0])
	}
	if requests[1].SourceObjectID != "apache/flink-kubernetes-operator#1234" {
		t.Fatalf("second PR request = %#v", requests[1])
	}
}

func TestPlanFollowUpLiveRequestsRejectsMalformedKnownSnapshots(t *testing.T) {
	_, err := PlanFollowUpLiveRequests(testLiveConfig(), []SnapshotRecord{{
		SnapshotKey:      "snapshot:jira:search:bad",
		SourceObjectType: "jira_search_page",
		Body:             []byte(`not-json`),
	}})
	if err == nil || !strings.Contains(err.Error(), "decode Jira search discovery snapshot") {
		t.Fatalf("expected decode error, got %T: %v", err, err)
	}
}
