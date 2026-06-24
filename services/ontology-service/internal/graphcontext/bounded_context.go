package graphcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

const (
	defaultCoverageState            = "unknown"
	defaultAbsenceClaimGateReason   = "source_coverage_gate"
	defaultAssociationMinConfidence = 1.0
)

// CoveragePolicy describes what the bounded graph can and cannot prove.
type CoveragePolicy struct {
	CoverageState                string   `json:"coverageState"`
	AbsenceClaimsAllowed         bool     `json:"absenceClaimsAllowed"`
	AbsenceClaimGateReason       string   `json:"absenceClaimGateReason"`
	Summary                      string   `json:"summary,omitempty"`
	AbsenceClaimAssociationTypes []string `json:"absenceClaimAssociationTypes,omitempty"`
	SourceSystem                 string   `json:"sourceSystem,omitempty"`
	SourceInstance               string   `json:"sourceInstance,omitempty"`
	CoverageWindowStart          string   `json:"coverageWindowStart,omitempty"`
	CoverageWindowEnd            string   `json:"coverageWindowEnd,omitempty"`
}

// Options controls policy fields that are not owned by graph traversal itself.
type Options struct {
	ContextHash                   string
	Coverage                      CoveragePolicy
	Guardrails                    []string
	AssociationClaimMinConfidence float64
	SourceAuthorityPolicy         SourceAuthorityPolicy
}

// Envelope is the GraphQL-shaped JSON wrapper consumed by the Python brief harness.
type Envelope struct {
	BoundedGraphContext BoundedGraphContext `json:"boundedGraphContext"`
}

// BoundedGraphContext is a source-neutral LLM context over typed graph rows.
type BoundedGraphContext struct {
	ContextHash    string         `json:"contextHash"`
	Seed           Ref            `json:"seed"`
	Depth          int            `json:"depth"`
	LimitPerObject int            `json:"limitPerObject"`
	ScopeMode      string         `json:"scopeMode"`
	Coverage       CoveragePolicy `json:"coverage"`
	Guardrails     []string       `json:"guardrails,omitempty"`
	Objects        []Object       `json:"objects"`
	Associations   []Association  `json:"associations"`
	Evidence       []Evidence     `json:"evidence,omitempty"`
}

// Ref is the camel-case JSON shape used by bounded graph prompts.
type Ref struct {
	ObjectType string `json:"objectType"`
	Key        string `json:"key"`
}

// Object is the prompt-safe projection of a domain graph object.
type Object struct {
	ObjectType          string  `json:"objectType"`
	Key                 string  `json:"key"`
	Title               string  `json:"title"`
	Source              string  `json:"source,omitempty"`
	SourceInstance      string  `json:"sourceInstance,omitempty"`
	ExternalID          string  `json:"externalID,omitempty"`
	Visibility          string  `json:"visibility,omitempty"`
	FreshnessState      string  `json:"freshnessState,omitempty"`
	ProofState          string  `json:"proofState"`
	ClaimAllowed        bool    `json:"claimAllowed"`
	ClaimGateReason     string  `json:"claimGateReason"`
	SourceCoverageState string  `json:"sourceCoverageState,omitempty"`
	RankScore           float64 `json:"rankScore,omitempty"`
}

// Association is the prompt-safe projection of a typed graph relationship.
type Association struct {
	Key                    string  `json:"key"`
	AssociationType        string  `json:"associationType"`
	From                   Ref     `json:"from"`
	To                     Ref     `json:"to"`
	EvidenceKey            string  `json:"evidenceKey,omitempty"`
	Source                 string  `json:"source,omitempty"`
	SourceInstance         string  `json:"sourceInstance,omitempty"`
	MapperVersion          string  `json:"mapperVersion,omitempty"`
	EvidenceSource         string  `json:"evidenceSource,omitempty"`
	EvidenceSourceInstance string  `json:"evidenceSourceInstance,omitempty"`
	EvidenceLocatorKind    string  `json:"evidenceLocatorKind,omitempty"`
	Confidence             float64 `json:"confidence,omitempty"`
	Visibility             string  `json:"visibility,omitempty"`
	FreshnessState         string  `json:"freshnessState,omitempty"`
	ProofState             string  `json:"proofState"`
	ClaimAllowed           bool    `json:"claimAllowed"`
	ClaimGateReason        string  `json:"claimGateReason"`
}

