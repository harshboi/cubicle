package graphcontext

import (
	_ "embed"
	"encoding/json"
	"strings"

	"cubicle/services/ontology-service/internal/domain"
)

//go:embed source_authority.json
var defaultSourceAuthorityJSON []byte

// SourceAuthorityPolicy describes which source systems can prove the presence
// of each relationship family before the relationship can support product prose.
type SourceAuthorityPolicy struct {
	RelationshipAuthority map[string]RelationshipSourceAuthority `json:"relationship_authority"`
}

// RelationshipSourceAuthority describes which source systems, optional source
// instances, mapper/source versions, and source-specific locator kinds can
// prove relationship presence. Durable extractor identity is still a connector
// convention layered over mapper version rather than a first-class policy field.
type RelationshipSourceAuthority struct {
	PresenceSources         []string            `json:"presence_sources"`
	PresenceSourceInstances map[string][]string `json:"presence_source_instances"`
	PresenceMapperVersions  map[string][]string `json:"presence_mapper_versions"`
	PresenceLocatorKinds    map[string][]string `json:"presence_locator_kinds"`
	AbsenceSources          []string            `json:"absence_sources"`
}

// DefaultSourceAuthorityPolicy returns the canonical relationship source policy
// used by Ent-backed bounded graph runtime and eval promotion audits.
func DefaultSourceAuthorityPolicy() SourceAuthorityPolicy {
	var policy SourceAuthorityPolicy
	if err := json.Unmarshal(defaultSourceAuthorityJSON, &policy); err != nil {
		return SourceAuthorityPolicy{}
	}
	normalizeSourceAuthorityPolicy(&policy)
	return policy
}

func (p SourceAuthorityPolicy) associationClaimGateReason(value domain.Association) string {
	if len(p.RelationshipAuthority) == 0 {
		return ""
	}
	associationType := strings.TrimSpace(string(value.AssociationType))
	row, ok := p.RelationshipAuthority[associationType]
	if !ok {
		return "relationship_authority_policy_missing"
	}
	evidenceSource := sourceAuthorityKey(value.Metadata.EvidenceSource)
	if evidenceSource == "" {
		return "relationship_source_authority_missing_evidence_source"
	}
	if !sourceAllowed(evidenceSource, row.PresenceSources) {
		return "relationship_source_not_authoritative_for_presence"
	}
	if gateReason := row.presenceSourceInstanceGateReason(evidenceSource, value.Metadata.EvidenceSourceInstance); gateReason != "" {
		return gateReason
	}
	if gateReason := row.presenceMapperVersionGateReason(evidenceSource, value.Metadata.MapperVersion); gateReason != "" {
		return gateReason
	}
	return row.presenceLocatorGateReason(evidenceSource, value.Metadata.EvidenceLocatorKind)
}

func (row RelationshipSourceAuthority) presenceSourceInstanceGateReason(source string, sourceInstance string) string {
	if len(row.PresenceSourceInstances) == 0 {
		return ""
	}
	allowed, ok := row.PresenceSourceInstances[source]
	if !ok {
		allowed, ok = row.PresenceSourceInstances["*"]
	}
	if !ok {
		return "relationship_source_authority_missing_instance_policy"
	}
	sourceInstance = sourceAuthorityKey(sourceInstance)
	if sourceInstance == "" {
		return "relationship_source_authority_missing_evidence_source_instance"
	}
	if sourceAllowed(sourceInstance, allowed) {
		return ""
	}
	return "relationship_source_instance_not_authoritative_for_presence"
}

func (row RelationshipSourceAuthority) presenceMapperVersionGateReason(source string, mapperVersion string) string {
	if len(row.PresenceMapperVersions) == 0 {
		return ""
	}
	allowed, ok := row.PresenceMapperVersions[source]
	if !ok {
		allowed, ok = row.PresenceMapperVersions["*"]
	}
	if !ok {
		return "relationship_source_authority_missing_mapper_version_policy"
	}
	mapperVersion = sourceAuthorityKey(mapperVersion)
	if mapperVersion == "" {
		return "relationship_source_authority_missing_evidence_mapper_version"
	}
	if sourceAllowed(mapperVersion, allowed) {
		return ""
	}
	return "relationship_mapper_version_not_authoritative_for_presence"
}

func (row RelationshipSourceAuthority) presenceLocatorGateReason(source string, locatorKind string) string {
	if len(row.PresenceLocatorKinds) == 0 {
		return ""
	}
	allowed, ok := row.PresenceLocatorKinds[source]
	if !ok {
		allowed, ok = row.PresenceLocatorKinds["*"]
	}
	if !ok {
		return "relationship_source_authority_missing_locator_policy"
	}
	locatorKind = sourceAuthorityKey(locatorKind)
	if locatorKind == "" {
		return "relationship_source_authority_missing_evidence_locator_kind"
	}
	if sourceAllowed(locatorKind, allowed) {
		return ""
	}
	return "relationship_locator_not_authoritative_for_presence"
}

func sourceAllowed(source string, allowed []string) bool {
	for _, candidate := range allowed {
		candidate = sourceAuthorityKey(candidate)
		if candidate == "*" || candidate == "all" || candidate == source {
			return true
		}
	}
	return false
}

func sourceAuthorityKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSourceAuthorityPolicy(policy *SourceAuthorityPolicy) {
	if policy.RelationshipAuthority == nil {
		policy.RelationshipAuthority = map[string]RelationshipSourceAuthority{}
		return
	}
	normalized := make(map[string]RelationshipSourceAuthority, len(policy.RelationshipAuthority))
	for relationshipType, row := range policy.RelationshipAuthority {
		key := strings.TrimSpace(relationshipType)
		if key == "" {
			continue
		}
		row.PresenceSources = normalizeAuthoritySourceList(row.PresenceSources)
		row.PresenceSourceInstances = normalizeAuthoritySourceMap(row.PresenceSourceInstances)
		row.PresenceMapperVersions = normalizeAuthoritySourceMap(row.PresenceMapperVersions)
		row.PresenceLocatorKinds = normalizeAuthoritySourceMap(row.PresenceLocatorKinds)
		row.AbsenceSources = normalizeAuthoritySourceList(row.AbsenceSources)
		normalized[key] = row
	}
	policy.RelationshipAuthority = normalized
}

func normalizeAuthoritySourceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for key, list := range values {
		key = sourceAuthorityKey(key)
		if key == "" {
			continue
		}
		out[key] = normalizeAuthoritySourceList(list)
	}
	return out
}

func normalizeAuthoritySourceList(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = sourceAuthorityKey(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
