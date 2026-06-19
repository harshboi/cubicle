package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Evidence is a citation/provenance row that explains why Cubicle believes a fact.
type Evidence struct {
	ent.Schema
}

// Annotations declares that Evidence is part of the future public entgql API.
func (Evidence) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines citation metadata shared by object and relationship claims.
func (Evidence) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("claim_kind").
				Values(claimKindValues()...).
				Default(claimKindObjectState).
				Comment("Kind of claim this proof supports: object state, relationship, identity, or candidate."),
			field.String("claim_target_kind").
				Optional().
				Comment("Product object/table kind this evidence supports, such as ticket, document, or ticket_pull_request."),
			field.Int("claim_target_id").
				Optional().
				Comment("Optional Ent row ID of the product object or typed relationship this evidence supports."),
			field.String("claim_field").
				Optional().
				Comment("Optional field or property this evidence supports, such as status, assignee, or summary."),
			field.String("relationship_kind").
				Optional().
				Comment("Optional semantic relationship name when the evidence supports a typed relationship row."),
			field.Int("relationship_id").
				Optional().
				Comment("Optional typed relationship row ID supported by this proof."),
			field.String("locator_kind").
				Optional().
				Comment("Source-local locator type, such as paragraph, comment, message, field, line, or span."),
			field.String("locator").
				Optional().
				Comment("Source-local locator value that can refetch or display the exact cited span."),
			field.String("source_span_key").
				Optional().
				Comment("Stable source-local span key used to dedupe proof rows across re-ingestion."),
			field.Int("ordinal").
				Optional().
				Comment("Order of this proof span inside its source object when the source provides one."),
			field.Int("span_start").
				Optional().
				Comment("Optional character, byte, or line start offset inside the source locator."),
			field.Int("span_end").
				Optional().
				Comment("Optional character, byte, or line end offset inside the source locator."),
			field.Text("excerpt").
				Optional().
				Comment("Short permission-checked source excerpt or citation text."),
			field.Bool("excerpt_truncated").
				Default(false).
				Comment("Whether excerpt was truncated before storage or display."),
			field.String("text_hash").
				Optional().
				Comment("Hash of normalized evidence text for idempotency checks."),
			field.String("lexical_fingerprint").
				Optional().
				Comment("Short normalized fingerprint for re-locating moved proof spans."),
			field.Enum("proof_state").
				Values(proofStateValues()...).
				Default(proofStateCurrent).
				Comment("Current citeability state of this proof locator."),
			field.Int("superseded_by_evidence_id").
				Optional().
				Comment("Optional newer Evidence row that replaces this proof."),
			field.String("acl_policy_key_snapshot").
				Optional().
				Comment("ACL policy key captured when this proof was observed."),
			field.String("visibility_hash_snapshot").
				Optional().
				Comment("Visibility hash captured when this proof was observed."),
			field.Time("observed_at").
				Optional().
				Comment("Time Cubicle observed this evidence."),
		},
		sourceBackedFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects superseded proof rows without turning proof spans into graph nodes.
func (Evidence) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("superseded_by_evidence", Evidence.Type).
			Unique().
			Field("superseded_by_evidence_id").
			Comment("Newer proof row that replaces this evidence."),
		edge.From("superseded_evidence", Evidence.Type).
			Ref("superseded_by_evidence").
			Comment("Older proof rows replaced by this evidence."),
	}
}

// Indexes supports proof lookup by supported claim and source locator.
func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("claim_kind", "claim_target_kind", "claim_target_id"),
		index.Fields("relationship_kind", "relationship_id"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id", "source_span_key"),
		index.Fields("text_hash"),
		index.Fields("acl_policy_key_snapshot", "visibility_hash_snapshot", "proof_state"),
	}
}
