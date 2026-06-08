package flink

import (
	"encoding/json"
	"fmt"

	"cubicle/services/ontology-service/internal/sourcefetch"
)

func PlanFollowUpLiveRequests(cfg LiveConfig, records []SnapshotRecord) ([]sourcefetch.Request, error) {
	var issueKeys []string
	var prURLs []string
	for _, record := range records {
		switch record.SourceObjectType {
		case "jira_search_page":
			keys, err := issueKeysFromJiraSearch(record)
			if err != nil {
				return nil, err
			}
			issueKeys = append(issueKeys, keys...)
		case "jira_remote_links":
			urls, err := prURLsFromJiraRemoteLinks(record)
			if err != nil {
				return nil, err
			}
			prURLs = append(prURLs, urls...)
		case "github_pr_search":
			urls, err := prURLsFromGitHubSearch(record)
			if err != nil {
				return nil, err
			}
			prURLs = append(prURLs, urls...)
		}
	}

	remoteLinkRequests, err := PlanJiraRemoteLinkRequests(cfg, issueKeys)
	if err != nil {
		return nil, err
	}
	prDetailRequests, err := PlanGitHubPRDetailRequests(cfg, prURLs)
	if err != nil {
		return nil, err
	}
	requests := make([]sourcefetch.Request, 0, len(remoteLinkRequests)+len(prDetailRequests))
	requests = append(requests, remoteLinkRequests...)
	requests = append(requests, prDetailRequests...)
	return requests, nil
}

func issueKeysFromJiraSearch(record SnapshotRecord) ([]string, error) {
	var page jiraSearchPage
	if err := json.Unmarshal(record.Body, &page); err != nil {
		return nil, fmt.Errorf("decode Jira search discovery snapshot %s: %w", record.SnapshotKey, err)
	}
	keys := make([]string, 0, len(page.Issues))
	for _, issue := range page.Issues {
		keys = append(keys, issue.Key)
	}
	return keys, nil
}

func prURLsFromJiraRemoteLinks(record SnapshotRecord) ([]string, error) {
	var links jiraRemoteLinks
	if err := json.Unmarshal(record.Body, &links); err != nil {
		return nil, fmt.Errorf("decode Jira remote-link discovery snapshot %s: %w", record.SnapshotKey, err)
	}
	urls := make([]string, 0, len(links.Links))
	for _, link := range links.Links {
		urls = append(urls, link.URL)
	}
	return urls, nil
}

func prURLsFromGitHubSearch(record SnapshotRecord) ([]string, error) {
	var search githubSearchPage
	if err := json.Unmarshal(record.Body, &search); err != nil {
		return nil, fmt.Errorf("decode GitHub search discovery snapshot %s: %w", record.SnapshotKey, err)
	}
	urls := make([]string, 0, len(search.Items))
	for _, item := range search.Items {
		urls = append(urls, item.HTMLURL)
	}
	return urls, nil
}

type githubSearchPage struct {
	Items []struct {
		HTMLURL string `json:"html_url"`
	} `json:"items"`
}
