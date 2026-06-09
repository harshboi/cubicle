package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

var ErrInvalidWorkstream = errors.New("invalid workstream query")

// WorkstreamOverview is the app-friendly view of a bounded workstream graph.
//
// The raw graph endpoint is still useful for debugging and future visual graph
// UIs. This DTO is narrower: it gives Swift predictable buckets for the common
// Cubicle screen without making the client reimplement ontology classification.
type WorkstreamOverview struct {
	Workstream       domain.Object        `json:"workstream"`
	Tickets          []domain.Object      `json:"tickets"`
	PullRequests     []domain.Object      `json:"pull_requests"`
	CodeFiles        []domain.Object      `json:"code_files"`
	Blockers         []domain.Object      `json:"blockers"`
	ActionCandidates []domain.Object      `json:"action_candidates"`
	Associations     []domain.Association `json:"associations"`
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
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: key},
		Depth:          3,
		LimitPerObject: 20,
	})
	if err != nil {
		return WorkstreamOverview{}, err
	}

	overview := WorkstreamOverview{Associations: graph.Associations}
	for _, node := range graph.Objects {
		switch node.ObjectType {
		case ontology.ObjectWorkstream:
			if node.Key == key {
				overview.Workstream = node
			}
		case ontology.ObjectTicket:
			overview.Tickets = append(overview.Tickets, node)
		case ontology.ObjectPullRequest:
			overview.PullRequests = append(overview.PullRequests, node)
		case ontology.ObjectCodeFile:
			overview.CodeFiles = append(overview.CodeFiles, node)
		case ontology.ObjectBlocker:
			overview.Blockers = append(overview.Blockers, node)
		case ontology.ObjectActionCandidate:
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
	if strings.HasPrefix(slug, string(ontology.ObjectWorkstream)+":") {
		return slug
	}
	return string(ontology.ObjectWorkstream) + ":" + slug
}

func fmtInvalidWorkstream(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidWorkstream, message)
}
