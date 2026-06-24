// Association:
//
//	ontology-service serve -> entstore.Open -> Ent client -> ontology hooks
//	ontology hooks -> WorkLensWindow -> typed lens result rows
//
// The command wires the runtime path that makes Ent schema and hook invariants
// active before HTTP traffic reaches the ontology graph.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/sourceconnection"
	"cubicle/services/ontology-service/ent/sourcescope"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/internal/config"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/entgraph"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcegraph"
	"cubicle/services/ontology-service/internal/graphcontext"
	ontologygraphql "cubicle/services/ontology-service/internal/graphql"
	graphqlmodel "cubicle/services/ontology-service/internal/graphql/model"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/ontology"
	"cubicle/services/ontology-service/internal/opengraphfixture"
	"cubicle/services/ontology-service/internal/sampledata"
)

// serveConfig is the runtime path: config/env/flags -> Ent store -> HTTP API.
type serveConfig struct {
	ConfigPath               string        // ConfigPath is the optional HOCON file path used to load runtime defaults.
	Listen                   string        // Listen is the host:port address the local HTTP server binds to.
	AllowPublicBind          bool          // AllowPublicBind permits non-localhost binds for explicit development use.
	DatabasePath             string        // DatabasePath is the SQLite file path used for local ontology storage.
	SQLiteBusyTimeout        time.Duration // SQLiteBusyTimeout is SQLite's local lock wait timeout.
	GraphQLPlaygroundEnabled bool          // GraphQLPlaygroundEnabled controls whether GET /playground is mounted.
}

// flinkFixtureSummaryConfig points at a source-capture fixture for coverage counts.
type flinkFixtureSummaryConfig struct {
	Dir string // Dir is a Flink source-capture fixture directory.
}

// flinkFixtureDeriveConfig bounds a large fixture to selected PRs and Jira keys.
type flinkFixtureDeriveConfig struct {
	SourceDir          string // SourceDir is the source fixture directory.
	OutDir             string // OutDir is the derived replay fixture directory.
	GitHubRepo         string // GitHubRepo is the selected PR repository.
	PullRequestNumbers []int  // PullRequestNumbers bounds PR source records.
	JiraKeys           []string
}

// flinkFixtureEnrichConfig fetches missing GitHub detail endpoints into a replay fixture.
type flinkFixtureEnrichConfig struct {
	Dir                string
	GitHubRepo         string
	PullRequestNumbers []int
	GitHubTokenEnv     string
	MaxRequests        int
	MaxBytes           int64
	HTTPTimeout        time.Duration
}

// flinkFixtureLoadConfig points at a fixture and database for graph materialization.
type flinkFixtureLoadConfig struct {
	Dir                string        // Dir is a Flink source-capture fixture directory.
	DatabasePath       string        // DatabasePath is the SQLite graph database to materialize into.
	SQLiteBusyTimeout  time.Duration // SQLiteBusyTimeout is SQLite's local lock wait timeout.
	StreamKey          string        // StreamKey overrides the default fixture stream key.
	RunKey             string        // RunKey overrides the generated source sync run key.
	GitHubRepo         string        // GitHubRepo is the selected PR repository for optional filtering.
	PullRequestNumbers []int         // PullRequestNumbers optionally bounds PR source records before loading.
	JiraKeys           []string      // JiraKeys optionally bounds Jira records before loading.
}

// workProgramGraphContextExportConfig points at typed graph rows for a bounded LLM context export.
type workProgramGraphContextExportConfig struct {
	DatabasePath      string
	SQLiteBusyTimeout time.Duration
	OutPath           string
	WorkstreamKey     string
	SourceInstance    string
	RunKey            string
	GeneratedAt       string
	ItemLimit         int
	ActionLimit       int
	EdgeLimit         int
	InsightLimit      int
	ForecastLimit     int
	EvidenceLimit     int
	TraversalDepth    int
}

// boundedGraphContextExportConfig points at the source-neutral graphstore demo path.
type boundedGraphContextExportConfig struct {
	DatabasePath                  string
	SQLiteBusyTimeout             time.Duration
	OutPath                       string
	Fixture                       string
	SourceAuthorityPath           string
	PrincipalKey                  string
	AllowedVisibilityClasses      []string
	PrincipalCoverageComplete     bool
	StartObjectType               string
	StartKey                      string
	AssociationTypes              []domain.AssociationType
	Depth                         int
	LimitPerObject                int
	SeedFixture                   bool
	ResetDatabase                 bool
	CoverageState                 string
	AbsenceClaimsAllowed          bool
	AbsenceClaimGateReason        string
	AbsenceClaimAssociationTypes  []string
	CoverageSourceSystem          string
	CoverageSourceInstance        string
	CoverageWindowStart           string
	CoverageWindowEnd             string
	CoverageSummary               string
	Guardrails                    []string
	AssociationClaimMinConfidence float64
}

// openGraphFixtureLoadConfig loads connector-shaped open graph rows from JSON.
type openGraphFixtureLoadConfig struct {
	FixturePath       string
	DatabasePath      string
	SQLiteBusyTimeout time.Duration
	ResetDatabase     bool
}

// sourceScopeRegisterConfig records a configured source scope before a worker attempts it.
type sourceScopeRegisterConfig struct {
	DatabasePath      string
	SQLiteBusyTimeout time.Duration
	SourceSystem      string
	SourceInstance    string
	DisplayName       string
	ConnectorKind     string
	ScopeKind         string
	ScopeKey          string
	ScopeDisplayName  string
	CrawlPolicy       string
	Enabled           bool
}

// fixtureSummary reports replay coverage before product rows are written.
type fixtureSummary struct {
	Total    int             `json:"total"`
	Sources  []summaryBucket `json:"sources"`
	Statuses []summaryBucket `json:"statuses"`
}

// summaryBucket is one source or status count in a fixture summary.
type summaryBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

const (
	openGraphFixtureName = "open-graph"

	// serverReadHeaderTimeout bounds how long the server waits for request headers.
	serverReadHeaderTimeout = 5 * time.Second

	// serverReadTimeout bounds how long the server spends reading the full request.
	serverReadTimeout = 15 * time.Second

	// serverWriteTimeout bounds how long the server spends writing a response.
	serverWriteTimeout = 30 * time.Second

	// serverIdleTimeout bounds how long idle keep-alive connections stay open.
	serverIdleTimeout = 60 * time.Second
)

// main routes CLI failures through structured logs and process exit.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("ontology_service_exit", "error", err)
		os.Exit(1)
	}
}

