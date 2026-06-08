package flink

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/sourcefetch"
)

const (
	SourceInstanceJira   = "apache-jira"
	SourceInstanceGitHub = "github.com/apache/flink-kubernetes-operator"
	SourceInstanceDocs   = "github.com/apache/flink-kubernetes-operator/docs"

	defaultJiraBaseURL      = "https://issues.apache.org/jira"
	defaultGitHubAPIBaseURL = "https://api.github.com"
	defaultGitHubRepo       = "apache/flink-kubernetes-operator"
	defaultDocsRawBaseURL   = "https://raw.githubusercontent.com/apache/flink-kubernetes-operator/main"
	defaultJiraPageSize     = 50
	defaultGitHubPageSize   = 50
	defaultIssueLimit       = 200
	defaultPRDetailLimit    = 50
)

var ErrInvalidLivePlan = errors.New("invalid Flink live source plan")

type LiveConfig struct {
	Slice            string
	CrawlStartedAt   time.Time
	JiraBaseURL      string
	JiraSince        time.Time
	JiraPageSize     int
	IssueLimit       int
	GitHubAPIBaseURL string
	GitHubRepo       string
	GitHubSeedSince  time.Time
	GitHubPageSize   int
	PRDetailLimit    int
	DocsRawBaseURL   string
	DocsPaths        []string
}

func DefaultLiveConfig() LiveConfig {
	return LiveConfig{
		Slice:            AutoscalerSlice,
		JiraBaseURL:      defaultJiraBaseURL,
		JiraPageSize:     defaultJiraPageSize,
		IssueLimit:       defaultIssueLimit,
		GitHubAPIBaseURL: defaultGitHubAPIBaseURL,
		GitHubRepo:       defaultGitHubRepo,
		GitHubPageSize:   defaultGitHubPageSize,
		PRDetailLimit:    defaultPRDetailLimit,
		DocsRawBaseURL:   defaultDocsRawBaseURL,
		DocsPaths: []string{
			"docs/content/docs/custom-resource/autoscaler.md",
		},
	}
}

func PlanInitialLiveRequests(cfg LiveConfig) ([]sourcefetch.Request, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	jiraSearch, err := planJiraSearchRequest(cfg, 0)
	if err != nil {
		return nil, err
	}
	gitHubSearch, err := planGitHubSeedSearchRequest(cfg)
	if err != nil {
		return nil, err
	}
	requests := []sourcefetch.Request{
		jiraSearch,
		gitHubSearch,
	}
	for _, docsPath := range cfg.DocsPaths {
		docsRequest, err := planDocsRequest(cfg, docsPath)
		if err != nil {
			return nil, err
		}
		requests = append(requests, docsRequest)
	}
	return requests, nil
}

func PlanJiraRemoteLinkRequests(cfg LiveConfig, issueKeys []string) ([]sourcefetch.Request, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validateCrawlStartedAt(); err != nil {
		return nil, err
	}
	keys := normalizeIssueKeys(issueKeys, cfg.IssueLimit)
	requests := make([]sourcefetch.Request, 0, len(keys))
	for _, issueKey := range keys {
		endpoint, err := joinURLPath(cfg.JiraBaseURL, "rest/api/2/issue/"+issueKey+"/remotelink")
		if err != nil {
			return nil, err
		}
		requests = append(requests, sourcefetch.Request{
			URL:              endpoint,
			Source:           SourceJira,
			SourceInstance:   SourceInstanceJira,
			SnapshotKey:      "snapshot:jira:remote-links:" + issueKey + ":" + crawlID(cfg.CrawlStartedAt),
			SourceObjectKind: "jira_remote_links",
			SourceObjectID:   issueKey,
			SourceURL:        "https://issues.apache.org/jira/browse/" + issueKey,
		})
	}
	return requests, nil
}