// Evidence is a minimal evidence-row stub for prompt harnesses that want a
// row-shaped provenance list without raw source excerpts or URLs.
type Evidence struct {
	Key            string  `json:"key"`
	Source         string  `json:"source,omitempty"`
	SourceInstance string  `json:"sourceInstance,omitempty"`
	LocatorKind    string  `json:"locatorKind,omitempty"`
	Visibility     string  `json:"visibility,omitempty"`
	FreshnessState string  `json:"freshnessState,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
}

// Build expands a graph neighborhood and converts it into BoundedGraphContext.
func Build(ctx context.Context, expander graphstore.Expander, req domain.ExpandRequest, opts Options) (BoundedGraphContext, error) {
	if expander == nil {
		return BoundedGraphContext{}, fmt.Errorf("bounded graph context: expander is required")
	}
	req = requestWithReadFilter(req)
	neighborhood, err := expander.Expand(ctx, req)
	if err != nil {
		return BoundedGraphContext{}, err
	}
	neighborhood, startVisible := promptReadableNeighborhood(neighborhood, req.Start, req.ReadFilter)
	if !startVisible {
		return BoundedGraphContext{}, fmt.Errorf("bounded graph context: start object is not readable by the authorization filter")
	}

	policy := normalizeCoverage(opts.Coverage, req.AssociationTypes)
	out := BoundedGraphContext{
		Seed: Ref{
			ObjectType: string(req.Start.ObjectType),
			Key:        req.Start.Key,
		},
		Depth:          req.Depth,
		LimitPerObject: req.LimitPerObject,
		ScopeMode:      "bounded_graph_context",
		Coverage:       policy,
		Guardrails:     normalizeGuardrails(opts.Guardrails, policy),
		Objects:        projectObjects(neighborhood.Objects, policy),
		Associations: projectAssociations(
			neighborhood.Associations,
			neighborhood.Objects,
			normalizeMinConfidence(opts.AssociationClaimMinConfidence),
			opts.SourceAuthorityPolicy,
		),
	}
	out.Evidence = evidenceFromAssociations(out.Associations)
	if strings.TrimSpace(opts.ContextHash) != "" {
		out.ContextHash = strings.TrimSpace(opts.ContextHash)
	} else {
		out.ContextHash = stableContextHash(out)
	}
	return out, nil
}

// BuildEnvelope returns the GraphQL-shaped wrapper expected by the harness.
func BuildEnvelope(ctx context.Context, expander graphstore.Expander, req domain.ExpandRequest, opts Options) (Envelope, error) {
	graphContext, err := Build(ctx, expander, req, opts)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{BoundedGraphContext: graphContext}, nil
}

func requestWithReadFilter(req domain.ExpandRequest) domain.ExpandRequest {
	if readFilterActive(req.ReadFilter) {
		return req
	}
	req.ReadFilter = publicPromptReadFilter()
	return req
}

func readFilterActive(filter domain.ExpandReadFilter) bool {
	return filter.ObjectAllowed != nil || filter.AssociationAllowed != nil
}

func publicPromptReadFilter() domain.ExpandReadFilter {
	return domain.ExpandReadFilter{
		PrincipalKey: "public_prompt",
		ObjectAllowed: func(object domain.Object) bool {
			return promptVisibleValue(object.Visibility)
		},
		AssociationAllowed: func(association domain.Association) bool {
			return promptVisibleValue(association.Metadata.Visibility)
		},
	}
}

func promptReadableNeighborhood(neighborhood domain.Neighborhood, start domain.ObjectRef, filter domain.ExpandReadFilter) (domain.Neighborhood, bool) {
	objectByKey := make(map[string]domain.Object)
	for _, object := range neighborhood.Objects {
		objectByKey[objectRefKey(object.Ref())] = object
	}
	startObject, startFound := objectByKey[objectRefKey(start)]
	if !startFound || !readFilterObjectAllowed(filter, startObject) {
		return domain.Neighborhood{}, false
	}

	visibleObjects := map[string]bool{objectRefKey(start): true}
	objectOrder := []string{objectRefKey(start)}
	associationOrder := make([]domain.Association, 0, len(neighborhood.Associations))
	seenAssociations := make(map[string]bool)
	progress := true
	for progress {
		progress = false
		for _, association := range neighborhood.Associations {
			associationKey := associationKey(association)
			if seenAssociations[associationKey] || !readFilterAssociationAllowed(filter, association) {
				continue
			}
			fromKey := objectRefKey(association.From)
			toKey := objectRefKey(association.To)
			fromObject, fromFound := objectByKey[fromKey]
			toObject, toFound := objectByKey[toKey]
			if !fromFound || !toFound || !readFilterObjectAllowed(filter, fromObject) || !readFilterObjectAllowed(filter, toObject) {
				continue
			}
			if !visibleObjects[fromKey] && !visibleObjects[toKey] {
				continue
			}
			seenAssociations[associationKey] = true
			associationOrder = append(associationOrder, association)
			for _, key := range []string{fromKey, toKey} {
				if !visibleObjects[key] {
					visibleObjects[key] = true
					objectOrder = append(objectOrder, key)
					progress = true
				}
			}
		}
	}

	objects := make([]domain.Object, 0, len(objectOrder))
	for _, key := range objectOrder {
		objects = append(objects, objectByKey[key])
	}
	return domain.Neighborhood{Objects: objects, Associations: associationOrder}, true
}

func readFilterObjectAllowed(filter domain.ExpandReadFilter, object domain.Object) bool {
	if filter.ObjectAllowed == nil {
		return true
	}
	return filter.ObjectAllowed(object)
}

func readFilterAssociationAllowed(filter domain.ExpandReadFilter, association domain.Association) bool {
	if filter.AssociationAllowed == nil {
		return true
	}
	return filter.AssociationAllowed(association)
}

func promptVisibleValue(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == domain.VisibilityPublic
}

func objectRefKey(ref domain.ObjectRef) string {
	return string(ref.ObjectType) + "\x00" + ref.Key
}

func normalizeCoverage(policy CoveragePolicy, requestedAssociationTypes []domain.AssociationType) CoveragePolicy {
	policy.CoverageState = strings.TrimSpace(policy.CoverageState)
	if policy.CoverageState == "" {
		policy.CoverageState = defaultCoverageState
	}
	policy.AbsenceClaimGateReason = strings.TrimSpace(policy.AbsenceClaimGateReason)
	if policy.AbsenceClaimGateReason == "" {
		policy.AbsenceClaimGateReason = defaultAbsenceClaimGateReason
	}
	policy.Summary = strings.TrimSpace(policy.Summary)
	policy.SourceSystem = strings.TrimSpace(policy.SourceSystem)
	policy.SourceInstance = strings.TrimSpace(policy.SourceInstance)
	policy.CoverageWindowStart = strings.TrimSpace(policy.CoverageWindowStart)
	policy.CoverageWindowEnd = strings.TrimSpace(policy.CoverageWindowEnd)
	policy.AbsenceClaimAssociationTypes = normalizeCoverageAssociationTypes(policy.AbsenceClaimAssociationTypes)
	if policy.AbsenceClaimsAllowed && policy.CoverageState != "complete" {
		policy.AbsenceClaimsAllowed = false
		policy.AbsenceClaimGateReason = "source_coverage_not_complete"
		policy.Summary = appendCoverageSummary(policy.Summary, "Absence claims remain disabled because source coverage is not complete.")
	}
	if policy.AbsenceClaimsAllowed && !coverageCoversRequestedAssociations(policy.AbsenceClaimAssociationTypes, requestedAssociationTypes) {
		policy.AbsenceClaimsAllowed = false
		policy.AbsenceClaimGateReason = "relation_path_coverage_required"
		policy.Summary = appendCoverageSummary(policy.Summary, "Absence claims remain disabled because source coverage is not scoped to the requested relationship path.")
	}
	if policy.AbsenceClaimsAllowed && (policy.SourceSystem == "" || policy.SourceInstance == "") {
		policy.AbsenceClaimsAllowed = false
		policy.AbsenceClaimGateReason = "source_scope_coverage_required"
		policy.Summary = appendCoverageSummary(policy.Summary, "Absence claims remain disabled because source coverage is not scoped to a source system and instance.")
	}
	if policy.AbsenceClaimsAllowed && (policy.CoverageWindowStart == "" || policy.CoverageWindowEnd == "") {
		policy.AbsenceClaimsAllowed = false
		policy.AbsenceClaimGateReason = "source_time_window_required"
		policy.Summary = appendCoverageSummary(policy.Summary, "Absence claims remain disabled because source coverage is not scoped to a freshness window.")
	}
	return policy
}

func normalizeCoverageAssociationTypes(values []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func coverageCoversRequestedAssociations(covered []string, requested []domain.AssociationType) bool {
	coveredSet := make(map[string]bool, len(covered))
	for _, value := range covered {
		coveredSet[value] = true
	}
	if coveredSet["*"] || coveredSet["all"] {
		return true
	}
	if len(requested) == 0 {
		return false
	}
	for _, associationType := range requested {
		if !coveredSet[string(associationType)] {
			return false
		}
	}
	return true
}

func appendCoverageSummary(summary string, caveat string) string {
	caveat = strings.TrimSpace(caveat)
	if caveat == "" || strings.Contains(summary, caveat) {
		return summary
	}
	if summary == "" {
		return caveat
	}
	return summary + " " + caveat
}

func normalizeMinConfidence(value float64) float64 {
	if value <= 0 {
		return defaultAssociationMinConfidence
	}
	return value
}

func normalizeGuardrails(values []string, coverage CoveragePolicy) []string {
	guardrails := make(map[string]bool)
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			guardrails[text] = true
		}
	}
	if !coverage.AbsenceClaimsAllowed {
		guardrails["Source coverage gates absence claims; missing neighbors are unknown, not absent."] = true
	}
	out := make([]string, 0, len(guardrails))
	for text := range guardrails {
		out = append(out, text)
	}
	sort.Strings(out)
	return out
}

func projectObjects(values []domain.Object, coverage CoveragePolicy) []Object {
	out := make([]Object, 0, len(values))
	for _, value := range values {
		if value.ObjectType == "" || value.Key == "" {
			continue
		}
		title := strings.TrimSpace(value.Title)
		if title == "" {
			title = value.Key
		}
		claimAllowed, gateReason := objectClaimPolicy(value)
		out = append(out, Object{
			ObjectType:          string(value.ObjectType),
			Key:                 value.Key,
			Title:               title,
			Source:              value.Source,
			SourceInstance:      value.SourceInstance,
			ExternalID:          value.ExternalID,
			Visibility:          value.Visibility,
			FreshnessState:      value.FreshnessState,
			ProofState:          objectProofState(value),
			ClaimAllowed:        claimAllowed,
			ClaimGateReason:     gateReason,
			SourceCoverageState: coverage.CoverageState,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObjectType == out[j].ObjectType {
			return out[i].Key < out[j].Key
		}
		return out[i].ObjectType < out[j].ObjectType
	})
	return out
}

func objectClaimPolicy(value domain.Object) (bool, string) {
	switch strings.TrimSpace(value.FreshnessState) {
	case "":
		return false, "object_freshness_missing"
	case "unknown":
		return false, "object_freshness_unknown"
	case "partial":
		return false, "object_partial_requires_hydration"
	case "stale", "superseded", "tombstoned":
		return false, "object_not_current"
	}
	if visibility := strings.TrimSpace(value.Visibility); visibility == "" {
		return false, "object_visibility_missing"
	} else if visibility != domain.VisibilityPublic {
		return false, "object_visibility_restricted"
	}
	switch strings.TrimSpace(value.Source) {
	case "cubicle_ai", "generated", "llm":
		return false, "object_generated_requires_source_evidence"
	}
	if !builtinObjectType(value.ObjectType) {
		return false, "open_graph_object_context_only"
	}
	return true, "typed_graph_object"
}

func builtinObjectType(value domain.ObjectType) bool {
	return ontology.BuiltinRegistry().HasObjectType(value)
}

func objectProofState(value domain.Object) string {
	if value.Source != "" || value.SourceInstance != "" || value.ExternalID != "" || value.SnapshotKey != "" {
		return "source_observed"
	}
	return "typed_graph_row"
}

func projectAssociations(values []domain.Association, objects []domain.Object, minConfidence float64, sourceAuthority SourceAuthorityPolicy) []Association {
	out := make([]Association, 0, len(values))
	objectsByRef := domainObjectsByRef(objects)
	conflicted := conflictingLogicalAssociationKeys(values, objectsByRef, minConfidence)
	for _, value := range values {
		if value.From.Key == "" || value.To.Key == "" || value.AssociationType == "" {
			continue
		}
		claimAllowed, gateReason := associationClaimPolicy(value, objectsByRef, minConfidence, sourceAuthority)
		if conflicted[logicalAssociationKey(value)] {
			claimAllowed = false
			gateReason = "relationship_multi_evidence_requires_review"
		}
		out = append(out, Association{
			Key:                    associationKey(value),
			AssociationType:        string(value.AssociationType),
			From:                   ref(value.From),
			To:                     ref(value.To),
			EvidenceKey:            value.Metadata.EvidenceKey,
			Source:                 value.Metadata.Source,
			SourceInstance:         value.Metadata.SourceInstance,
			MapperVersion:          value.Metadata.MapperVersion,
			EvidenceSource:         value.Metadata.EvidenceSource,
			EvidenceSourceInstance: value.Metadata.EvidenceSourceInstance,
			EvidenceLocatorKind:    value.Metadata.EvidenceLocatorKind,
			Confidence:             value.Metadata.Confidence,
			Visibility:             value.Metadata.Visibility,
			FreshnessState:         value.Metadata.FreshnessState,
			ProofState:             associationProofState(claimAllowed, value),
			ClaimAllowed:           claimAllowed,
			ClaimGateReason:        gateReason,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].AssociationType < out[j].AssociationType
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func domainObjectsByRef(values []domain.Object) map[string]domain.Object {
	out := make(map[string]domain.Object, len(values))
	for _, value := range values {
		if value.ObjectType == "" || value.Key == "" {
			continue
		}
		out[objectRefKey(value.Ref())] = value
	}
	return out
}

func conflictingLogicalAssociationKeys(values []domain.Association, objects map[string]domain.Object, minConfidence float64) map[string]bool {
	grouped := make(map[string][]domain.Association)
	for _, value := range values {
		if value.From.Key == "" || value.To.Key == "" || value.AssociationType == "" {
			continue
		}
		key := logicalAssociationKey(value)
		grouped[key] = append(grouped[key], value)
	}
	conflicted := make(map[string]bool)
	for key, group := range grouped {
		if len(group) <= 1 {
			continue
		}
		conflicted[key] = true
	}
	return conflicted
}

func logicalAssociationKey(value domain.Association) string {
	return string(value.From.ObjectType) + ":" + value.From.Key +
		"|" + string(value.AssociationType) +
		"|" + string(value.To.ObjectType) + ":" + value.To.Key
}

func associationClaimPolicy(value domain.Association, objects map[string]domain.Object, minConfidence float64, sourceAuthority SourceAuthorityPolicy) (bool, string) {
	if gateReason := associationEndpointClaimGateReason(value, objects); gateReason != "" {
		return false, gateReason
	}
	if strings.TrimSpace(value.Metadata.EvidenceKey) == "" {
		return false, "missing_relationship_evidence"
	}
	if value.Metadata.EvidenceClaimKind == "" {
		return false, "relationship_evidence_claim_kind_missing"
	}
	if value.Metadata.EvidenceClaimKind != "relationship" {
		return false, "relationship_evidence_claim_kind_mismatch"
	}
	if value.Metadata.EvidenceRelationshipKind == "" {
		return false, "relationship_evidence_kind_missing"
	}
	if value.Metadata.EvidenceRelationshipKind != string(value.AssociationType) {
		return false, "relationship_evidence_kind_mismatch"
	}
	if value.Metadata.EvidenceProofState == "" {
		return false, "relationship_evidence_proof_state_missing"
	}
	if value.Metadata.EvidenceProofState != "current" {
		return false, "relationship_evidence_not_current"
	}
	switch strings.TrimSpace(value.Metadata.FreshnessState) {
	case "":
		return false, "relationship_freshness_missing"
	case "unknown":
		return false, "relationship_freshness_unknown"
	case "partial":
		return false, "relationship_partial_requires_hydration"
	case "stale", "superseded", "tombstoned":
		return false, "relationship_not_current"
	}
	if value.Metadata.Visibility == "" {
		return false, "relationship_visibility_missing"
	}
	if value.Metadata.Visibility != domain.VisibilityPublic {
		return false, "relationship_visibility_restricted"
	}
	switch strings.TrimSpace(value.Metadata.Source) {
	case "cubicle_ai", "generated", "llm":
		return false, "relationship_generated_requires_source_evidence"
	}
	hasSourceAuthority := len(sourceAuthority.RelationshipAuthority) > 0
	if gateReason := sourceAuthority.associationClaimGateReason(value); gateReason != "" {
		return false, gateReason
	}
	if value.Metadata.EvidenceCount > 1 && !hasSourceAuthority {
		return false, "relationship_multi_evidence_requires_review"
	}
	if value.Metadata.Confidence < minConfidence {
		return false, "candidate_link_requires_human_review"
	}
	return true, "source_evidence_full_confidence"
}

func associationEndpointClaimGateReason(value domain.Association, objects map[string]domain.Object) string {
	for _, ref := range []domain.ObjectRef{value.From, value.To} {
		object, ok := objects[objectRefKey(ref)]
		if !ok {
			return "relationship_endpoint_missing_requires_hydration"
		}
		switch strings.TrimSpace(object.FreshnessState) {
		case "":
			return "relationship_endpoint_freshness_missing"
		case "unknown":
			return "relationship_endpoint_freshness_unknown"
		case "partial":
			return "relationship_endpoint_partial_requires_hydration"
		case "stale", "superseded", "tombstoned":
			return "relationship_endpoint_not_current"
		}
	}
	return ""
}

func associationProofState(claimAllowed bool, value domain.Association) string {
	if claimAllowed {
		return "source_observed"
	}
	if strings.TrimSpace(value.Metadata.EvidenceKey) == "" {
		return "evidence_needed"
	}
	return "candidate"
}

func evidenceFromAssociations(associations []Association) []Evidence {
	seen := make(map[string]Evidence)
	for _, association := range associations {
		if strings.TrimSpace(association.EvidenceKey) == "" {
			continue
		}
		seen[association.EvidenceKey] = Evidence{
			Key:            association.EvidenceKey,
			Source:         association.EvidenceSource,
			SourceInstance: association.EvidenceSourceInstance,
			LocatorKind:    association.EvidenceLocatorKind,
			Visibility:     association.Visibility,
			FreshnessState: association.FreshnessState,
			Confidence:     association.Confidence,
		}
	}
	out := make([]Evidence, 0, len(seen))
	for _, evidence := range seen {
		out = append(out, evidence)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func ref(value domain.ObjectRef) Ref {
	return Ref{ObjectType: string(value.ObjectType), Key: value.Key}
}

func associationKey(value domain.Association) string {
	if strings.TrimSpace(value.Key) != "" {
		return strings.TrimSpace(value.Key)
	}
	return string(value.From.ObjectType) + ":" + value.From.Key +
		"|" + string(value.AssociationType) +
		"|" + string(value.To.ObjectType) + ":" + value.To.Key
}

func stableContextHash(value BoundedGraphContext) string {
	value.ContextHash = ""
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])[:16]
}