// run chooses the command path: serve, summarize fixture, or load fixture.
func run(args []string, logger *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("expected command: serve")
	}

	switch args[0] {
	case "serve":
		cfg, err := parseServeConfig(args)
		if err != nil {
			return err
		}
		return serve(cfg, logger)
	case "flink-fixture-summary":
		cfg, err := parseFlinkFixtureSummaryConfig(args)
		if err != nil {
			return err
		}
		return summarizeFlinkFixture(cfg, os.Stdout)
	case "flink-fixture-derive":
		cfg, err := parseFlinkFixtureDeriveConfig(args)
		if err != nil {
			return err
		}
		return deriveFlinkFixture(cfg, os.Stdout)
	case "flink-fixture-enrich":
		cfg, err := parseFlinkFixtureEnrichConfig(args)
		if err != nil {
			return err
		}
		return enrichFlinkFixture(context.Background(), cfg, os.Stdout)
	case "flink-fixture-load":
		cfg, err := parseFlinkFixtureLoadConfig(args)
		if err != nil {
			return err
		}
		return loadFlinkFixture(context.Background(), cfg, os.Stdout)
	case "work-program-graph-context-export":
		cfg, err := parseWorkProgramGraphContextExportConfig(args)
		if err != nil {
			return err
		}
		return exportWorkProgramGraphContext(context.Background(), cfg, os.Stdout)
	case "bounded-graph-context-export":
		cfg, err := parseBoundedGraphContextExportConfig(args)
		if err != nil {
			return err
		}
		return exportBoundedGraphContext(context.Background(), cfg, os.Stdout)
	case "open-graph-fixture-load":
		cfg, err := parseOpenGraphFixtureLoadConfig(args)
		if err != nil {
			return err
		}
		return loadOpenGraphFixture(context.Background(), cfg, os.Stdout)
	case "source-scope-register":
		cfg, err := parseSourceScopeRegisterConfig(args)
		if err != nil {
			return err
		}
		return registerSourceScope(context.Background(), cfg, os.Stdout)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// parseServeConfig loads serve config using the real process environment.
func parseServeConfig(args []string) (serveConfig, error) {
	return parseServeConfigWithEnv(args, os.Getenv)
}

// parseServeConfigWithEnv merges config file, env, and flags for the HTTP runtime.
func parseServeConfigWithEnv(args []string, getenv func(string) string) (serveConfig, error) {
	configPath, err := configPathFromArgs(args)
	if err != nil {
		return serveConfig{}, err
	}
	appCfg, err := config.LoadWithOptions(config.LoadOptions{
		ConfigPath: configPath,
		Getenv:     getenv,
	})
	if err != nil {
		return serveConfig{}, err
	}

	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	cfg := serveConfig{
		ConfigPath:               appCfg.ConfigPath,
		Listen:                   appCfg.ListenAddr,
		AllowPublicBind:          appCfg.AllowPublicBind,
		DatabasePath:             appCfg.DatabasePath,
		SQLiteBusyTimeout:        appCfg.SQLiteBusyTimeout,
		GraphQLPlaygroundEnabled: appCfg.GraphQLPlaygroundEnabled,
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.BoolVar(&cfg.GraphQLPlaygroundEnabled, "graphql-playground", cfg.GraphQLPlaygroundEnabled, "mount the local GraphQL playground")
	flags.BoolVar(&cfg.AllowPublicBind, "allow-public-bind", cfg.AllowPublicBind, "allow binding outside localhost for development")
	if err := flags.Parse(args[1:]); err != nil {
		return serveConfig{}, err
	}
	if err := validateListenAddress(cfg.Listen, cfg.AllowPublicBind); err != nil {
		return serveConfig{}, err
	}
	return cfg, nil
}

// configPathFromArgs finds --config before loading env-backed defaults.
func configPathFromArgs(args []string) (string, error) {
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 >= len(args) {
				return "", errors.New("missing value for --config")
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config="), nil
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config="), nil
		}
	}
	return "", nil
}

// parseFlinkFixtureSummaryConfig validates the fixture summary command inputs.
func parseFlinkFixtureSummaryConfig(args []string) (flinkFixtureSummaryConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-summary", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var cfg flinkFixtureSummaryConfig
	flags.StringVar(&cfg.Dir, "dir", "", "Flink source-capture fixture directory")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureSummaryConfig{}, err
	}
	if cfg.Dir == "" {
		return flinkFixtureSummaryConfig{}, errors.New("missing required --dir")
	}
	return cfg, nil
}

// parseFlinkFixtureDeriveConfig validates the bounded fixture derivation inputs.
func parseFlinkFixtureDeriveConfig(args []string) (flinkFixtureDeriveConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-derive", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var cfg flinkFixtureDeriveConfig
	var prNumbers string
	var jiraKeys string
	flags.StringVar(&cfg.SourceDir, "source-dir", "", "source Flink fixture directory")
	flags.StringVar(&cfg.OutDir, "out-dir", "", "derived replay fixture directory")
	flags.StringVar(&cfg.GitHubRepo, "github-repo", "", "GitHub repository for selected PRs")
	flags.StringVar(&prNumbers, "pr-numbers", "", "comma-separated GitHub pull request numbers")
	flags.StringVar(&jiraKeys, "jira-keys", "", "comma-separated Jira issue keys to include")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureDeriveConfig{}, err
	}
	if cfg.SourceDir == "" {
		return flinkFixtureDeriveConfig{}, errors.New("missing required --source-dir")
	}
	if cfg.OutDir == "" {
		return flinkFixtureDeriveConfig{}, errors.New("missing required --out-dir")
	}
	var err error
	cfg.PullRequestNumbers, err = parseIntCSV(prNumbers)
	if err != nil {
		return flinkFixtureDeriveConfig{}, err
	}
	cfg.JiraKeys = parseStringCSV(jiraKeys)
	if len(cfg.PullRequestNumbers) == 0 && len(cfg.JiraKeys) == 0 {
		return flinkFixtureDeriveConfig{}, errors.New("at least one of --pr-numbers or --jira-keys is required")
	}
	return cfg, nil
}

// parseFlinkFixtureEnrichConfig validates GitHub detail enrichment inputs.
func parseFlinkFixtureEnrichConfig(args []string) (flinkFixtureEnrichConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-enrich", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := flinkFixtureEnrichConfig{
		GitHubTokenEnv: "GITHUB_TOKEN",
		MaxRequests:    100,
		MaxBytes:       100 * 1024 * 1024,
		HTTPTimeout:    30 * time.Second,
	}
	var prNumbers string
	flags.StringVar(&cfg.Dir, "dir", "", "replay fixture directory to enrich")
	flags.StringVar(&cfg.GitHubRepo, "github-repo", "", "GitHub repository for selected PRs")
	flags.StringVar(&prNumbers, "pr-numbers", "", "comma-separated GitHub pull request numbers")
	flags.StringVar(&cfg.GitHubTokenEnv, "github-token-env", cfg.GitHubTokenEnv, "environment variable containing a GitHub token")
	flags.IntVar(&cfg.MaxRequests, "max-requests", cfg.MaxRequests, "maximum source requests")
	flags.Int64Var(&cfg.MaxBytes, "max-bytes", cfg.MaxBytes, "maximum source bytes")
	flags.DurationVar(&cfg.HTTPTimeout, "http-timeout", cfg.HTTPTimeout, "per-request HTTP timeout")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureEnrichConfig{}, err
	}
	if cfg.Dir == "" {
		return flinkFixtureEnrichConfig{}, errors.New("missing required --dir")
	}
	var err error
	cfg.PullRequestNumbers, err = parseIntCSV(prNumbers)
	if err != nil {
		return flinkFixtureEnrichConfig{}, err
	}
	if len(cfg.PullRequestNumbers) == 0 {
		return flinkFixtureEnrichConfig{}, errors.New("missing required --pr-numbers")
	}
	return cfg, nil
}

