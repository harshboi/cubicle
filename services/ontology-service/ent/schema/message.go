package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Message is a source communication item such as a Slack, chat, email, or thread message.
type Message struct {
	ent.Schema
}

// Annotations declares that Message is part of the future public entgql API.
func (Message) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines message text, thread context, and optional author key.
func (Message) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Text("body").
				Optional().
				Comment("Message body text."),
			field.String("channel_key").
				Optional().
				Comment("Source channel, room, or conversation key."),
			field.String("thread_key").
				Optional().
				Comment("Source thread key when the message belongs to a thread."),
			field.String("author_person_key").
				Optional().
				Comment("Best-known Cubicle person key for the source author."),
			field.Time("sent_at").
				Optional().
				Comment("Source timestamp for when the message was sent."),
		},
		textFields(),
		sourceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges exposes ticket discussion links and bounded person communication panes.
func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tickets", Ticket.Type).
			Ref("messages").
			Comment("Tickets discussed by this message."),
		edge.From("work_panes", WorkPane.Type).
			Ref("messages").
			Comment("Person work panes that include this message."),
	}
}

// Indexes supports thread/channel slicing and source author lookups.
func (Message) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_key", "thread_key"),
		index.Fields("author_person_key"),
		index.Fields("sent_at"),
	}
}
