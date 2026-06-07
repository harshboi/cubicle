package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
)

var ErrInvalidWorkstream = errors.New("invalid workstream query")

// WorkstreamOverview is the app-friendly view of a bounded workstream graph.
//
// The raw graph endpoint is still useful for debugging and future visual graph
// UIs. This DTO is narrower: it gives Swift predictable buckets for the common
// Cubicle screen without making the client reimplement ontology classification.
type WorkstreamOverview struct {
	Workstream       domain.Node   `json:"workstream"`
	Tickets          []domain.Node `json:"tickets"`
	PullRequests     []domain.Node `json:"pull_requests"`
	CodeFiles        []domain.Node `json:"code_files"`
	Blockers         []domain.Node `json:"blockers"`
	ActionCandidates []domain.Node `json:"action_candidates"`
	Edges            []domain.Edge `json:"edges"`
}

type WorkstreamService struct {
	graph graphstore.Expander
}

func NewWorkstreamService(graph graphstore.Expander) *WorkstreamService {
	return &WorkstreamService{graph: graph}
}

func (s *WorkstreamService) Overview(ctx context.Context, slug string) (WorkstreamOverview, error) {
	key := normalizeWorkstreamKey(slug)
	if key == "" {
		return WorkstreamOverview{}, fmtInvalidWorkstream("slug is required")
	}

	graph, err := s.graph.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: key},
		Depth:        3,
		LimitPerNode: 20,
	})
	if err != nil {
		return WorkstreamOverview{}, err
	}

	overview := WorkstreamOverview{Edges: graph.Edges}
	for _, node := range graph.Nodes {
		switch node.Kind {
		case domain.KindWorkstream:
			if node.Key == key {
				overview.Workstream = node
			}
		case domain.KindTicket:
			overview.Tickets = append(overview.Tickets, node)
		case domain.KindPullRequest:
			overview.PullRequests = append(overview.PullRequests, node)
		case domain.KindCodeFile:
			overview.CodeFiles = append(overview.CodeFiles, node)
		case domain.KindBlocker:
			overview.Blockers = append(overview.Blockers, node)
		case domain.KindActionCandidate:
			overview.ActionCandidates = append(overview.ActionCandidates, node)
		}
	}
	if overview.Workstream.Key == "" {
		return WorkstreamOverview{}, fmtInvalidWorkstream("workstream not found in graph response")
	}
	return overview, nil
}

func normalizeWorkstreamKey(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	if strings.HasPrefix(slug, string(domain.KindWorkstream)+":") {
		return slug
	}
	return string(domain.KindWorkstream) + ":" + slug
}

func fmtInvalidWorkstream(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidWorkstream, message)
}