// parseFlinkFixtureLoadConfig validates the fixture load command inputs.
func parseFlinkFixtureLoadConfig(args []string) (flinkFixtureLoadConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-load", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := flinkFixtureLoadConfig{
		SQLiteBusyTimeout: 5 * time.Second,
	}
	var prNumbers string
	var jiraKeys string
	flags.StringVar(&cfg.Dir, "dir", "", "Flink source-capture fixture directory")
	flags.StringVar(&cfg.DatabasePath, "database", "", "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.StringVar(&cfg.StreamKey, "stream-key", "", "fixture stream key")
	flags.StringVar(&cfg.RunKey, "run-key", "", "source sync run key")
	flags.StringVar(&cfg.GitHubRepo, "github-repo", "", "GitHub repository for selected PRs")
	flags.StringVar(&prNumbers, "pr-numbers", "", "comma-separated GitHub pull request numbers to load")
	flags.StringVar(&jiraKeys, "jira-keys", "", "comma-separated Jira issue keys to load")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureLoadConfig{}, err
	}
	if cfg.Dir == "" {
		return flinkFixtureLoadConfig{}, errors.New("missing required --dir")
	}
	if cfg.DatabasePath == "" {
		return flinkFixtureLoadConfig{}, errors.New("missing required --database")
	}
	var err error
	cfg.PullRequestNumbers, err = parseIntCSV(prNumbers)
	if err != nil {
		return flinkFixtureLoadConfig{}, err
	}
	cfg.JiraKeys = parseStringCSV(jiraKeys)
	return cfg, nil
}

