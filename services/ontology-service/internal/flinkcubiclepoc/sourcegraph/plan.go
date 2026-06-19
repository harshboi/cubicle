// Association:
//
//	Config -> Plan* -> sourcecapture.Request -> source snapshot
//	Record -> extractor -> issue keys / PR refs / docs paths
//
// The planner names every source request before fetching so replay, idempotency,
// and later evidence all share the same source identity.
package sourcegraph

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

const (
	SourceJira   = "jira"
	SourceGitHub = "github"
	SourceDocs   = "github_docs"
	SourceMail   = "ponymail"

	SourceInstanceJira   = "apache-jira"
	SourceInstanceGitHub = "github.com/apache/flink-kubernetes-operator"
	SourceInstanceDocs   = "github.com/apache/flink-kubernetes-operator/docs"
	SourceInstanceMail   = "lists.apache.org/flink"

	defaultJiraBaseURL      = "https://issues.apache.org/jira"
	defaultGitHubAPIBaseURL = "https://api.github.com"
	defaultGitHubRepo       = "apache/flink-kubernetes-operator"
	defaultDocsRef          = "main"
	defaultJiraPageSize     = 50
	defaultGitHubPageSize   = 100
)

var (
	ErrInvalidConfig     = errors.New("invalid Flink source config")
	issueKeyPattern      = regexp.MustCompile(`\bFLINK-\d+\b`)
	exactIssueKeyPattern = regexp.MustCompile(`^FLINK-\d+$`)
)

// Config controls a bounded Flink Autoscaler source plan.
type Config struct {
	CrawlStartedAt   time.Time
	JiraBaseURL      string
	JiraSince        time.Time
	JiraPageSize     int
	GitHubAPIBaseURL string
	GitHubRepo       string
	GitHubSeedSince  time.Time
	GitHubPageSize   int
	DocsRef          string
}

// ReadFixtureManifest converts the captured Flink dataset manifest into sourcecapture records.
func ReadFixtureManifest(dir string) ([]sourcecapture.Record, error) {
	return sourcecapture.ReadCaptureManifest(dir, sourcecapture.CaptureManifestOptions{
		SourceInstances: map[string]string{
			SourceJira:   SourceInstanceJira,
			SourceGitHub: SourceInstanceGitHub,
			SourceDocs:   SourceInstanceDocs,
			SourceMail:   SourceInstanceMail,
		},
	})
}

// DefaultConfig returns the current Flink Autoscaler POC crawl scope.
func DefaultConfig(crawlStartedAt time.Time) Config {
	return Config{
		CrawlStartedAt:   crawlStartedAt,
		JiraBaseURL:      defaultJiraBaseURL,
		JiraSince:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		JiraPageSize:     defaultJiraPageSize,
		GitHubAPIBaseURL: defaultGitHubAPIBaseURL,
		GitHubRepo:       defaultGitHubRepo,
		GitHubSeedSince:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		GitHubPageSize:   defaultGitHubPageSize,
		DocsRef:          defaultDocsRef,
	}
}

// PlanSeedRequests plans the first bounded source pass: Jira page, GitHub seed search, and docs tree.
func PlanSeedRequests(cfg Config) ([]sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	jira, err := PlanJiraSearchPage(cfg, 0)
	if err != nil {
		return nil, err
	}
	github, err := PlanGitHubSeedSearch(cfg, 1)
	if err != nil {
		return nil, err
	}
	docs, err := PlanDocsTree(cfg)
	if err != nil {
		return nil, err
	}
	return []sourcecapture.Request{jira, github, docs}, nil
}

// PlanJiraSearchPage creates one bounded Autoscaler Jira search page request.
func PlanJiraSearchPage(cfg Config, startAt int) (sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	endpoint, err := parseBaseURL(cfg.JiraBaseURL)
	if err != nil {
		return sourcecapture.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "rest/api/2/search")
	query := endpoint.Query()
	query.Set("jql", JiraAutoscalerJQL(cfg.JiraSince, cfg.CrawlStartedAt))
	query.Set("fields", strings.Join([]string{
		"key",
		"summary",
		"status",
		"issuetype",
		"priority",
		"assignee",
		"reporter",
		"created",
		"updated",
		"components",
		"labels",
		"fixVersions",
		"issuelinks",
		"description",
		"comment",
	}, ","))
	query.Set("maxResults", strconv.Itoa(cfg.JiraPageSize))
	query.Set("startAt", strconv.Itoa(startAt))
	endpoint.RawQuery = query.Encode()

	objectID := fmt.Sprintf("start-%d", startAt)
	return sourcecapture.Request{
		SnapshotKey:      "snapshot:jira:search:" + crawlID(cfg.CrawlStartedAt) + ":" + objectID,
		URL:              endpoint.String(),
		SourceKey:        SourceJira,
		SourceInstance:   SourceInstanceJira,
		SourceObjectType: "jira_search_page",
		SourceObjectID:   objectID,
		SourceURL:        strings.TrimRight(cfg.JiraBaseURL, "/") + "/issues/?jql=" + url.QueryEscape(JiraAutoscalerJQL(cfg.JiraSince, cfg.CrawlStartedAt)),
	}, nil
}

