# Go Gin Huma Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first Cubicle graph HTTP server using Gin as the Go server framework and Huma as the typed REST/OpenAPI contract layer.

**Architecture:** The HTTP surface is a thin adapter over the graph query layer. Gin owns server framework concerns: routing, route groups, middleware, recovery, request logging, and localhost binding. Huma owns typed request/response DTOs, validation, OpenAPI 3.1 generation, generated docs, and the contract that the Cubicle Swift app can consume through Swift OpenAPI Generator.

**Tech Stack:** Go 1.25.1, `github.com/gin-gonic/gin@v1.12.0`, `github.com/danielgtaylor/huma/v2@v2.38.0`, `github.com/danielgtaylor/huma/v2/adapters/humagin`, `log/slog`, `httptest`, existing `internal/domain`, existing in-memory graphstore scaffold.

---

## File Structure

```text
services/cubicle-graph/
 |
 +-- go.mod
 |     -> pin Gin and Huma versions
 |
 +-- cmd/cubicle-graph/main.go
 |     -> parse `serve` command flags, seed fixture graph, start localhost server
 |
 +-- internal/domain/graph.go
 |     -> existing graph node/edge DTOs used in API bodies
 |
 +-- internal/graphstore/store.go
 |     -> small interface used by HTTP/query layers
 |
 +-- internal/graphstore/memory_store.go
 |     -> existing POC implementation
 |
 +-- internal/fixtures/workstream.go
 |     -> deterministic in-memory graph used by tests and local server
 |
 +-- internal/httpapi/contracts.go
 |     -> Huma request/response DTOs and API error body
 |
 +-- internal/httpapi/router.go
 |     -> Gin router, Huma API registration, middleware
 |
 +-- internal/httpapi/health.go
 |     -> `GET /healthz`
 |
 +-- internal/httpapi/graph.go
 |     -> `POST /v1/graph/expand`
 |
 +-- internal/httpapi/router_test.go
 |     -> contract tests for JSON shape and OpenAPI
 |
 +-- internal/httpapi/graph_test.go
 |     -> graph expansion endpoint tests
```

## API Shape

```text
GET  /healthz
GET  /openapi.json
GET  /docs
POST /v1/graph/expand
```

The first server slice intentionally exposes one generic graph operation plus health/OpenAPI. Product endpoints such as readiness, trace, owner gaps, source health, and action candidates should be added after their query services exist.

## Task 1: Pin Server Dependencies

**Files:**

- Modify: `services/cubicle-graph/go.mod`

- [ ] **Step 1: Add framework dependencies**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go get github.com/gin-gonic/gin@v1.12.0
go get github.com/danielgtaylor/huma/v2@v2.38.0
go mod tidy
```

Expected `go.mod` contains:

```go
module cubicle/services/cubicle-graph

go 1.25.1

require (
	github.com/danielgtaylor/huma/v2 v2.38.0
	github.com/gin-gonic/gin v1.12.0
)
```

- [ ] **Step 2: Verify dependency graph**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go list -m github.com/gin-gonic/gin github.com/danielgtaylor/huma/v2
```

Expected output includes:

```text
github.com/gin-gonic/gin v1.12.0
github.com/danielgtaylor/huma/v2 v2.38.0
```

- [ ] **Step 3: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/go.mod services/cubicle-graph/go.sum
git commit -m "chore: add graph server dependencies"
```

## Task 2: Add Store Interface and Fixture Graph

**Files:**

- Create: `services/cubicle-graph/internal/graphstore/store.go`
- Create: `services/cubicle-graph/internal/fixtures/workstream.go`
- Create: `services/cubicle-graph/internal/fixtures/workstream_test.go`

- [ ] **Step 1: Write fixture test**

Create `services/cubicle-graph/internal/fixtures/workstream_test.go`:

```go
package fixtures

import (
	"context"
	"testing"

	"cubicle/services/cubicle-graph/internal/domain"
)