// parseWorkProgramGraphContextExportConfig validates GraphQL-context export inputs.
func parseWorkProgramGraphContextExportConfig(args []string) (workProgramGraphContextExportConfig, error) {
	flags := flag.NewFlagSet("work-program-graph-context-export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := workProgramGraphContextExportConfig{
		SQLiteBusyTimeout: 5 * time.Second,
	}
	flags.StringVar(&cfg.DatabasePath, "database", "", "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.StringVar(&cfg.OutPath, "out", "", "optional output JSON path; stdout is used when omitted")
	flags.StringVar(&cfg.WorkstreamKey, "workstream-key", "", "GraphQL workstream key to export")
	flags.StringVar(&cfg.SourceInstance, "source-instance", "", "optional analytics/source instance scope")
	flags.StringVar(&cfg.RunKey, "run-key", "", "optional work program run key scope")
	flags.StringVar(&cfg.GeneratedAt, "generated-at", "", "optional RFC3339 generated-at run scope")
	flags.IntVar(&cfg.ItemLimit, "item-limit", 0, "optional work item limit")
	flags.IntVar(&cfg.ActionLimit, "action-limit", 0, "optional work action limit")
	flags.IntVar(&cfg.EdgeLimit, "edge-limit", 0, "optional dependency edge limit")
	flags.IntVar(&cfg.InsightLimit, "insight-limit", 0, "optional insight limit")
	flags.IntVar(&cfg.ForecastLimit, "forecast-limit", 0, "optional forecast limit")
	flags.IntVar(&cfg.EvidenceLimit, "evidence-limit", 0, "optional packet evidence limit")
	flags.IntVar(&cfg.TraversalDepth, "traversal-depth", 0, "optional dependency traversal depth")
	if err := flags.Parse(args[1:]); err != nil {
		return workProgramGraphContextExportConfig{}, err
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return workProgramGraphContextExportConfig{}, errors.New("missing required --database")
	}
	if strings.TrimSpace(cfg.WorkstreamKey) == "" {
		return workProgramGraphContextExportConfig{}, errors.New("missing required --workstream-key")
	}
	return cfg, nil
}

// parseBoundedGraphContextExportConfig validates generic graph-context export inputs.
func parseBoundedGraphContextExportConfig(args []string) (boundedGraphContextExportConfig, error) {
	flags := flag.NewFlagSet("bounded-graph-context-export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := boundedGraphContextExportConfig{
		Fixture:                       "generic-doc-message-ticket",
		Depth:                         2,
		LimitPerObject:                4,
		CoverageState:                 "sparse",
		AbsenceClaimsAllowed:          false,
		AbsenceClaimGateReason:        "partial_demo_fixture",
		CoverageSummary:               "Only selected demo graph rows were loaded.",
		AssociationClaimMinConfidence: 1,
	}
	var associationTypes string
	var absenceClaimAssociationTypes string
	var allowedVisibilityClasses string
	var guardrails string
	flags.StringVar(&cfg.DatabasePath, "database", "", "optional SQLite database path for Ent-backed bounded graph export")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout for Ent-backed export")
	flags.StringVar(&cfg.OutPath, "out", "", "optional output JSON path; stdout is used when omitted")
	flags.StringVar(&cfg.Fixture, "fixture", cfg.Fixture, "sample fixture: generic-doc-message-ticket, customer-incident-runbook, flink-autoscaler, company-ai-first-minimum, open-customer-incident, or open-graph")
	flags.StringVar(&cfg.SourceAuthorityPath, "source-authority-json", "", "optional relationship source-authority JSON for Ent-backed bounded graph export")
	flags.StringVar(&cfg.PrincipalKey, "principal-key", "", "principal key for Ent-backed bounded graph read filtering")
	flags.StringVar(&allowedVisibilityClasses, "allowed-visibility-classes", "", "comma-separated non-public visibility classes this principal can read")
	flags.BoolVar(&cfg.PrincipalCoverageComplete, "principal-coverage-complete", false, "mark source coverage as complete for this principal when absence-claim policy is otherwise complete")
	flags.StringVar(&cfg.StartObjectType, "start-object-type", "", "seed object type; defaults to the fixture seed")
	flags.StringVar(&cfg.StartKey, "start-key", "", "seed object key; defaults to the fixture seed")
	flags.StringVar(&associationTypes, "association-types", "", "comma-separated association types to traverse; empty means all")
	flags.IntVar(&cfg.Depth, "depth", cfg.Depth, "bounded traversal depth")
	flags.IntVar(&cfg.LimitPerObject, "limit-per-object", cfg.LimitPerObject, "bounded traversal fanout per object")
	flags.BoolVar(&cfg.SeedFixture, "seed-fixture", cfg.SeedFixture, "seed the selected fixture before Ent-backed export")
	flags.BoolVar(&cfg.ResetDatabase, "reset-database", cfg.ResetDatabase, "remove the SQLite database before Ent-backed fixture seeding")
	flags.StringVar(&cfg.CoverageState, "coverage-state", cfg.CoverageState, "source coverage state for absence-claim gating")
	flags.BoolVar(&cfg.AbsenceClaimsAllowed, "absence-claims-allowed", cfg.AbsenceClaimsAllowed, "allow product absence claims from this bounded context")
	flags.StringVar(&cfg.AbsenceClaimGateReason, "absence-claim-gate-reason", cfg.AbsenceClaimGateReason, "reason absence claims are allowed or gated")
	flags.StringVar(&absenceClaimAssociationTypes, "absence-claim-association-types", "", "comma-separated association types covered for absence-claim policy")
	flags.StringVar(&cfg.CoverageSourceSystem, "coverage-source-system", "", "source system covered by this bounded context")
	flags.StringVar(&cfg.CoverageSourceInstance, "coverage-source-instance", "", "source instance covered by this bounded context")
	flags.StringVar(&cfg.CoverageWindowStart, "coverage-window-start", "", "inclusive RFC3339 source coverage window start")
	flags.StringVar(&cfg.CoverageWindowEnd, "coverage-window-end", "", "exclusive RFC3339 source coverage window end")
	flags.StringVar(&cfg.CoverageSummary, "coverage-summary", cfg.CoverageSummary, "short coverage summary for the context")
	flags.StringVar(&guardrails, "guardrails", "", "comma-separated extra guardrail text")
	flags.Float64Var(&cfg.AssociationClaimMinConfidence, "association-claim-min-confidence", cfg.AssociationClaimMinConfidence, "minimum confidence for association claimAllowed=true")
	if err := flags.Parse(args[1:]); err != nil {
		return boundedGraphContextExportConfig{}, err
	}
	if cfg.Depth < 0 {
		return boundedGraphContextExportConfig{}, errors.New("--depth must be non-negative")
	}
	if cfg.LimitPerObject <= 0 {
		return boundedGraphContextExportConfig{}, errors.New("--limit-per-object must be positive")
	}
	for _, typ := range parseStringCSV(associationTypes) {
		cfg.AssociationTypes = append(cfg.AssociationTypes, domain.AssociationType(typ))
	}
	cfg.AllowedVisibilityClasses = parseStringCSV(allowedVisibilityClasses)
	cfg.AbsenceClaimAssociationTypes = parseStringCSV(absenceClaimAssociationTypes)
	cfg.Guardrails = parseStringCSV(guardrails)
	return cfg, nil
}

// parseOpenGraphFixtureLoadConfig validates the open graph JSON fixture loader.
func parseOpenGraphFixtureLoadConfig(args []string) (openGraphFixtureLoadConfig, error) {
	flags := flag.NewFlagSet("open-graph-fixture-load", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := openGraphFixtureLoadConfig{
		SQLiteBusyTimeout: 5 * time.Second,
	}
	flags.StringVar(&cfg.FixturePath, "fixture-json", "", "open graph fixture JSON path")
	flags.StringVar(&cfg.DatabasePath, "database", "", "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.BoolVar(&cfg.ResetDatabase, "reset-database", cfg.ResetDatabase, "remove the SQLite database before loading")
	if err := flags.Parse(args[1:]); err != nil {
		return openGraphFixtureLoadConfig{}, err
	}
	if strings.TrimSpace(cfg.FixturePath) == "" {
		return openGraphFixtureLoadConfig{}, errors.New("missing required --fixture-json")
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return openGraphFixtureLoadConfig{}, errors.New("missing required --database")
	}
	return cfg, nil
}

// parseSourceScopeRegisterConfig validates the source-scope registration inputs.
func parseSourceScopeRegisterConfig(args []string) (sourceScopeRegisterConfig, error) {
	flags := flag.NewFlagSet("source-scope-register", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := sourceScopeRegisterConfig{
		SQLiteBusyTimeout: 5 * time.Second,
		ConnectorKind:     "source_scope_registration",
		Enabled:           true,
	}
	flags.StringVar(&cfg.DatabasePath, "database", "", "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.StringVar(&cfg.SourceSystem, "source-system", "", "source system family, such as jira, github, slack, or google_drive")
	flags.StringVar(&cfg.SourceInstance, "source-instance", "", "tenant, workspace, repository owner, or installation namespace")
	flags.StringVar(&cfg.DisplayName, "display-name", "", "human-readable source connection name")
	flags.StringVar(&cfg.ConnectorKind, "connector-kind", cfg.ConnectorKind, "connector implementation key")
	flags.StringVar(&cfg.ScopeKind, "scope-kind", "", "source-local scope kind, such as project, channel, repository, or folder")
	flags.StringVar(&cfg.ScopeKey, "scope-key", "", "source-local scope identifier")
	flags.StringVar(&cfg.ScopeDisplayName, "scope-display-name", "", "human-readable source scope name")
	flags.StringVar(&cfg.CrawlPolicy, "crawl-policy", "", "connector-specific policy key for how this scope should be synced")
	flags.BoolVar(&cfg.Enabled, "enabled", cfg.Enabled, "whether sync workers may process this scope")
	if err := flags.Parse(args[1:]); err != nil {
		return sourceScopeRegisterConfig{}, err
	}
	cfg.SourceSystem = strings.TrimSpace(cfg.SourceSystem)
	cfg.SourceInstance = strings.TrimSpace(cfg.SourceInstance)
	cfg.DisplayName = strings.TrimSpace(cfg.DisplayName)
	cfg.ConnectorKind = strings.TrimSpace(cfg.ConnectorKind)
	cfg.ScopeKind = strings.TrimSpace(cfg.ScopeKind)
	cfg.ScopeKey = strings.TrimSpace(cfg.ScopeKey)
	cfg.ScopeDisplayName = strings.TrimSpace(cfg.ScopeDisplayName)
	cfg.CrawlPolicy = strings.TrimSpace(cfg.CrawlPolicy)
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return sourceScopeRegisterConfig{}, errors.New("missing required --database")
	}
	if cfg.SourceSystem == "" {
		return sourceScopeRegisterConfig{}, errors.New("missing required --source-system")
	}
	if cfg.SourceInstance == "" {
		return sourceScopeRegisterConfig{}, errors.New("missing required --source-instance")
	}
	if cfg.ScopeKind == "" {
		return sourceScopeRegisterConfig{}, errors.New("missing required --scope-kind")
	}
	if cfg.ScopeKey == "" {
		return sourceScopeRegisterConfig{}, errors.New("missing required --scope-key")
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.SourceSystem + " " + cfg.SourceInstance
	}
	if cfg.ScopeDisplayName == "" {
		cfg.ScopeDisplayName = cfg.DisplayName + " " + cfg.ScopeKind + " " + cfg.ScopeKey
	}
	return cfg, nil
}

// summarizeFlinkFixture counts replay coverage without touching the product graph.
func summarizeFlinkFixture(cfg flinkFixtureSummaryConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.Dir)
	if err != nil {
		return err
	}
	summary := fixtureSummary{
		Total: len(records),
		Sources: countBuckets(records, func(key recordStatusKey) string {
			return key.source
		}),
		Statuses: countBuckets(records, func(key recordStatusKey) string {
			return strconv.Itoa(key.status)
		}),
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

// loadOpenGraphFixture loads generic connector-shaped rows into Ent open graph tables.
func loadOpenGraphFixture(ctx context.Context, cfg openGraphFixtureLoadConfig, writer io.Writer) error {
	databasePath := strings.TrimSpace(cfg.DatabasePath)
	if databasePath == "" {
		return errors.New("missing required --database")
	}
	if cfg.ResetDatabase {
		if err := resetSQLiteDatabaseFiles(databasePath); err != nil {
			return err
		}
	}
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: databasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()
	summary, err := opengraphfixture.LoadFile(ctx, graphStore.Client(), strings.TrimSpace(cfg.FixturePath))
	if err != nil {
		return err
	}
	return writeJSON(writer, map[string]any{
		"openGraphFixtureLoad": summary,
	})
}

type sourceScopeRegisterResult struct {
	SourceConnectionID int    `json:"source_connection_id"`
	SourceScopeID      int    `json:"source_scope_id"`
	SourceScopeStateID int    `json:"source_scope_state_id"`
	ConnectionCreated  bool   `json:"connection_created"`
	ScopeCreated       bool   `json:"scope_created"`
	StateCreated       bool   `json:"state_created"`
	FreshnessState     string `json:"freshness_state"`
	CoverageMode       string `json:"coverage_mode"`
	LastAttemptedAt    string `json:"last_attempted_at,omitempty"`
	SyncRunCount       int    `json:"sync_run_count"`
}

// registerSourceScope records planned source coverage without claiming a sync attempt.
func registerSourceScope(ctx context.Context, cfg sourceScopeRegisterConfig, writer io.Writer) error {
	databasePath := strings.TrimSpace(cfg.DatabasePath)
	if databasePath == "" {
		return errors.New("missing required --database")
	}
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: databasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()

	conn, connectionCreated, err := ensureRegisteredSourceConnection(ctx, graphStore.Client(), cfg)
	if err != nil {
		return err
	}
	scope, scopeCreated, err := ensureRegisteredSourceScope(ctx, graphStore.Client(), conn.ID, cfg)
	if err != nil {
		return err
	}
	state, stateCreated, err := ensureRegisteredSourceScopeState(ctx, graphStore.Client(), scope.ID)
	if err != nil {
		return err
	}
	syncRunCount, err := scope.QuerySyncRuns().Count(ctx)
	if err != nil {
		return fmt.Errorf("count source sync runs for scope %d: %w", scope.ID, err)
	}
	result := sourceScopeRegisterResult{
		SourceConnectionID: conn.ID,
		SourceScopeID:      scope.ID,
		SourceScopeStateID: state.ID,
		ConnectionCreated:  connectionCreated,
		ScopeCreated:       scopeCreated,
		StateCreated:       stateCreated,
		FreshnessState:     state.FreshnessState.String(),
		CoverageMode:       state.CoverageMode.String(),
		SyncRunCount:       syncRunCount,
	}
	if !state.LastAttemptedAt.IsZero() {
		result.LastAttemptedAt = state.LastAttemptedAt.UTC().Format(time.RFC3339)
	}
	return writeJSON(writer, map[string]any{
		"sourceScopeRegister": result,
	})
}

func ensureRegisteredSourceConnection(ctx context.Context, client *genent.Client, cfg sourceScopeRegisterConfig) (*genent.SourceConnection, bool, error) {
	conn, err := client.SourceConnection.Query().Where(
		sourceconnection.SourceSystemEQ(cfg.SourceSystem),
		sourceconnection.SourceInstanceEQ(cfg.SourceInstance),
	).Only(ctx)
	if err == nil {
		updater := conn.Update().
			SetDisplayName(cfg.DisplayName).
			SetIsEnabled(cfg.Enabled)
		if cfg.ConnectorKind != "" {
			updater.SetConnectorKind(cfg.ConnectorKind)
		} else {
			updater.ClearConnectorKind()
		}
		updated, err := updater.Save(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("update source connection %s/%s: %w", cfg.SourceSystem, cfg.SourceInstance, err)
		}
		return updated, false, nil
	}
	if !genent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query source connection %s/%s: %w", cfg.SourceSystem, cfg.SourceInstance, err)
	}
	builder := client.SourceConnection.Create().
		SetKey(registeredSourceConnectionKey(cfg.SourceSystem, cfg.SourceInstance)).
		SetSourceSystem(cfg.SourceSystem).
		SetSourceInstance(cfg.SourceInstance).
		SetDisplayName(cfg.DisplayName).
		SetIsEnabled(cfg.Enabled)
	if cfg.ConnectorKind != "" {
		builder.SetConnectorKind(cfg.ConnectorKind)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create source connection %s/%s: %w", cfg.SourceSystem, cfg.SourceInstance, err)
	}
	return created, true, nil
}

func ensureRegisteredSourceScope(ctx context.Context, client *genent.Client, sourceConnectionID int, cfg sourceScopeRegisterConfig) (*genent.SourceScope, bool, error) {
	scope, err := client.SourceScope.Query().Where(
		sourcescope.SourceConnectionIDEQ(sourceConnectionID),
		sourcescope.ScopeKindEQ(cfg.ScopeKind),
		sourcescope.ScopeKeyEQ(cfg.ScopeKey),
	).Only(ctx)
	if err == nil {
		updater := scope.Update().
			SetDisplayName(cfg.ScopeDisplayName).
			SetIsEnabled(cfg.Enabled)
		if cfg.CrawlPolicy != "" {
			updater.SetCrawlPolicy(cfg.CrawlPolicy)
		} else {
			updater.ClearCrawlPolicy()
		}
		updated, err := updater.Save(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("update source scope %s/%s: %w", cfg.ScopeKind, cfg.ScopeKey, err)
		}
		return updated, false, nil
	}
	if !genent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query source scope %s/%s: %w", cfg.ScopeKind, cfg.ScopeKey, err)
	}
	builder := client.SourceScope.Create().
		SetKey(registeredSourceScopeKey(cfg.SourceSystem, cfg.SourceInstance, cfg.ScopeKind, cfg.ScopeKey)).
		SetSourceConnectionID(sourceConnectionID).
		SetScopeKind(cfg.ScopeKind).
		SetScopeKey(cfg.ScopeKey).
		SetDisplayName(cfg.ScopeDisplayName).
		SetIsEnabled(cfg.Enabled)
	if cfg.CrawlPolicy != "" {
		builder.SetCrawlPolicy(cfg.CrawlPolicy)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create source scope %s/%s: %w", cfg.ScopeKind, cfg.ScopeKey, err)
	}
	return created, true, nil
}

func ensureRegisteredSourceScopeState(ctx context.Context, client *genent.Client, sourceScopeID int) (*genent.SourceScopeState, bool, error) {
	state, err := client.SourceScopeState.Query().Where(sourcescopestate.SourceScopeIDEQ(sourceScopeID)).Only(ctx)
	if err == nil {
		return state, false, nil
	}
	if !genent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query source scope state for scope %d: %w", sourceScopeID, err)
	}
	created, err := client.SourceScopeState.Create().
		SetSourceScopeID(sourceScopeID).
		SetFreshnessState(sourcescopestate.FreshnessStateUnknown).
		SetCoverageMode(sourcescopestate.CoverageModeUnknown).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create source scope state for scope %d: %w", sourceScopeID, err)
	}
	return created, true, nil
}

func registeredSourceConnectionKey(sourceSystem string, sourceInstance string) string {
	return "source-connection:" + sourceSystem + ":" + sourceInstance
}

func registeredSourceScopeKey(sourceSystem string, sourceInstance string, scopeKind string, scopeKey string) string {
	return "source-scope:" + sourceSystem + ":" + sourceInstance + ":" + scopeKind + ":" + scopeKey
}

// deriveFlinkFixture writes a smaller replay fixture for a selected PR/Jira slice.
func deriveFlinkFixture(cfg flinkFixtureDeriveConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.SourceDir)
	if err != nil {
		return err
	}
	filtered, err := sourcegraph.FilterRecords(records, sourcegraph.RecordFilterOptions{
		GitHubRepo:         cfg.GitHubRepo,
		PullRequestNumbers: cfg.PullRequestNumbers,
		JiraKeys:           cfg.JiraKeys,
	})
	if err != nil {
		return err
	}
	if err := sourcecapture.WriteManifest(cfg.OutDir, filtered, sourcecapture.DumpOptions{GeneratedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return writeJSON(writer, map[string]any{
		"source_dir": cfg.SourceDir,
		"out_dir":    cfg.OutDir,
		"records":    len(filtered),
		"sources": countBuckets(filtered, func(key recordStatusKey) string {
			return key.source
		}),
		"statuses": countBuckets(filtered, func(key recordStatusKey) string {
			return strconv.Itoa(key.status)
		}),
	})
}

// enrichFlinkFixture fetches missing GitHub detail endpoints into a replay fixture.
func enrichFlinkFixture(ctx context.Context, cfg flinkFixtureEnrichConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.Dir)
	if err != nil {
		return err
	}
	repo := cfg.GitHubRepo
	if repo == "" {
		repo = "apache/flink-kubernetes-operator"
	}
	existing := existingSuccessfulRecordTypes(records)
	var requests []sourcecapture.Request
	now := time.Now().UTC()
	sourceCfg := sourcegraph.DefaultConfig(now)
	sourceCfg.GitHubRepo = repo
	for _, number := range cfg.PullRequestNumbers {
		bundleRequests, err := sourcegraph.PlanGitHubPRBundle(sourceCfg, sourcegraph.PullRequestRef{Repo: repo, Number: number})
		if err != nil {
			return err
		}
		for _, request := range bundleRequests {
			if existing[request.SourceObjectID+"\x00"+request.SourceObjectType] {
				continue
			}
			if token := githubTokenFromEnv(cfg.GitHubTokenEnv); token != "" {
				if request.Headers == nil {
					request.Headers = make(map[string]string)
				}
				request.Headers["Authorization"] = "Bearer " + token
			}
			requests = append(requests, request)
		}
	}
	fetcher := sourcecapture.Fetcher{
		Client: &http.Client{Timeout: cfg.HTTPTimeout},
		Budget: sourcecapture.Budget{
			MaxRequests: cfg.MaxRequests,
			MaxBytes:    cfg.MaxBytes,
		},
		Now: func() time.Time { return time.Now().UTC() },
	}
	fetched, usage, fetchErr := fetcher.FetchAll(ctx, requests)
	merged := mergeFixtureRecords(records, fetched)
	if err := sourcecapture.WriteManifest(cfg.Dir, merged, sourcecapture.DumpOptions{GeneratedAt: time.Now().UTC()}); err != nil {
		return err
	}
	result := map[string]any{
		"dir":                 cfg.Dir,
		"planned_requests":    len(requests),
		"fetched_records":     len(fetched),
		"merged_records":      len(merged),
		"usage":               usage,
		"rate_limited":        false,
		"partial_fetch_error": "",
	}
	if fetchErr != nil {
		var rateLimitErr sourcecapture.RateLimitError
		if errors.As(fetchErr, &rateLimitErr) {
			result["rate_limited"] = true
			result["partial_fetch_error"] = rateLimitErr.Error()
			return writeJSON(writer, result)
		}
		if len(fetched) > 0 {
			result["partial_fetch_error"] = fetchErr.Error()
			return writeJSON(writer, result)
		}
		return fetchErr
	}
	return writeJSON(writer, result)
}

// loadFlinkFixture replays captured source bytes into typed ontology rows.
func loadFlinkFixture(ctx context.Context, cfg flinkFixtureLoadConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.Dir)
	if err != nil {
		return err
	}
	if len(cfg.PullRequestNumbers) > 0 || len(cfg.JiraKeys) > 0 {
		records, err = sourcegraph.FilterRecords(records, sourcegraph.RecordFilterOptions{
			GitHubRepo:         cfg.GitHubRepo,
			PullRequestNumbers: cfg.PullRequestNumbers,
			JiraKeys:           cfg.JiraKeys,
		})
		if err != nil {
			return err
		}
	}
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()

	result, err := sourcegraph.LoadFixture(ctx, graphStore.Client(), records, sourcegraph.LoadOptions{
		StreamKey: cfg.StreamKey,
		RunKey:    cfg.RunKey,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// exportWorkProgramGraphContext writes the GraphQL-shaped bounded context used by the LLM harness.
func exportWorkProgramGraphContext(ctx context.Context, cfg workProgramGraphContextExportConfig, writer io.Writer) error {
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()

	resolver := (&ontologygraphql.Resolver{EntClient: graphStore.Client()}).Query()
	graphContext, err := resolver.WorkProgramGraphContext(
		ctx,
		strings.TrimSpace(cfg.WorkstreamKey),
		positiveIntPointer(cfg.ItemLimit),
		positiveIntPointer(cfg.ActionLimit),
		positiveIntPointer(cfg.EdgeLimit),
		positiveIntPointer(cfg.InsightLimit),
		positiveIntPointer(cfg.ForecastLimit),
		positiveIntPointer(cfg.EvidenceLimit),
		positiveIntPointer(cfg.TraversalDepth),
		trimmedStringPointer(cfg.RunKey),
		trimmedStringPointer(cfg.GeneratedAt),
		trimmedStringPointer(cfg.SourceInstance),
	)
	if err != nil {
		return err
	}

	output := writer
	if strings.TrimSpace(cfg.OutPath) != "" {
		outPath := strings.TrimSpace(cfg.OutPath)
		if dir := filepath.Dir(outPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		file, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}
	return writeJSON(output, map[string]any{
		"data": map[string]any{
			"workProgramGraphContext": graphContext,
		},
	})
}

// exportBoundedGraphContext writes a generic bounded graph context from a sample graphstore.
func exportBoundedGraphContext(ctx context.Context, cfg boundedGraphContextExportConfig, writer io.Writer) error {
	if strings.TrimSpace(cfg.DatabasePath) != "" {
		return exportEntBoundedGraphContext(ctx, cfg, writer)
	}
	store, seed, err := boundedGraphFixture(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.StartObjectType) != "" {
		seed.ObjectType = domain.ObjectType(strings.TrimSpace(cfg.StartObjectType))
	}
	if strings.TrimSpace(cfg.StartKey) != "" {
		seed.Key = strings.TrimSpace(cfg.StartKey)
	}
	envelope, err := graphcontext.BuildEnvelope(ctx, store, domain.ExpandRequest{
		Start:            seed,
		AssociationTypes: cfg.AssociationTypes,
		Depth:            cfg.Depth,
		LimitPerObject:   cfg.LimitPerObject,
	}, graphcontext.Options{
		Coverage: graphcontext.CoveragePolicy{
			CoverageState:                cfg.CoverageState,
			AbsenceClaimsAllowed:         cfg.AbsenceClaimsAllowed,
			AbsenceClaimGateReason:       cfg.AbsenceClaimGateReason,
			AbsenceClaimAssociationTypes: cfg.AbsenceClaimAssociationTypes,
			SourceSystem:                 cfg.CoverageSourceSystem,
			SourceInstance:               cfg.CoverageSourceInstance,
			CoverageWindowStart:          cfg.CoverageWindowStart,
			CoverageWindowEnd:            cfg.CoverageWindowEnd,
			Summary:                      cfg.CoverageSummary,
		},
		Guardrails:                    cfg.Guardrails,
		AssociationClaimMinConfidence: cfg.AssociationClaimMinConfidence,
	})
	if err != nil {
		return err
	}
	output := writer
	if strings.TrimSpace(cfg.OutPath) != "" {
		outPath := strings.TrimSpace(cfg.OutPath)
		if dir := filepath.Dir(outPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		file, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}
	return writeJSON(output, envelope)
}

func applyBoundedGraphExportCoverageOverrides(graphContext *graphqlmodel.BoundedGraphContext, cfg boundedGraphContextExportConfig) {
	if graphContext == nil {
		return
	}
	if graphContext.Coverage == nil {
		graphContext.Coverage = &graphqlmodel.BoundedGraphCoverage{}
	}
	coverage := graphContext.Coverage
	if value := strings.TrimSpace(cfg.CoverageSourceSystem); value != "" {
		coverage.SourceSystem = stringPointer(value)
	}
	if value := strings.TrimSpace(cfg.CoverageSourceInstance); value != "" {
		coverage.SourceInstance = stringPointer(value)
	}
	if value := strings.TrimSpace(cfg.CoverageWindowStart); value != "" {
		coverage.CoverageWindowStart = stringPointer(value)
	}
	if value := strings.TrimSpace(cfg.CoverageWindowEnd); value != "" {
		coverage.CoverageWindowEnd = stringPointer(value)
	}
	if cfg.AbsenceClaimAssociationTypes != nil {
		coverage.AbsenceClaimAssociationTypes = append([]string(nil), cfg.AbsenceClaimAssociationTypes...)
	}
}

// exportEntBoundedGraphContext writes a generic bounded context from typed Ent product rows.
func exportEntBoundedGraphContext(ctx context.Context, cfg boundedGraphContextExportConfig, writer io.Writer) error {
	databasePath := strings.TrimSpace(cfg.DatabasePath)
	if databasePath == "" {
		return errors.New("missing required --database for Ent-backed bounded graph export")
	}
	if cfg.ResetDatabase {
		if err := resetSQLiteDatabaseFiles(databasePath); err != nil {
			return err
		}
	}
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: databasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()

	if cfg.SeedFixture {
		if err := seedBoundedGraphEntFixture(ctx, graphStore.Client(), cfg.Fixture); err != nil {
			return err
		}
	}
	seed, err := boundedGraphEntFixtureSeed(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.StartObjectType) != "" {
		seed.ObjectType = domain.ObjectType(strings.TrimSpace(cfg.StartObjectType))
	}
	if strings.TrimSpace(cfg.StartKey) != "" {
		seed.Key = strings.TrimSpace(cfg.StartKey)
	}
	sourceAuthorityPolicy, err := boundedGraphEntSourceAuthorityPolicy(cfg)
	if err != nil {
		return err
	}
	ctx = boundedGraphExportContextWithPrincipalAccess(ctx, cfg)
	resolver := (&ontologygraphql.Resolver{
		EntClient:                   graphStore.Client(),
		GraphExpander:               boundedGraphEntFixtureExpander(graphStore.Client(), cfg.Fixture),
		BoundedGraphSourceAuthority: sourceAuthorityPolicy,
	}).Query()
	graphContext, err := resolver.BoundedGraphContext(
		ctx,
		string(seed.ObjectType),
		seed.Key,
		associationTypeStrings(cfg.AssociationTypes),
		intPointer(cfg.Depth),
		positiveIntPointer(cfg.LimitPerObject),
	)
	if err != nil {
		return err
	}
	applyBoundedGraphExportCoverageOverrides(graphContext, cfg)
	output := writer
	if strings.TrimSpace(cfg.OutPath) != "" {
		outPath := strings.TrimSpace(cfg.OutPath)
		if dir := filepath.Dir(outPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		file, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}
	return writeJSON(output, map[string]any{
		"boundedGraphContext": graphContext,
	})
}

func boundedGraphExportContextWithPrincipalAccess(ctx context.Context, cfg boundedGraphContextExportConfig) context.Context {
	if strings.TrimSpace(cfg.PrincipalKey) == "" && len(cfg.AllowedVisibilityClasses) == 0 && !cfg.PrincipalCoverageComplete {
		return ctx
	}
	return ontologygraphql.WithBoundedGraphPrincipalAccess(ctx, ontologygraphql.BoundedGraphPrincipalAccess{
		PrincipalKey:                 strings.TrimSpace(cfg.PrincipalKey),
		AllowedVisibilityClasses:     append([]string(nil), cfg.AllowedVisibilityClasses...),
		CoverageCompleteForPrincipal: cfg.PrincipalCoverageComplete,
	})
}

func boundedGraphFixture(cfg boundedGraphContextExportConfig) (graphstore.Expander, domain.ObjectRef, error) {
	switch strings.TrimSpace(cfg.Fixture) {
	case "", "generic-doc-message-ticket":
		store := sampledata.NewGenericDocumentMessageTicketMemoryStore()
		return store, domain.ObjectRef{ObjectType: ontology.ObjectDocument, Key: "doc:architecture-note"}, nil
	case "customer-incident-runbook":
		store := sampledata.NewCustomerIncidentRunbookMemoryStore()
		return store, domain.ObjectRef{ObjectType: domain.ObjectType("customer_account"), Key: "customer-account:acme"}, nil
	case "flink-autoscaler":
		store := sampledata.NewFakeFlinkAutoscalerMemoryStore()
		return store, domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"}, nil
	default:
		return nil, domain.ObjectRef{}, fmt.Errorf("unknown bounded graph fixture: %s", cfg.Fixture)
	}
}

func seedBoundedGraphEntFixture(ctx context.Context, client *genent.Client, fixture string) error {
	switch strings.TrimSpace(fixture) {
	case "company-ai-first-minimum":
		return sampledata.SeedCompanyAIFirstMinimum(ctx, client)
	case sampledata.OpenCustomerIncidentFixture:
		return sampledata.SeedOpenCustomerIncidentGraph(ctx, client)
	case openGraphFixtureName:
		return errors.New("open-graph fixture uses open-graph-fixture-load; --seed-fixture is not supported")
	default:
		return fmt.Errorf("unknown Ent-backed bounded graph fixture: %s", fixture)
	}
}

func boundedGraphEntFixtureExpander(client *genent.Client, fixture string) graphstore.Expander {
	switch strings.TrimSpace(fixture) {
	case sampledata.OpenCustomerIncidentFixture, openGraphFixtureName:
		return entgraph.NewOpenGraphExpander(client)
	default:
		return entgraph.NewProductExpander(client)
	}
}

func boundedGraphEntSourceAuthorityPolicy(cfg boundedGraphContextExportConfig) (graphcontext.SourceAuthorityPolicy, error) {
	if strings.TrimSpace(cfg.SourceAuthorityPath) != "" {
		return loadSourceAuthorityPolicy(cfg.SourceAuthorityPath)
	}
	return boundedGraphEntFixtureSourceAuthority(cfg.Fixture), nil
}

func loadSourceAuthorityPolicy(path string) (graphcontext.SourceAuthorityPolicy, error) {
	payload, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return graphcontext.SourceAuthorityPolicy{}, fmt.Errorf("read source authority policy %s: %w", path, err)
	}
	var policy graphcontext.SourceAuthorityPolicy
	if err := json.Unmarshal(payload, &policy); err != nil {
		return graphcontext.SourceAuthorityPolicy{}, fmt.Errorf("decode source authority policy %s: %w", path, err)
	}
	return policy, nil
}

func boundedGraphEntFixtureSourceAuthority(fixture string) graphcontext.SourceAuthorityPolicy {
	switch strings.TrimSpace(fixture) {
	case sampledata.OpenCustomerIncidentFixture:
		return sampledata.OpenCustomerIncidentSourceAuthorityPolicy()
	default:
		return graphcontext.DefaultSourceAuthorityPolicy()
	}
}

func boundedGraphEntFixtureSeed(cfg boundedGraphContextExportConfig) (domain.ObjectRef, error) {
	switch strings.TrimSpace(cfg.Fixture) {
	case "company-ai-first-minimum":
		return domain.ObjectRef{ObjectType: ontology.ObjectDocument, Key: "document:company-plan"}, nil
	case sampledata.OpenCustomerIncidentFixture:
		return sampledata.OpenCustomerIncidentSeed(), nil
	default:
		if strings.TrimSpace(cfg.StartObjectType) != "" && strings.TrimSpace(cfg.StartKey) != "" {
			return domain.ObjectRef{ObjectType: domain.ObjectType(strings.TrimSpace(cfg.StartObjectType)), Key: strings.TrimSpace(cfg.StartKey)}, nil
		}
		return domain.ObjectRef{}, fmt.Errorf("missing --start-object-type and --start-key for Ent-backed fixture %q", cfg.Fixture)
	}
}

func associationTypeStrings(values []domain.AssociationType) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(string(value)) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(string(value)))
	}
	return out
}

func resetSQLiteDatabaseFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reset SQLite database %s: %w", candidate, err)
		}
	}
	return nil
}

