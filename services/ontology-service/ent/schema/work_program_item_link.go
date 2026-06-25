package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramItemLink stores inspectable product/open-graph adjacency for an
// operating register item.
//
// WorkProgramItem keeps compact display fields for UI summaries, but those
// fields must not be the only representation of ticket, PR, document, or
// generic connector relationships. This link table lets graph traversal, proof
// checks, and future rankers follow relationships without parsing text blobs.
type WorkProgramItemLink struct {
	ent.Schema
}

// Annotations declares WorkProgramItemLink as a public operating relationship.
func (WorkProgramItemLink) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_program_item_links_one_typed_target": "((pull_request_id IS NULL OR ticket_id IS NULL) AND (open_graph_object_id IS NULL OR (pull_request_id IS NULL AND ticket_id IS NULL)))",
	}))
}

// Fields defines one outbound link from a program item to a typed or open
// graph target.
func (WorkProgramItemLink) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_program_item_id").
				Comment("WorkProgramItem that owns this structured link."),
			field.Int("pull_request_id").
				Optional().
				Comment("Resolved PullRequest target when the link points at a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Resolved Ticket target when the link points at a ticket."),
			field.Int("open_graph_object_id").
				Optional().
				Comment("Resolved OpenGraphObject target for non-PR/non-ticket links."),
			field.Int("evidence_attachment_id").
				Optional().
				Comment("Optional EvidenceAttachment proving this specific link claim."),
			field.Enum("link_kind").
				Values(workProgramItemLinkKindValues()...).
				Default(workProgramItemLinkRelated).
				Comment("Relationship semantics between the program item and target."),
			field.String("target_object_type").
				NotEmpty().
				Comment("Target object type, such as ticket, pull_request, document, message, or connector-specific types."),
			field.String("target_key").
				NotEmpty().
				Comment("Stable source-neutral target key."),
			field.Enum("target_resolution_state").
				Values(workDependencyEndpointResolutionValues()...).
				Default(workDependencyEndpointKeyOnly).
				Comment("Whether the target is resolved to a typed row, key-only, or known missing."),
			field.Text("link_summary").
				Optional().
				Comment("Short explanation of why this target is attached to the program item."),
			field.String("link_source").
				Optional().
				Comment("Source or derivation path that produced this link."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the structured link to its owner, optional target, and proof.
func (WorkProgramItemLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_program_item", WorkProgramItem.Type).
			Unique().
			Required().
			Field("work_program_item_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Program item that owns this link."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("PullRequest target when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket target when resolved."),
		edge.To("open_graph_object", OpenGraphObject.Type).
			Unique().
			Field("open_graph_object_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Open graph target when resolved."),
		edge.To("evidence_attachment", EvidenceAttachment.Type).
			Unique().
			Field("evidence_attachment_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Evidence attachment proving this link claim."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this link row."),
	}
}

// Indexes support bounded traversal from a register item, target lookup, and
// source-identity upserts.
func (WorkProgramItemLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_program_item_id", "link_kind", "rank_score", "last_activity_at"),
		index.Fields("target_object_type", "target_key", "link_kind"),
		index.Fields("pull_request_id", "link_kind"),
		index.Fields("ticket_id", "link_kind"),
		index.Fields("open_graph_object_id", "link_kind"),
		index.Fields("evidence_attachment_id"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
