// Package sourcespine contains small helpers for the Source Evidence Spine.
//
// The package deliberately avoids product query APIs. It only centralizes
// invariants that Ent cannot express directly: polymorphic provenance target
// validation and evidence-anchor permission/freshness filtering.
package sourcespine

import (
	"context"
	"errors"
	"fmt"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidenceanchor"
	"cubicle/services/ontology-service/ent/sourceobservation"
	"cubicle/services/ontology-service/ent/sourcerun"
)

var (
	ErrNilClient                = errors.New("nil ent client")                 // ErrNilClient means the caller did not provide a generated Ent client.
	ErrUnknownTargetKind        = errors.New("unknown target kind")            // ErrUnknownTargetKind means ExternalIdentity.target_kind is not a supported ontology kind.
	ErrMissingPermissionFilters = errors.New("missing permission filters")     // ErrMissingPermissionFilters means evidence preview text would be queryable without policy and visibility filters.
	ErrInvalidTargetID          = errors.New("invalid source spine target id") // ErrInvalidTargetID means ExternalIdentity.target_id cannot identify an Ent row.
)

// TargetKind names a canonical Cubicle object kind that ExternalIdentity can point at.
type TargetKind string

const (
	TargetKindPerson      TargetKind = "person"       // TargetKindPerson maps source identities to Person rows.
	TargetKindWorkstream  TargetKind = "workstream"   // TargetKindWorkstream maps source identities to Workstream rows.
	TargetKindTicket      TargetKind = "ticket"       // TargetKindTicket maps source identities to Ticket rows.
	TargetKindPullRequest TargetKind = "pull_request" // TargetKindPullRequest maps source identities to PullRequest rows.
	TargetKindDocument    TargetKind = "document"     // TargetKindDocument maps source identities to Document rows.
	TargetKindMessage     TargetKind = "message"      // TargetKindMessage maps source identities to Message rows.
	TargetKindEvidence    TargetKind = "evidence"     // TargetKindEvidence maps source identities to Evidence rows when a source has evidence IDs.
)

// AnchorVisibilityFilter contains the required permission and source-run gates for evidence-anchor reads.
type AnchorVisibilityFilter struct {
	PermissionPolicyKeys []string // PermissionPolicyKeys are source policy namespaces the current viewer is allowed to inspect.
	VisibilityHashes     []string // VisibilityHashes are source ACL fingerprints the current viewer is allowed to inspect.
	IncludePartialRuns   bool     // IncludePartialRuns allows anchors from partial SourceRuns but callers must surface a coverage warning.
}

// KnownTargetKindStrings returns target kinds accepted by ValidateTarget.
func KnownTargetKindStrings() []string {
	return []string{
		string(TargetKindPerson),
		string(TargetKindWorkstream),
		string(TargetKindTicket),
		string(TargetKindPullRequest),
		string(TargetKindDocument),
		string(TargetKindMessage),
		string(TargetKindEvidence),
	}
}

// ValidateTarget verifies that an ExternalIdentity target pointer resolves to a typed Ent object.
func ValidateTarget(ctx context.Context, client *genent.Client, targetKind string, targetID int) error {
	if client == nil {
		return ErrNilClient
	}
	if targetID <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidTargetID, targetID)
	}

	switch TargetKind(targetKind) {
	case TargetKindPerson:
		_, err := client.Person.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindWorkstream:
		_, err := client.Workstream.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindTicket:
		_, err := client.Ticket.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindPullRequest:
		_, err := client.PullRequest.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindDocument:
		_, err := client.Document.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindMessage:
		_, err := client.Message.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	case TargetKindEvidence:
		_, err := client.Evidence.Get(ctx, targetID)
		return wrapTargetLookup(targetKind, targetID, err)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownTargetKind, targetKind)
	}
}

// CurrentEvidenceAnchorQuery returns an Ent query constrained to current, permission-filtered anchors.
//
// The helper encodes the legal query shape for evidence preview reads. It does
// not execute the query and does not decide how to warn users about partial
// coverage; callers that set IncludePartialRuns must surface that warning.
func CurrentEvidenceAnchorQuery(client *genent.Client, filter AnchorVisibilityFilter) (*genent.EvidenceAnchorQuery, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if len(filter.PermissionPolicyKeys) == 0 || len(filter.VisibilityHashes) == 0 {
		return nil, ErrMissingPermissionFilters
	}

	statuses := []sourcerun.Status{sourcerun.StatusComplete}
	if filter.IncludePartialRuns {
		statuses = append(statuses, sourcerun.StatusPartial)
	}

	return client.EvidenceAnchor.Query().
		Where(
			evidenceanchor.HasSourceObservationWith(
				sourceobservation.IsDeleted(false),
				sourceobservation.PermissionPolicyKeyIn(filter.PermissionPolicyKeys...),
				sourceobservation.VisibilityHashIn(filter.VisibilityHashes...),
				sourceobservation.HasSourceRunWith(
					sourcerun.StatusIn(statuses...),
				),
			),
		), nil
}

// wrapTargetLookup adds target context to Ent lookup failures.
func wrapTargetLookup(targetKind string, targetID int, err error) error {
	if err != nil {
		return fmt.Errorf("load %s target %d: %w", targetKind, targetID, err)
	}
	return nil
}