func PlanGitHubPRDetailRequests(cfg LiveConfig, prURLs []string) ([]sourcefetch.Request, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validateCrawlStartedAt(); err != nil {
		return nil, err
	}
	prs := normalizePRs(prURLs, cfg.GitHubRepo, cfg.PRDetailLimit)
	requests := make([]sourcefetch.Request, 0, len(prs))
	for _, pr := range prs {
		endpoint, err := joinURLPath(cfg.GitHubAPIBaseURL, "repos/"+pr.repo+"/pulls/"+strconv.Itoa(pr.number))
		if err != nil {
			return nil, err
		}
		htmlURL := "https://github.com/" + pr.repo + "/pull/" + strconv.Itoa(pr.number)
		requests = append(requests, sourcefetch.Request{
			URL:              endpoint,
			Source:           SourceGitHub,
			SourceInstance:   SourceInstanceGitHub,
			SnapshotKey:      "snapshot:github:pr:" + safeKey(pr.repo) + ":" + strconv.Itoa(pr.number) + ":" + crawlID(cfg.CrawlStartedAt),
			SourceObjectKind: "github_pull_request",
			SourceObjectID:   pr.repo + "#" + strconv.Itoa(pr.number),
			SourceURL:        htmlURL,
		})
	}
	return requests, nil
}

func (cfg LiveConfig) withDefaults() LiveConfig {
	defaults := DefaultLiveConfig()
	if cfg.Slice == "" {
		cfg.Slice = defaults.Slice
	}
	if cfg.JiraBaseURL == "" {
		cfg.JiraBaseURL = defaults.JiraBaseURL
	}
	if cfg.JiraPageSize == 0 {
		cfg.JiraPageSize = defaults.JiraPageSize
	}
	if cfg.IssueLimit == 0 {
		cfg.IssueLimit = defaults.IssueLimit
	}
	if cfg.GitHubAPIBaseURL == "" {
		cfg.GitHubAPIBaseURL = defaults.GitHubAPIBaseURL
	}
	if cfg.GitHubRepo == "" {
		cfg.GitHubRepo = defaults.GitHubRepo
	}
	if cfg.GitHubPageSize == 0 {
		cfg.GitHubPageSize = defaults.GitHubPageSize
	}
	if cfg.PRDetailLimit == 0 {
		cfg.PRDetailLimit = defaults.PRDetailLimit
	}
	if cfg.DocsRawBaseURL == "" {
		cfg.DocsRawBaseURL = defaults.DocsRawBaseURL
	}
	if cfg.DocsPaths == nil {
		cfg.DocsPaths = defaults.DocsPaths
	}
	return cfg
}

func (cfg LiveConfig) validate() error {
	if err := cfg.validateCrawlStartedAt(); err != nil {
		return err
	}
	if cfg.JiraSince.IsZero() {
		return fmt.Errorf("%w: jira since is required", ErrInvalidLivePlan)
	}
	if cfg.GitHubSeedSince.IsZero() {
		return fmt.Errorf("%w: github seed since is required", ErrInvalidLivePlan)
	}
	if cfg.JiraPageSize <= 0 || cfg.JiraPageSize > 100 {
		return fmt.Errorf("%w: jira page size must be between 1 and 100", ErrInvalidLivePlan)
	}
	if cfg.GitHubPageSize <= 0 || cfg.GitHubPageSize > 100 {
		return fmt.Errorf("%w: github page size must be between 1 and 100", ErrInvalidLivePlan)
	}
	if cfg.IssueLimit < 0 || cfg.PRDetailLimit < 0 {
		return fmt.Errorf("%w: source limits cannot be negative", ErrInvalidLivePlan)
	}
	return nil
}

func (cfg LiveConfig) validateCrawlStartedAt() error {
	if cfg.CrawlStartedAt.IsZero() {
		return fmt.Errorf("%w: crawl started at is required", ErrInvalidLivePlan)
	}
	return nil
}

func planJiraSearchRequest(cfg LiveConfig, startAt int) (sourcefetch.Request, error) {
	endpoint, err := parseBaseURL(cfg.JiraBaseURL)
	if err != nil {
		return sourcefetch.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "rest/api/2/search")
	query := endpoint.Query()
	query.Set("jql", jiraAutoscalerJQL(cfg.JiraSince, cfg.CrawlStartedAt))
	query.Set("fields", "key,summary,status,resolution,assignee,reporter,created,updated,labels,components,fixVersions,priority,issuelinks,parent,subtasks")
	query.Set("maxResults", strconv.Itoa(cfg.JiraPageSize))
	query.Set("startAt", strconv.Itoa(startAt))
	endpoint.RawQuery = query.Encode()
	return sourcefetch.Request{
		URL:              endpoint.String(),
		Source:           SourceJira,
		SourceInstance:   SourceInstanceJira,
		SnapshotKey:      "snapshot:jira:search:" + crawlID(cfg.CrawlStartedAt) + ":start-" + strconv.Itoa(startAt),
		SourceObjectKind: "jira_search_page",
		SourceObjectID:   "start-" + strconv.Itoa(startAt),
		SourceURL:        cfg.JiraBaseURL + "/issues/?jql=" + url.QueryEscape(jiraAutoscalerJQL(cfg.JiraSince, cfg.CrawlStartedAt)),
	}, nil
}

