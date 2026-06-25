package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvidenceAttachment binds one Evidence row to the exact object, field, or
// relationship claim it supports.
//
// latest_evidence_id remains a denormalized display cache on product rows.
// Product claimability should be decided from current EvidenceAttachment rows
// that match the target address and claim kind.
type EvidenceAttachment struct {
	ent.Schema
}

// Annotations declares EvidenceAttachment as an inspectable proof link.
func (EvidenceAttachment) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"evidence_attachments_relationship_claim_has_relationship": "claim_kind != 'relationship' OR relationship_kind IS NOT NULL OR relationship_id IS NOT NULL",
	}))
}

// Fields defines the generic target address plus optional typed row pointers.
func (EvidenceAttachment) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("evidence_id").
				Immutable().
				Comment("Evidence row attached to this exact claim."),
			field.Enum("claim_kind").
				Values(claimKindValues()...).
				Default(claimKindObjectState).
				Comment("Kind of claim this attachment supports."),
			field.String("target_kind").
				NotEmpty().
				Comment("Source-neutral target kind, such as ticket, pull_request, work_action, or ticket_pull_request."),
			field.String("target_key").
				NotEmpty().
				Comment("Stable target key; for typed rows this should match the row key when available."),
			field.String("target_table").
				Optional().
				Comment("Optional physical table name for audit/debugging; not used as the product API address."),
			field.Int("target_id").
				Optional().
				Comment("Optional Ent row ID for the attached target."),
			field.String("claim_field").
				Optional().
				Comment("Optional field/property this proof supports, such as status, assignee, or readiness_state."),
			field.String("relationship_kind").
				Optional().
				Comment("Relationship kind this proof supports when claim_kind is relationship."),
			field.Int("relationship_id").
				Optional().
				Comment("Optional typed relationship row ID supported by this proof."),
			field.Enum("attachment_state").
				Values(evidenceAttachmentStateValues()...).
				Default(evidenceAttachmentCurrent).
				Comment("Whether this proof attachment is current, candidate, superseded, or rejected."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket target pointer."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest target pointer."),
			field.Int("ticket_pull_request_id").
				Optional().
				Comment("Optional TicketPullRequest relationship pointer."),
			field.Int("open_graph_object_id").
				Optional().
				Comment("Optional OpenGraphObject target pointer."),
			field.Int("open_graph_association_id").
				Optional().
				Comment("Optional OpenGraphAssociation relationship pointer."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional WorkAction target pointer."),
			field.Int("work_insight_id").
				Optional().
				Comment("Optional WorkInsight target pointer."),
			field.Int("work_blocker_id").
				Optional().
				Comment("Optional WorkBlocker target pointer."),
			field.Int("work_dependency_edge_id").
				Optional().
				Comment("Optional WorkDependencyEdge target pointer."),
			field.Int("work_item_forecast_id").
				Optional().
				Comment("Optional WorkItemForecast target pointer."),
			field.Int("work_responsibility_id").
				Optional().
				Comment("Optional WorkResponsibility target pointer."),
		},
		sourceIdentityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects proof attachments to their evidence row and optional typed
// claim targets.
func (EvidenceAttachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("evidence", Evidence.Type).
			Unique().
			Required().
			Immutable().
			Field("evidence_id").
			Comment("Evidence row attached to the claim."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Ticket target when this attachment proves a ticket field."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("PullRequest target when this attachment proves a PR field."),
		edge.To("ticket_pull_request", TicketPullRequest.Type).
			Unique().
			Field("ticket_pull_request_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("TicketPullRequest target when this attachment proves an implementation relationship."),
		edge.To("open_graph_object", OpenGraphObject.Type).
			Unique().
			Field("open_graph_object_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("OpenGraphObject target when this attachment proves a generic object field."),
		edge.To("open_graph_association", OpenGraphAssociation.Type).
			Unique().
			Field("open_graph_association_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("OpenGraphAssociation target when this attachment proves a generic relationship."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkAction target when this attachment proves an action gate."),
		edge.To("work_insight", WorkInsight.Type).
			Unique().
			Field("work_insight_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkInsight target when this attachment proves a generated insight state."),
		edge.To("work_blocker", WorkBlocker.Type).
			Unique().
			Field("work_blocker_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkBlocker target when this attachment proves a blocker claim."),
		edge.To("work_dependency_edge", WorkDependencyEdge.Type).
			Unique().
			Field("work_dependency_edge_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkDependencyEdge target when this attachment proves an operating topology edge."),
		edge.To("work_item_forecast", WorkItemForecast.Type).
			Unique().
			Field("work_item_forecast_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkItemForecast target when this attachment proves a forecast/gate."),
		edge.To("work_responsibility", WorkResponsibility.Type).
			Unique().
			Field("work_responsibility_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("WorkResponsibility target when this attachment proves accountability."),
	}
}

// Indexes support claim policy lookups by target, evidence, and typed pointer.
func (EvidenceAttachment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_kind", "target_key", "claim_kind", "attachment_state"),
		index.Fields("target_kind", "target_key", "claim_field", "attachment_state"),
		index.Fields("relationship_kind", "relationship_id", "attachment_state"),
		index.Fields("evidence_id", "attachment_state"),
		index.Fields("ticket_id", "claim_kind", "attachment_state"),
		index.Fields("pull_request_id", "claim_kind", "attachment_state"),
		index.Fields("ticket_pull_request_id", "claim_kind", "attachment_state"),
		index.Fields("open_graph_object_id", "claim_kind", "attachment_state"),
		index.Fields("open_graph_association_id", "claim_kind", "attachment_state"),
		index.Fields("work_action_id", "claim_kind", "attachment_state"),
		index.Fields("work_insight_id", "claim_kind", "attachment_state"),
		index.Fields("work_blocker_id", "claim_kind", "attachment_state"),
		index.Fields("work_dependency_edge_id", "claim_kind", "attachment_state"),
		index.Fields("work_item_forecast_id", "claim_kind", "attachment_state"),
		index.Fields("work_responsibility_id", "claim_kind", "attachment_state"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
