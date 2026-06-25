package sourcegraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

// RecordFilterOptions bounds a large replay fixture to a focused PR/Jira slice.
type RecordFilterOptions struct {
	GitHubRepo         string
	PullRequestNumbers []int
	JiraKeys           []string
}

// FilterRecords keeps selected PR bundle snapshots, selected Jira issue snapshots,
// and GitHub search snippets trimmed to selected PRs. It never turns search hits
// into product relationships; it only preserves evidence for resolver queues.
func FilterRecords(records []sourcecapture.Record, opts RecordFilterOptions) ([]sourcecapture.Record, error) {
	if len(opts.PullRequestNumbers) == 0 && len(opts.JiraKeys) == 0 {
		filtered := make([]sourcecapture.Record, len(records))
		copy(filtered, records)
		return filtered, nil
	}
	repo := opts.GitHubRepo
	if repo == "" {
		repo = defaultGitHubRepo
	}
	selectedPRs := selectedPRObjectIDs(repo, opts.PullRequestNumbers)
	selectedJiraKeys := make(map[string]struct{})
	for _, key := range NormalizeIssueKeys(opts.JiraKeys) {
		selectedJiraKeys[key] = struct{}{}
	}
	for _, key := range issueKeysFromSelectedPRRecords(records, selectedPRs) {
		selectedJiraKeys[key] = struct{}{}
	}
	if err := addSearchIssueKeysFromSelectedPRs(records, selectedPRs, selectedJiraKeys); err != nil {
		return nil, err
	}

	filtered := make([]sourcecapture.Record, 0, len(records))
	for _, record := range records {
		switch {
		case isPRBundleType(record.SourceObjectType):
			if _, ok := selectedPRs[record.SourceObjectID]; ok {
				filtered = append(filtered, record)
			}
		case isJiraIssueRecordType(record.SourceObjectType) || isJiraRemoteLinksRecordType(record.SourceObjectType):
			if _, ok := selectedJiraKeys[record.SourceObjectID]; ok {
				filtered = append(filtered, record)
			}
		case record.SourceObjectType == "github_pr_search" || record.SourceObjectType == "github_pr_key_search":
			trimmed, ok, err := trimGitHubSearchRecord(record, selectedPRs, selectedJiraKeys)
			if err != nil {
				return nil, err
			}
			if ok {
				filtered = append(filtered, trimmed)
			}
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].SnapshotKey < filtered[j].SnapshotKey
	})
	return filtered, nil
}

func addSearchIssueKeysFromSelectedPRs(records []sourcecapture.Record, selectedPRs map[string]struct{}, selectedJiraKeys map[string]struct{}) error {
	for _, record := range records {
		if record.Response.StatusCode != 200 || (record.SourceObjectType != "github_pr_search" && record.SourceObjectType != "github_pr_key_search") {
			continue
		}
		var page struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(record.Body, &page); err != nil {
			return fmt.Errorf("decode GitHub search record %s: %w", record.SnapshotKey, err)
		}
		for _, raw := range page.Items {
			var item struct {
				HTMLURL     string `json:"html_url"`
				TextMatches []struct {
					Fragment string `json:"fragment"`
				} `json:"text_matches"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return fmt.Errorf("decode GitHub search item in %s: %w", record.SnapshotKey, err)
			}
			ref, ok := ParseGitHubPRURL(item.HTMLURL)
			if !ok {
				continue
			}
			if _, selected := selectedPRs[prExternalID(ref)]; !selected {
				continue
			}
			for _, textMatch := range item.TextMatches {
				for _, key := range ExtractIssueKeys(textMatch.Fragment) {
					selectedJiraKeys[key] = struct{}{}
				}
			}
		}
	}
	return nil
}

func selectedPRObjectIDs(repo string, numbers []int) map[string]struct{} {
	selected := make(map[string]struct{}, len(numbers))
	for _, number := range numbers {
		if number <= 0 {
			continue
		}
		selected[repo+"#"+strconv.Itoa(number)] = struct{}{}
	}
	return selected
}

func issueKeysFromSelectedPRRecords(records []sourcecapture.Record, selectedPRs map[string]struct{}) []string {
	seen := make(map[string]struct{})
	for _, record := range records {
		if _, ok := selectedPRs[record.SourceObjectID]; !ok || record.Response.StatusCode != 200 {
			continue
		}
		switch record.SourceObjectType {
		case "github_pull_request":
			links, err := issueLinksFromPullRequestRecord(record)
			if err != nil {
				continue
			}
			for _, link := range links {
				seen[link.IssueKey] = struct{}{}
			}
		case "github_pull_request_commits":
			links, err := issueLinksFromCommitRecord(record)
			if err != nil {
				continue
			}
			for _, link := range links {
				seen[link.IssueKey] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func trimGitHubSearchRecord(record sourcecapture.Record, selectedPRs map[string]struct{}, selectedJiraKeys map[string]struct{}) (sourcecapture.Record, bool, error) {
	var page struct {
		TotalCount        int               `json:"total_count"`
		IncompleteResults bool              `json:"incomplete_results"`
		Items             []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(record.Body, &page); err != nil {
		return sourcecapture.Record{}, false, fmt.Errorf("decode GitHub search record %s: %w", record.SnapshotKey, err)
	}
	filteredItems := make([]json.RawMessage, 0, len(page.Items))
	for _, raw := range page.Items {
		var item struct {
			HTMLURL     string `json:"html_url"`
			TextMatches []struct {
				Fragment string `json:"fragment"`
			} `json:"text_matches"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return sourcecapture.Record{}, false, fmt.Errorf("decode GitHub search item in %s: %w", record.SnapshotKey, err)
		}
		ref, ok := ParseGitHubPRURL(item.HTMLURL)
		if !ok {
			continue
		}
		if _, selected := selectedPRs[prExternalID(ref)]; !selected {
			continue
		}
		filteredItems = append(filteredItems, raw)
		for _, textMatch := range item.TextMatches {
			for _, key := range ExtractIssueKeys(textMatch.Fragment) {
				selectedJiraKeys[key] = struct{}{}
			}
		}
	}
	if len(filteredItems) == 0 {
		return sourcecapture.Record{}, false, nil
	}
	page.TotalCount = len(filteredItems)
	page.Items = filteredItems
	body, err := json.Marshal(page)
	if err != nil {
		return sourcecapture.Record{}, false, fmt.Errorf("encode trimmed GitHub search record %s: %w", record.SnapshotKey, err)
	}
	trimmed := record
	trimmed.Body = body
	trimmed.BodySHA256 = sourcecapture.HashBody(body)
	return trimmed, true, nil
}