// PlanJiraRemoteLinks creates remote-link requests for stable Jira issue keys.
func PlanJiraRemoteLinks(cfg Config, issueKeys []string) ([]sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	keys := NormalizeIssueKeys(issueKeys)
	requests := make([]sourcecapture.Request, 0, len(keys))
	for _, key := range keys {
		endpoint, err := joinURLPath(cfg.JiraBaseURL, "rest/api/2/issue/"+key+"/remotelink")
		if err != nil {
			return nil, err
		}
		requests = append(requests, sourcecapture.Request{
			SnapshotKey:      "snapshot:jira:remote-links:" + key + ":" + crawlID(cfg.CrawlStartedAt),
			URL:              endpoint,
			SourceKey:        SourceJira,
			SourceInstance:   SourceInstanceJira,
			SourceObjectType: "jira_remote_links",
			SourceObjectID:   key,
			SourceURL:        strings.TrimRight(cfg.JiraBaseURL, "/") + "/browse/" + key,
		})
	}
	return requests, nil
}

// PlanGitHubSeedSearch creates one repo-wide hint search page.
func PlanGitHubSeedSearch(cfg Config, pageNumber int) (sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	endpoint, err := parseBaseURL(cfg.GitHubAPIBaseURL)
	if err != nil {
		return sourcecapture.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "search/issues")
	query := endpoint.Query()
	query.Set("q", fmt.Sprintf("repo:%s is:pr FLINK- updated:%s..%s", cfg.GitHubRepo, date(cfg.GitHubSeedSince), date(cfg.CrawlStartedAt)))
	query.Set("sort", "updated")
	query.Set("order", "asc")
	query.Set("per_page", strconv.Itoa(cfg.GitHubPageSize))
	query.Set("page", strconv.Itoa(pageNumber))
	endpoint.RawQuery = query.Encode()
	return sourcecapture.Request{
		SnapshotKey:      "snapshot:github:search:" + safeKey(cfg.GitHubRepo) + ":" + crawlID(cfg.CrawlStartedAt) + ":page-" + strconv.Itoa(pageNumber),
		URL:              endpoint.String(),
		SourceKey:        SourceGitHub,
		SourceInstance:   githubSourceInstance(cfg.GitHubRepo),
		SourceObjectType: "github_pr_search",
		SourceObjectID:   cfg.GitHubRepo + ":page-" + strconv.Itoa(pageNumber),
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/pulls",
	}, nil
}

// PlanGitHubIssueKeySearch creates an exact issue-key PR search to avoid repo-wide search caps.
func PlanGitHubIssueKeySearch(cfg Config, issueKey string) (sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	key := strings.ToUpper(strings.TrimSpace(issueKey))
	if !exactIssueKeyPattern.MatchString(key) {
		return sourcecapture.Request{}, fmt.Errorf("%w: invalid Flink issue key %q", ErrInvalidConfig, issueKey)
	}
	endpoint, err := parseBaseURL(cfg.GitHubAPIBaseURL)
	if err != nil {
		return sourcecapture.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "search/issues")
	query := endpoint.Query()
	query.Set("q", fmt.Sprintf("repo:%s is:pr %s", cfg.GitHubRepo, key))
	query.Set("per_page", "10")
	endpoint.RawQuery = query.Encode()
	return sourcecapture.Request{
		SnapshotKey:      "snapshot:github:search-key:" + key + ":" + crawlID(cfg.CrawlStartedAt),
		URL:              endpoint.String(),
		SourceKey:        SourceGitHub,
		SourceInstance:   githubSourceInstance(cfg.GitHubRepo),
		SourceObjectType: "github_pr_key_search",
		SourceObjectID:   key,
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/pulls?q=" + url.QueryEscape(key),
	}, nil
}