func TestFlinkAutoscalerStoreExpandsKnownWorkstream(t *testing.T) {
	ctx := context.Background()
	store := NewFlinkAutoscalerStore()

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        2,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand fixture graph: %v", err)
	}

	if len(graph.Nodes) < 4 {
		t.Fatalf("expected connected graph fixture, got %d nodes: %#v", len(graph.Nodes), graph.Nodes)
	}
	if len(graph.Edges) < 3 {
		t.Fatalf("expected evidence-backed fixture edges, got %d edges: %#v", len(graph.Edges), graph.Edges)
	}
	for _, edge := range graph.Edges {
		if edge.Metadata.EvidenceKey == "" {
			t.Fatalf("edge %s has empty evidence key", edge.Key)
		}
	}
}
```

- [ ] **Step 2: Run fixture test to verify it fails**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/fixtures -run TestFlinkAutoscalerStoreExpandsKnownWorkstream -v
```

Expected: FAIL because `NewFlinkAutoscalerStore` does not exist.

- [ ] **Step 3: Add graphstore interface**

Create `services/cubicle-graph/internal/graphstore/store.go`:

```go
package graphstore

import (
	"context"

	"cubicle/services/cubicle-graph/internal/domain"
)

type Expander interface {
	Expand(context.Context, domain.ExpandRequest) (domain.Neighborhood, error)
}
```

- [ ] **Step 4: Add deterministic fixture store**

Create `services/cubicle-graph/internal/fixtures/workstream.go`:

```go
package fixtures

import (
	"context"
	"time"

	"cubicle/services/cubicle-graph/internal/domain"
	"cubicle/services/cubicle-graph/internal/graphstore"
)

func NewFlinkAutoscalerStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Node{
		{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindBlocker, Key: "blocker:missing-review", Title: "Missing review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindActionCandidate, Key: "action:request-review", Title: "Request review", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, node := range nodes {
		if err := store.UpsertNode(ctx, node); err != nil {
			panic(err)
		}
	}

	edges := []domain.Edge{
		fixtureEdge("workstream:flink-autoscaler", domain.KindWorkstream, domain.PredicateContains, "ticket:FLINK-39743", domain.KindTicket, "evidence:jira-component", observedAt),
		fixtureEdge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateImplementedBy, "pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, "evidence:jira-remote-link", observedAt),
		fixtureEdge("pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, domain.PredicateChangesFile, "file:JobVertexScaler.java", domain.KindCodeFile, "evidence:github-files", observedAt),
		fixtureEdge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateBlockedBy, "blocker:missing-review", domain.KindBlocker, "evidence:review-gap", observedAt),
		fixtureEdge("blocker:missing-review", domain.KindBlocker, domain.PredicateNeedsAction, "action:request-review", domain.KindActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertEdge(ctx, edge); err != nil {
			panic(err)
		}
	}

	return store
}

func fixtureEdge(fromKey string, fromKind domain.Kind, predicate domain.Predicate, toKey string, toKind domain.Kind, evidenceKey string, observedAt time.Time) domain.Edge {
	return domain.Edge{
		From: domain.NodeRef{Kind: fromKind, Key: fromKey},
		To:   domain.NodeRef{Kind: toKind, Key: toKey},
		Metadata: domain.EdgeMetadata{
			Predicate:      predicate,
			EvidenceKey:    evidenceKey,
			Source:         "fixture",
			Confidence:     1,
			Visibility:     "public",
			FreshnessState: "fresh",
			ObservedAt:     observedAt,
		},
	}
}
```

- [ ] **Step 5: Run fixture test to verify it passes**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/fixtures -run TestFlinkAutoscalerStoreExpandsKnownWorkstream -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/internal/graphstore/store.go services/cubicle-graph/internal/fixtures
git commit -m "feat: add graph server fixture store"
```

## Task 3: Add Huma Contracts

**Files:**

- Create: `services/cubicle-graph/internal/httpapi/contracts.go`
- Create: `services/cubicle-graph/internal/httpapi/contracts_test.go`

- [ ] **Step 1: Write contract-shape tests**

Create `services/cubicle-graph/internal/httpapi/contracts_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"testing"
)

func TestHealthResponseJSONShape(t *testing.T) {
	body, err := json.Marshal(HealthResponse{OK: true})
	if err != nil {
		t.Fatalf("marshal health response: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected health response JSON: %s", body)
	}
}