func planGitHubSeedSearchRequest(cfg LiveConfig) (sourcefetch.Request, error) {
	endpoint, err := parseBaseURL(cfg.GitHubAPIBaseURL)
	if err != nil {
		return sourcefetch.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "search/issues")
	query := endpoint.Query()
	query.Set("q", "repo:"+cfg.GitHubRepo+" is:pr FLINK- updated:>="+cfg.GitHubSeedSince.Format("2006-01-02"))
	query.Set("per_page", strconv.Itoa(cfg.GitHubPageSize))
	endpoint.RawQuery = query.Encode()
	return sourcefetch.Request{
		URL:              endpoint.String(),
		Source:           SourceGitHub,
		SourceInstance:   SourceInstanceGitHub,
		SnapshotKey:      "snapshot:github:search:" + crawlID(cfg.CrawlStartedAt),
		SourceObjectKind: "github_pr_search",
		SourceObjectID:   cfg.GitHubRepo,
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/pulls",
	}, nil
}

func planDocsRequest(cfg LiveConfig, docsPath string) (sourcefetch.Request, error) {
	endpoint, err := parseBaseURL(cfg.DocsRawBaseURL)
	if err != nil {
		return sourcefetch.Request{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, docsPath)
	return sourcefetch.Request{
		URL:              endpoint.String(),
		Source:           SourceDocs,
		SourceInstance:   SourceInstanceDocs,
		SnapshotKey:      "snapshot:docs:" + safeKey(docsPath) + ":" + crawlID(cfg.CrawlStartedAt),
		SourceObjectKind: "docs_markdown",
		SourceObjectID:   docsPath,
		SourceURL:        "https://github.com/" + cfg.GitHubRepo + "/blob/main/" + docsPath,
	}, nil
}

func jiraAutoscalerJQL(since, crawlStartedAt time.Time) string {
	return `project = FLINK AND component = "Autoscaler" AND updated >= "` +
		since.UTC().Format("2006-01-02 15:04") +
		`" AND updated <= "` +
		crawlStartedAt.UTC().Format("2006-01-02 15:04") +
		`" ORDER BY updated ASC, key ASC`
}

func normalizeIssueKeys(issueKeys []string, limit int) []string {
	seen := make(map[string]bool)
	keys := make([]string, 0, len(issueKeys))
	re := regexp.MustCompile(`^FLINK-\d+$`)
	for _, issueKey := range issueKeys {
		issueKey = strings.ToUpper(strings.TrimSpace(issueKey))
		if !re.MatchString(issueKey) || seen[issueKey] {
			continue
		}
		seen[issueKey] = true
		keys = append(keys, issueKey)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		return keys[:limit]
	}
	return keys
}

type plannedPR struct {
	repo   string
	number int
}

func normalizePRs(rawURLs []string, expectedRepo string, limit int) []plannedPR {
	seen := make(map[string]bool)
	prs := make([]plannedPR, 0, len(rawURLs))
	for _, rawURL := range rawURLs {
		repo, number, ok := parseGitHubPR(rawURL)
		if !ok || repo != expectedRepo {
			continue
		}
		key := repo + "#" + strconv.Itoa(number)
		if seen[key] {
			continue
		}
		seen[key] = true
		prs = append(prs, plannedPR{repo: repo, number: number})
	}
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].repo == prs[j].repo {
			return prs[i].number < prs[j].number
		}
		return prs[i].repo < prs[j].repo
	})
	if limit > 0 && len(prs) > limit {
		return prs[:limit]
	}
	return prs
}

func joinURLPath(baseURL, suffix string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Join(parsed.Path, suffix)
	return parsed.String(), nil
}

func parseBaseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base url %q: %v", ErrInvalidLivePlan, rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid base url %q", ErrInvalidLivePlan, rawURL)
	}
	return parsed, nil
}

func crawlID(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func safeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("/", "-", "#", "-", ":", "-", " ", "-", ".", "-")
	return replacer.Replace(value)
}
