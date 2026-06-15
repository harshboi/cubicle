package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ticket is an issue-tracker work item such as a Jira ticket or GitHub issue.
type Ticket struct {
	ent.Schema
}

// Annotations declares that Ticket is part of the future public entgql API.
func (Ticket) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the queryable execution fields for issue-tracker work.
func (Ticket) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("title").
				NotEmpty().
				Comment("Human-readable ticket title."),
			field.Text("body").
				Optional().
				Comment("Ticket description or source body text."),
			field.Enum("status").
				Values(ticketStatusUnknown, ticketStatusOpen, ticketStatusClosed).
				Default(ticketStatusUnknown).
				Comment("Normalized ticket lifecycle status."),
			field.String("priority").
				Optional().
				Comment("Source priority label when available."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges declares the typed work graph relationships available in this schema slice.
func (Ticket) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workstreams", Workstream.Type).
			Ref("tickets").
			Comment("Workstreams that include this ticket."),
		edge.To("pull_requests", PullRequest.Type).
			Through("ticket_pull_requests", TicketPullRequest.Type).
			Comment("Pull requests that implement this ticket."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this ticket state."),
	}
}

// Indexes supports Ent-filter search and status slicing before a dedicated
// search index exists.
func (Ticket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "last_activity_at"),
		index.Fields("priority"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