func TestErrorResponseJSONShape(t *testing.T) {
	body, err := json.Marshal(ErrorResponse{Code: "invalid_request", Message: "depth must be non-negative"})
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}
	if string(body) != `{"code":"invalid_request","message":"depth must be non-negative"}` {
		t.Fatalf("unexpected error response JSON: %s", body)
	}
}
```

- [ ] **Step 2: Run contract tests to verify they fail**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'Test.*JSONShape' -v
```

Expected: FAIL because `internal/httpapi` and DTO types do not exist.

- [ ] **Step 3: Add contracts**

Create `services/cubicle-graph/internal/httpapi/contracts.go`:

```go
package httpapi

import "cubicle/services/cubicle-graph/internal/domain"

type HealthOutput struct {
	Body HealthResponse
}

type HealthResponse struct {
	OK bool `json:"ok"`
}

type ExpandInput struct {
	Body domain.ExpandRequest
}

type ExpandOutput struct {
	Body domain.Neighborhood
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
```

- [ ] **Step 4: Run contract tests to verify they pass**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'Test.*JSONShape' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/internal/httpapi/contracts.go services/cubicle-graph/internal/httpapi/contracts_test.go
git commit -m "feat: add graph API contracts"
```

## Task 4: Add Gin + Huma Router and Health Endpoint

**Files:**

- Create: `services/cubicle-graph/internal/httpapi/router.go`
- Create: `services/cubicle-graph/internal/httpapi/health.go`
- Create: `services/cubicle-graph/internal/httpapi/router_test.go`

- [ ] **Step 1: Write router tests**

Create `services/cubicle-graph/internal/httpapi/router_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cubicle/services/cubicle-graph/internal/fixtures"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("unexpected health body: %q", rec.Body.String())
	}
}

func TestOpenAPIDocumentIncludesHealth(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi response is not JSON: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi document has no paths: %#v", doc)
	}
	if _, ok := paths["/healthz"]; !ok {
		t.Fatalf("openapi document missing /healthz: %#v", paths)
	}
}
```

- [ ] **Step 2: Run router tests to verify they fail**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'TestHealthzReturnsOK|TestOpenAPIDocumentIncludesHealth' -v
```

Expected: FAIL because `NewRouter` does not exist.

- [ ] **Step 3: Add router**

Create `services/cubicle-graph/internal/httpapi/router.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"

	"cubicle/services/cubicle-graph/internal/graphstore"
)

func NewRouter(store graphstore.Expander, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	config := huma.DefaultConfig("Cubicle Graph API", "0.1.0")
	config.DocsPath = "/docs"
	config.OpenAPIPath = "/openapi"
	config.Servers = []*huma.Server{{URL: "http://127.0.0.1:48080"}}

	api := humagin.New(router, config)
	registerHealth(api)
	registerGraph(api, store)

	return router
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		logger.InfoContext(
			c.Request.Context(),
			"graph_http_request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
}
```

- [ ] **Step 4: Add health operation**

Create `services/cubicle-graph/internal/httpapi/health.go`:

```go
package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func registerHealth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Check graph service health",
		Tags:        []string{"system"},
	}, func(context.Context, *struct{}) (*HealthOutput, error) {
		return &HealthOutput{Body: HealthResponse{OK: true}}, nil
	})
}
```

- [ ] **Step 5: Add temporary graph registration stub**

Create `services/cubicle-graph/internal/httpapi/graph.go`:

```go
package httpapi

import (
	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/cubicle-graph/internal/graphstore"
)

func registerGraph(api huma.API, store graphstore.Expander) {
	_ = api
	_ = store
}
```

- [ ] **Step 6: Run router tests to verify they pass**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'TestHealthzReturnsOK|TestOpenAPIDocumentIncludesHealth' -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/internal/httpapi
git commit -m "feat: add gin huma graph router"
```

## Task 5: Add Graph Expansion Endpoint

**Files:**

- Modify: `services/cubicle-graph/internal/httpapi/graph.go`
- Create: `services/cubicle-graph/internal/httpapi/graph_test.go`

- [ ] **Step 1: Write graph endpoint tests**

Create `services/cubicle-graph/internal/httpapi/graph_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cubicle/services/cubicle-graph/internal/domain"
	"cubicle/services/cubicle-graph/internal/fixtures"
)

