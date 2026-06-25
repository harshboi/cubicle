package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ticket is an issue-tracker work item such as a Jira ticket or GitHub issue.
//
// Association:
//
//	Workstream -> WorkstreamTicket -> Ticket
//	Ticket -> TicketPullRequest -> PullRequest
//	Ticket -> TicketDocument -> Document
//	Ticket -> TicketMessage -> Message
//
// Ticket acts as a work parent while evidence remains attached as proof.
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
		edge.To("documents", Document.Type).
			Through("ticket_documents", TicketDocument.Type).
			Comment("Documents that explain or support this ticket."),
		edge.To("messages", Message.Type).
			Through("ticket_messages", TicketMessage.Type).
			Comment("Messages that discuss this ticket."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this ticket state."),
		edge.From("insights", WorkInsight.Type).
			Ref("ticket").
			Comment("Generated TPM/product insights about this ticket."),
		edge.From("actions", WorkAction.Type).
			Ref("ticket").
			Comment("Gated TPM actions about this ticket."),
		edge.From("state_snapshots", WorkItemStateSnapshot.Type).
			Ref("ticket").
			Comment("Observed state snapshots for this ticket."),
		edge.From("state_transitions", WorkItemStateTransition.Type).
			Ref("ticket").
			Comment("Observed state transitions for this ticket."),
		edge.From("milestones", WorkProgramMilestone.Type).
			Ref("ticket").
			Comment("Source-backed milestone and date signals for this ticket."),
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
