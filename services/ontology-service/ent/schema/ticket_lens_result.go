package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketLensResult is the ranked association from a work lens window to a ticket.
//
// Association:
//
//	WorkArea -> WorkLens -> WorkLensWindow -> TicketLensResult -> Ticket
//
// The window parent keeps ticket results pageable without a direct WorkLens to
// Ticket fanout.
type TicketLensResult struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketLensResult) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("work_lens_id", "ticket_id", "relation_kind")
}

// Fields stores endpoint foreign keys plus ranked ticket relationship metadata.
func (TicketLensResult) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("work_lens_id").
				Immutable().
				Comment("Source WorkLens endpoint for this result."),
			field.Int("work_lens_window_id").
				Immutable().
				Comment("Bounded WorkLensWindow this result is assigned to for paging and materialization."),
			field.Int("ticket_id").
				Immutable().
				Comment("Target Ticket endpoint for this result."),
		},
		linkFields("relation_kind", ticketRelationValues()),
	)
}

// Edges connects the result row to its lens, ticket, and latest evidence.
func (TicketLensResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lens", WorkLens.Type).
			Unique().
			Required().
			Immutable().
			Field("work_lens_id").
			Comment("Work lens that owns this ticket result."),
		edge.From("window", WorkLensWindow.Type).
			Ref("ticket_results").
			Unique().
			Required().
			Immutable().
			Field("work_lens_window_id").
			Comment("Bounded lens window used to page this ticket result."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket target for this result."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this result."),
	}
}

// Indexes supports fast paged ticket reads from a lens.
func (TicketLensResult) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("work_lens_id", "ticket_id", "relation_kind"),
		index.Fields("work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_window_id", "relation_kind", "last_activity_at"),
		index.Fields("work_lens_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_id", "relation_kind", "last_activity_at"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
