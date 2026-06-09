package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkPane is a bounded, ranked view within a person's work surface.
//
// Each pane declares exactly one target kind. Ent implements that declaration
// through concrete target-specific edges: documents, pull requests, tickets, or
// messages. Query code must page over the link rows before loading targets.
type WorkPane struct {
	ent.Schema
}

// Annotations declares that WorkPane is part of the future public entgql API.
func (WorkPane) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines pane identity, target declaration, and cached rollups.
func (WorkPane) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_surface_id").
				Comment("Parent WorkSurface row that owns this pane."),
			field.Enum("pane_kind").
				Values(paneKindValues()...).
				Comment("Specific bounded view represented by this pane."),
			field.Enum("target_kind").
				Values(targetKindValues()...).
				Comment("Only target kind this pane is allowed to expose."),
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable pane label, such as Documents Commented On."),
			field.Text("description").
				Optional().
				Comment("Short explanation of the pane's semantic relationship."),
			field.Int("target_count").
				Default(0).
				Comment("Cached number of targets behind this pane."),
			field.Int("source_count").
				Default(0).
				Comment("Number of source systems that contributed to this pane."),
			field.Bool("is_complete").
				Default(false).
				Comment("Whether all configured sources for this pane have been crawled."),
			field.Time("last_indexed_at").
				Optional().
				Comment("Time this pane's rollups were last rebuilt or updated."),
		},
		activityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects a pane to its parent surface and concrete target association
// lists. Each target edge uses a typed Through schema so link metadata is stored
// on the relationship row.
func (WorkPane) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("surface", WorkSurface.Type).
			Ref("panes").
			Unique().
			Required().
			Field("work_surface_id").
			Comment("Parent surface that owns this pane."),
		edge.To("documents", Document.Type).
			Through("document_links", PaneDocumentLink.Type).
			Comment("Documents linked from this pane."),
		edge.To("pull_requests", PullRequest.Type).
			Through("pull_request_links", PanePullRequestLink.Type).
			Comment("Pull requests linked from this pane."),
		edge.To("tickets", Ticket.Type).
			Through("ticket_links", PaneTicketLink.Type).
			Comment("Tickets linked from this pane."),
		edge.To("messages", Message.Type).
			Through("message_links", PaneMessageLink.Type).
			Comment("Messages linked from this pane."),
	}
}

// Indexes prevents duplicate panes under a surface and supports fast pane
// discovery for person-centered graph traversal.
func (WorkPane) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_surface_id", "pane_kind").Unique(),
		index.Fields("work_surface_id", "target_kind", "rank_score", "last_activity_at"),
	}
}