func TestGraphExpandReturnsNeighborhood(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	body := `{"start":{"kind":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_node":10}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/expand", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var graph domain.Neighborhood
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("expand response is not graph JSON: %v", err)
	}
	if len(graph.Nodes) < 4 {
		t.Fatalf("expected graph nodes, got %#v", graph.Nodes)
	}
	if len(graph.Edges) < 3 {
		t.Fatalf("expected graph edges, got %#v", graph.Edges)
	}
}

func TestGraphExpandRejectsInvalidBounds(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	body := `{"start":{"kind":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_node":0}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/expand", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run graph endpoint tests to verify they fail**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'TestGraphExpand' -v
```

Expected: FAIL because `/v1/graph/expand` is not registered.

- [ ] **Step 3: Implement graph endpoint**

Replace `services/cubicle-graph/internal/httpapi/graph.go` with:

```go
package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/cubicle-graph/internal/graphstore"
)

func registerGraph(api huma.API, store graphstore.Expander) {
	huma.Register(api, huma.Operation{
		OperationID: "expand-graph",
		Method:      http.MethodPost,
		Path:        "/v1/graph/expand",
		Summary:     "Expand a bounded graph neighborhood",
		Tags:        []string{"graph"},
	}, func(ctx context.Context, input *ExpandInput) (*ExpandOutput, error) {
		graph, err := store.Expand(ctx, input.Body)
		if err != nil {
			if errors.Is(err, graphstore.ErrInvalidExpansion) || errors.Is(err, graphstore.ErrMissingNode) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("graph expansion failed")
		}
		return &ExpandOutput{Body: graph}, nil
	})
}
```

- [ ] **Step 4: Run graph endpoint tests to verify they pass**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run 'TestGraphExpand' -v
```

Expected: PASS.

- [ ] **Step 5: Add exact OpenAPI path assertion for graph endpoint**

Replace `TestOpenAPIDocumentIncludesHealth` in `services/cubicle-graph/internal/httpapi/router_test.go` with:

```go
func TestOpenAPIDocumentIncludesHealthAndGraph(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi response is not JSON: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi document has no paths: %#v", doc)
	}
	for _, path := range []string{"/healthz", "/v1/graph/expand"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("openapi document missing %s: %#v", path, paths)
		}
	}
}
```

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run TestOpenAPIDocumentIncludesHealthAndGraph -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/internal/httpapi/graph.go services/cubicle-graph/internal/httpapi/graph_test.go services/cubicle-graph/internal/httpapi/router_test.go
git commit -m "feat: add graph expansion API"
```

## Task 6: Add Serve Command

**Files:**

- Create: `services/cubicle-graph/cmd/cubicle-graph/main.go`
- Create: `services/cubicle-graph/cmd/cubicle-graph/main_test.go`

- [ ] **Step 1: Write command parsing test**

Create `services/cubicle-graph/cmd/cubicle-graph/main_test.go`:

```go
package main

import "testing"

func TestParseServeConfigDefaultsToLocalhost(t *testing.T) {
	cfg, err := parseServeConfig([]string{"serve"})
	if err != nil {
		t.Fatalf("parse serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
}

func TestParseServeConfigRejectsPublicBindWithoutFlag(t *testing.T) {
	_, err := parseServeConfig([]string{"serve", "--listen", "0.0.0.0:48080"})
	if err == nil {
		t.Fatal("expected public bind without --allow-public-bind to fail")
	}
}
```

- [ ] **Step 2: Run command tests to verify they fail**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./cmd/cubicle-graph -run TestParseServeConfig -v
```

Expected: FAIL because `parseServeConfig` does not exist.

- [ ] **Step 3: Add serve command**

Create `services/cubicle-graph/cmd/cubicle-graph/main.go`:

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"cubicle/services/cubicle-graph/internal/fixtures"
	"cubicle/services/cubicle-graph/internal/httpapi"
)

type serveConfig struct {
	Listen          string
	AllowPublicBind bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("cubicle_graph_exit", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected command: serve")
	}
	switch args[0] {
	case "serve":
		cfg, err := parseServeConfig(args)
		if err != nil {
			return err
		}
		return serve(cfg)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func parseServeConfig(args []string) (serveConfig, error) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	cfg := serveConfig{Listen: "127.0.0.1:48080"}
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.BoolVar(&cfg.AllowPublicBind, "allow-public-bind", false, "allow binding outside localhost for development")
	if err := flags.Parse(args[1:]); err != nil {
		return serveConfig{}, err
	}
	if err := validateListenAddress(cfg.Listen, cfg.AllowPublicBind); err != nil {
		return serveConfig{}, err
	}
	return cfg, nil
}

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

func serve(cfg serveConfig) error {
	logger := slog.Default()
	router := httpapi.NewRouter(fixtures.NewFlinkAutoscalerStore(), logger)
	server := &http.Server{
		Addr:    cfg.Listen,
		Handler: router,
	}
	logger.Info("cubicle_graph_listening", "url", "http://"+cfg.Listen)
	return server.ListenAndServe()
}
```

- [ ] **Step 4: Run command tests to verify they pass**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./cmd/cubicle-graph -run TestParseServeConfig -v
```

Expected: PASS.

- [ ] **Step 5: Run local server smoke test**

Run in terminal 1:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go run ./cmd/cubicle-graph serve --listen 127.0.0.1:48080
```

Run in terminal 2:

```bash
curl -s http://127.0.0.1:48080/healthz
curl -s http://127.0.0.1:48080/openapi.json
curl -s -X POST http://127.0.0.1:48080/v1/graph/expand \
  -H 'Content-Type: application/json' \
  -d '{"start":{"kind":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_node":10}'
```

Expected first response:

```json
{"ok":true}
```

Expected OpenAPI response includes `"openapi"` and `"paths"`.

Expected graph response includes `ticket:FLINK-39743` and `pr:apache/flink-kubernetes-operator#1127`.

- [ ] **Step 6: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/cmd/cubicle-graph
git commit -m "feat: add graph serve command"
```

## Task 7: Add Swift Contract Artifact Check

**Files:**

- Create: `services/cubicle-graph/internal/httpapi/openapi_test.go`

- [ ] **Step 1: Write OpenAPI contract test**

Create `services/cubicle-graph/internal/httpapi/openapi_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"cubicle/services/cubicle-graph/internal/fixtures"
)

func TestOpenAPIDocumentIsSwiftClientReady(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi document is not JSON: %v", err)
	}
	if doc["openapi"] == "" {
		t.Fatalf("openapi version missing: %#v", doc)
	}
	paths := doc["paths"].(map[string]any)
	for _, path := range []string{"/healthz", "/v1/graph/expand"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("missing OpenAPI path %s in %#v", path, paths)
		}
	}
}
```

- [ ] **Step 2: Run OpenAPI test**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/httpapi -run TestOpenAPIDocumentIsSwiftClientReady -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph/internal/httpapi/openapi_test.go
git commit -m "test: lock graph API openapi contract"
```

## Task 8: Full Verification

**Files:**

- Modify only if verification exposes compile or contract issues in files from Tasks 1-7.

- [ ] **Step 1: Format Go code**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
gofmt -w cmd internal
```

Expected: no output.

- [ ] **Step 2: Run full Go test suite**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Check repo diff**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git diff --check
git status --short --branch
```

Expected: `git diff --check` prints no whitespace errors. Status shows either a clean branch or only intentional files from the current task before commit.

- [ ] **Step 4: Commit any verification fixes**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
git add services/cubicle-graph
git commit -m "chore: verify graph server scaffold"
```

Skip this commit only when Task 8 made no file changes.

## Acceptance Criteria

```text
go.mod pins Gin v1.12.0 and Huma v2.38.0
Gin router exists under internal/httpapi
Huma registers GET /healthz and POST /v1/graph/expand
/openapi.json exposes both operations
/docs is available from Huma
server binds to 127.0.0.1:48080 by default
public bind is rejected unless --allow-public-bind is passed
handlers return DTOs from internal/domain or internal/httpapi
handlers do not expose Ent structs, SQLite paths, FTS tables, or snapshot paths
graph expansion returns evidence-backed nodes and edges from the fixture graph
go test ./... passes under services/cubicle-graph
```

## Sources

- Gin package docs: https://pkg.go.dev/github.com/gin-gonic/gin
- Huma package docs: https://pkg.go.dev/github.com/danielgtaylor/huma/v2
- Huma Gin adapter package docs: https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humagin
- Swift OpenAPI Generator: https://github.com/apple/swift-openapi-generator