// PlanGitHubPRBundle creates the endpoint snapshots needed to analyze one PR.
func PlanGitHubPRBundle(cfg Config, pr PullRequestRef) ([]sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	if pr.Repo == "" {
		pr.Repo = cfg.GitHubRepo
	}
	if pr.Number <= 0 {
		return nil, fmt.Errorf("%w: pull request number must be positive", ErrInvalidConfig)
	}
	endpoints := []struct {
		objectType string
		apiPath    string
	}{
		{"github_pull_request", fmt.Sprintf("repos/%s/pulls/%d", pr.Repo, pr.Number)},
		{"github_pull_request_files", fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", pr.Repo, pr.Number)},
		{"github_issue_comments", fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", pr.Repo, pr.Number)},
		{"github_pull_request_review_comments", fmt.Sprintf("repos/%s/pulls/%d/comments?per_page=100", pr.Repo, pr.Number)},
		{"github_pull_request_reviews", fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100", pr.Repo, pr.Number)},
		{"github_pull_request_commits", fmt.Sprintf("repos/%s/pulls/%d/commits?per_page=100", pr.Repo, pr.Number)},
	}
	requests := make([]sourcecapture.Request, 0, len(endpoints))
	for _, endpoint := range endpoints {
		requestURL, err := joinURLPathWithQuery(cfg.GitHubAPIBaseURL, endpoint.apiPath)
		if err != nil {
			return nil, err
		}
		requests = append(requests, sourcecapture.Request{
			SnapshotKey:      "snapshot:github:" + endpoint.objectType + ":" + safeKey(pr.Repo) + ":" + strconv.Itoa(pr.Number) + ":" + crawlID(cfg.CrawlStartedAt),
			URL:              requestURL,
			SourceKey:        SourceGitHub,
			SourceInstance:   "github.com/" + pr.Repo,
			SourceObjectType: endpoint.objectType,
			SourceObjectID:   pr.Repo + "#" + strconv.Itoa(pr.Number),
			SourceURL:        "https://github.com/" + pr.Repo + "/pull/" + strconv.Itoa(pr.Number),
		})
	}
	return requests, nil
}

// PlanDocsTree creates the GitHub tree request used to discover markdown docs.
func PlanDocsTree(cfg Config) (sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	requestURL, err := joinURLPath(cfg.GitHubAPIBaseURL, fmt.Sprintf("repos/%s/git/trees/%s", cfg.GitHubRepo, cfg.DocsRef))
	if err != nil {
		return sourcecapture.Request{}, err
	}
	requestURL += "?recursive=1"
	return sourcecapture.Request{
		SnapshotKey:      "snapshot:docs:tree:" + safeKey(cfg.GitHubRepo) + ":" + cfg.DocsRef + ":" + crawlID(cfg.CrawlStartedAt),
		URL:              requestURL,
		SourceKey:        SourceDocs,
		SourceInstance:   docsSourceInstance(cfg.GitHubRepo),
		SourceObjectType: "github_docs_tree",
		SourceObjectID:   cfg.GitHubRepo + "@" + cfg.DocsRef,
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/tree/" + cfg.DocsRef + "/docs/content/docs",
	}, nil
}

// PlanDocsRaw creates a raw markdown request for one repo path.
func PlanDocsRaw(cfg Config, docPath string) (sourcecapture.Request, error) {
	cfg = cfg.withDefaults()
	docPath = strings.TrimPrefix(strings.TrimSpace(docPath), "/")
	if docPath == "" {
		return sourcecapture.Request{}, fmt.Errorf("%w: docs path is required", ErrInvalidConfig)
	}
	requestURL := "https://raw.githubusercontent.com/" + cfg.GitHubRepo + "/" + cfg.DocsRef + "/" + docPath
	return sourcecapture.Request{
		SnapshotKey:      "snapshot:docs:raw:" + safeKey(cfg.GitHubRepo+":"+docPath) + ":" + cfg.DocsRef + ":" + crawlID(cfg.CrawlStartedAt),
		URL:              requestURL,
		SourceKey:        SourceDocs,
		SourceInstance:   docsSourceInstance(cfg.GitHubRepo),
		SourceObjectType: "github_markdown_doc",
		SourceObjectID:   cfg.GitHubRepo + ":" + docPath,
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/blob/" + cfg.DocsRef + "/" + docPath,
	}, nil
}

// JiraAutoscalerJQL returns the bounded source query for the current POC slice.
func JiraAutoscalerJQL(since time.Time, crawlStartedAt time.Time) string {
	return fmt.Sprintf(`project = FLINK AND component = "Autoscaler" AND updated >= "%s" AND updated <= "%s" ORDER BY updated ASC, key ASC`, date(since), date(crawlStartedAt))
}

// IssueKeysFromJiraSearch extracts issue keys from a Jira search page snapshot.
func IssueKeysFromJiraSearch(record sourcecapture.Record) ([]string, error) {
	var page struct {
		Issues []struct {
			Key string `json:"key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(record.Body, &page); err != nil {
		return nil, fmt.Errorf("decode Jira search snapshot %s: %w", record.SnapshotKey, err)
	}
	keys := make([]string, 0, len(page.Issues))
	for _, issue := range page.Issues {
		keys = append(keys, issue.Key)
	}
	return NormalizeIssueKeys(keys), nil
}

// PRURLsFromJiraRemoteLinks extracts GitHub PR URLs from a Jira remote-link snapshot.
func PRURLsFromJiraRemoteLinks(record sourcecapture.Record) ([]string, error) {
	var links []struct {
		Object struct {
			URL string `json:"url"`
		} `json:"object"`
	}
	if err := json.Unmarshal(record.Body, &links); err != nil {
		return nil, fmt.Errorf("decode Jira remote-link snapshot %s: %w", record.SnapshotKey, err)
	}
	urls := make([]string, 0, len(links))
	for _, link := range links {
		if _, ok := ParseGitHubPRURL(link.Object.URL); ok {
			urls = append(urls, link.Object.URL)
		}
	}
	sort.Strings(urls)
	return urls, nil
}

// PullRequestsFromGitHubSearch extracts PR refs from a GitHub search snapshot.
func PullRequestsFromGitHubSearch(record sourcecapture.Record) ([]PullRequestRef, error) {
	var page struct {
		Items []struct {
			HTMLURL string `json:"html_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(record.Body, &page); err != nil {
		return nil, fmt.Errorf("decode GitHub search snapshot %s: %w", record.SnapshotKey, err)
	}
	refs := make([]PullRequestRef, 0, len(page.Items))
	for _, item := range page.Items {
		if ref, ok := ParseGitHubPRURL(item.HTMLURL); ok {
			refs = append(refs, ref)
		}
	}
	return dedupePRRefs(refs), nil
}

// MarkdownDocsFromTree extracts docs/content/docs markdown paths from a GitHub tree snapshot.
func MarkdownDocsFromTree(record sourcecapture.Record) ([]string, error) {
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(record.Body, &tree); err != nil {
		return nil, fmt.Errorf("decode docs tree snapshot %s: %w", record.SnapshotKey, err)
	}
	paths := make([]string, 0)
	for _, entry := range tree.Tree {
		if entry.Type == "blob" && strings.HasPrefix(entry.Path, "docs/content/docs/") && strings.HasSuffix(entry.Path, ".md") {
			paths = append(paths, entry.Path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// ExtractIssueKeys returns unique FLINK issue keys from source text.
func ExtractIssueKeys(text string) []string {
	return NormalizeIssueKeys(issueKeyPattern.FindAllString(strings.ToUpper(text), -1))
}

// NormalizeIssueKeys uppercases, dedupes, filters, and sorts FLINK issue keys.
func NormalizeIssueKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := strings.ToUpper(strings.TrimSpace(key))
		if !exactIssueKeyPattern.MatchString(normalized) {
			continue
		}
		seen[normalized] = struct{}{}
	}
	normalized := make([]string, 0, len(seen))
	for key := range seen {
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)
	return normalized
}

// PullRequestRef identifies a GitHub pull request without depending on API response shape.
type PullRequestRef struct {
	Repo   string
	Number int
}

// ParseGitHubPRURL extracts repo and number from a GitHub pull request URL.
func ParseGitHubPRURL(rawURL string) (PullRequestRef, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host != "github.com" {
		return PullRequestRef{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return PullRequestRef{}, false
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return PullRequestRef{}, false
	}
	return PullRequestRef{Repo: parts[0] + "/" + parts[1], Number: number}, true
}

// withDefaults fills the bounded Flink crawl plan without hiding caller overrides.
func (cfg Config) withDefaults() Config {
	defaults := DefaultConfig(cfg.CrawlStartedAt)
	if cfg.JiraBaseURL == "" {
		cfg.JiraBaseURL = defaults.JiraBaseURL
	}
	if cfg.JiraSince.IsZero() {
		cfg.JiraSince = defaults.JiraSince
	}
	if cfg.JiraPageSize == 0 {
		cfg.JiraPageSize = defaults.JiraPageSize
	}
	if cfg.GitHubAPIBaseURL == "" {
		cfg.GitHubAPIBaseURL = defaults.GitHubAPIBaseURL
	}
	if cfg.GitHubRepo == "" {
		cfg.GitHubRepo = defaults.GitHubRepo
	}
	if cfg.GitHubSeedSince.IsZero() {
		cfg.GitHubSeedSince = defaults.GitHubSeedSince
	}
	if cfg.GitHubPageSize == 0 {
		cfg.GitHubPageSize = defaults.GitHubPageSize
	}
	if cfg.DocsRef == "" {
		cfg.DocsRef = defaults.DocsRef
	}
	return cfg
}

// validate rejects source plans that would make coverage claims ambiguous.
func (cfg Config) validate() error {
	if cfg.CrawlStartedAt.IsZero() {
		return fmt.Errorf("%w: crawl_started_at is required", ErrInvalidConfig)
	}
	if cfg.JiraPageSize <= 0 || cfg.JiraPageSize > 100 {
		return fmt.Errorf("%w: jira page size must be between 1 and 100", ErrInvalidConfig)
	}
	if cfg.GitHubPageSize <= 0 || cfg.GitHubPageSize > 100 {
		return fmt.Errorf("%w: github page size must be between 1 and 100", ErrInvalidConfig)
	}
	return nil
}

// dedupePRRefs keeps one stable PR ref before the loader decides what is product truth.
func dedupePRRefs(refs []PullRequestRef) []PullRequestRef {
	seen := make(map[string]PullRequestRef, len(refs))
	for _, ref := range refs {
		if ref.Repo == "" || ref.Number <= 0 {
			continue
		}
		seen[ref.Repo+"#"+strconv.Itoa(ref.Number)] = ref
	}
	out := make([]PullRequestRef, 0, len(seen))
	for _, ref := range seen {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo == out[j].Repo {
			return out[i].Number < out[j].Number
		}
		return out[i].Repo < out[j].Repo
	})
	return out
}

// parseBaseURL checks source API roots before request paths are joined onto them.
func parseBaseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse base URL %q: %v", ErrInvalidConfig, rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base URL %q requires scheme and host", ErrInvalidConfig, rawURL)
	}
	return parsed, nil
}

// joinURLPath builds a source URL while preserving the configured source host.
func joinURLPath(rawBase string, parts string) (string, error) {
	parsed, err := parseBaseURL(rawBase)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Join(parsed.Path, parts)
	return parsed.String(), nil
}

// joinURLPathWithQuery keeps endpoint query strings attached after safe path joining.
func joinURLPathWithQuery(rawBase string, apiPath string) (string, error) {
	parts := strings.SplitN(apiPath, "?", 2)
	joined, err := joinURLPath(rawBase, parts[0])
	if err != nil {
		return "", err
	}
	if len(parts) == 1 {
		return joined, nil
	}
	separator := "?"
	if strings.Contains(joined, "?") {
		separator = "&"
	}
	return joined + separator + parts[1], nil
}

// date turns crawl bounds into the day-granular source query format.
func date(value time.Time) string {
	return value.UTC().Format("2006-01-02")
}

// crawlID names snapshots by crawl time so replay batches stay deterministic.
func crawlID(value time.Time) string {
	return value.UTC().Format("20060102T150405Z")
}

// safeKey turns source IDs into filename- and key-safe fragments.
func safeKey(value string) string {
	replacer := strings.NewReplacer("/", "-", ":", "-", "#", "-", " ", "-")
	return replacer.Replace(value)
}

// githubSourceInstance names the GitHub repo scope used by product source fields.
func githubSourceInstance(repo string) string {
	return "github.com/" + repo
}

// docsSourceInstance names the docs scope separately from PR and issue snapshots.
func docsSourceInstance(repo string) string {
	return "github.com/" + repo + "/docs"
}