func existingSuccessfulRecordTypes(records []sourcecapture.Record) map[string]bool {
	existing := make(map[string]bool, len(records))
	for _, record := range records {
		if record.Response.StatusCode == http.StatusOK {
			existing[record.SourceObjectID+"\x00"+record.SourceObjectType] = true
		}
	}
	return existing
}

func mergeFixtureRecords(existing []sourcecapture.Record, fetched []sourcecapture.Record) []sourcecapture.Record {
	byKey := make(map[string]sourcecapture.Record, len(existing)+len(fetched))
	for _, record := range existing {
		byKey[record.SourceObjectID+"\x00"+record.SourceObjectType] = record
	}
	for _, record := range fetched {
		byKey[record.SourceObjectID+"\x00"+record.SourceObjectType] = record
	}
	merged := make([]sourcecapture.Record, 0, len(byKey))
	for _, record := range byKey {
		merged = append(merged, record)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].SnapshotKey < merged[j].SnapshotKey
	})
	return merged
}

func githubTokenFromEnv(primary string) string {
	for _, key := range []string{primary, "GH_TOKEN", "GITHUB_TOKEN"} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token
		}
	}
	return ""
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func parseIntCSV(value string) ([]int, error) {
	parts := parseStringCSV(value)
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("invalid positive integer %q", part)
		}
		numbers = append(numbers, number)
	}
	return numbers, nil
}

func parseStringCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func trimmedStringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// recordStatusKey is the source/status tuple used by fixture coverage buckets.
type recordStatusKey struct {
	source string
	status int
}

// countBuckets turns replay records into stable summary buckets.
func countBuckets(records []sourcecapture.Record, keyFunc func(recordStatusKey) string) []summaryBucket {
	counts := make(map[string]int)
	for _, record := range records {
		key := keyFunc(recordStatusKey{source: record.SourceKey, status: record.Response.StatusCode})
		if key == "" {
			continue
		}
		counts[key]++
	}
	buckets := make([]summaryBucket, 0, len(counts))
	for key, count := range counts {
		buckets = append(buckets, summaryBucket{Key: key, Count: count})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Key < buckets[j].Key
	})
	return buckets
}

// validateListenAddress keeps the local service private unless the caller opts out.
func validateListenAddress(listen string, allowPublicBind bool) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", listen, err)
	}
	if allowPublicBind {
		return nil
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing public bind %q without --allow-public-bind", listen)
	}
	return nil
}

// serve opens Ent, registers runtime invariants, and starts the HTTP API.
func serve(cfg serveConfig, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info(
		"ontology_service_config",
		"config_path", cfg.ConfigPath,
		"listen_addr", cfg.Listen,
		"database_path", cfg.DatabasePath,
		"sqlite_busy_timeout_ms", cfg.SQLiteBusyTimeout.Milliseconds(),
		"graphql_playground_enabled", cfg.GraphQLPlaygroundEnabled,
	)

	graphStore, err := entstore.Open(context.Background(), entstore.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()
	logger.Info("ontology_ent_ready")

	router := httpapi.NewRouterWithOptions(logger, httpapi.RouterOptions{
		GraphQLPlaygroundEnabled: cfg.GraphQLPlaygroundEnabled,
		EntClient:                graphStore.Client(),
	})
	server := newHTTPServer(cfg, router)

	logger.Info("ontology_service_listening", "url", "http://"+cfg.Listen)
	return server.ListenAndServe()
}

// newHTTPServer applies fixed HTTP timeouts around the ontology API.
func newHTTPServer(cfg serveConfig, router http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           router,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}
