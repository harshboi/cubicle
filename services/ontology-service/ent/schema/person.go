package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Person is a human identity observed across source systems.
type Person struct {
	ent.Schema
}

// Annotations declares that Person is part of the future public entgql API.
func (Person) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines source-neutral identity fields plus convenient denormalized handles.
func (Person) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable person name shown in graph and search results."),
			field.String("primary_email").
				Optional().
				Comment("Best-known email address when a source exposes one."),
			field.String("github_login").
				Optional().
				Comment("GitHub username used for code and pull-request activity matching."),
			field.String("jira_account_id").
				Optional().
				Comment("Jira account identifier used for ticket activity matching."),
			field.String("slack_user_id").
				Optional().
				Comment("Slack user identifier used for message and mention matching."),
			field.String("google_account_id").
				Optional().
				Comment("Google account identifier used for Docs and Drive matching."),
			field.String("avatar_url").
				Optional().
				Comment("Optional profile image URL for UI rendering."),
		},
		textFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects Person only to low-cardinality identity and serving parents.
//
// Person/work facts are introduced later as typed relationship rows and bounded
// serving parents, never as direct Person fanout to high-cardinality work.
func (Person) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_areas", WorkArea.Type).
			Comment("Bounded Cubicle work areas owned by this person."),
		edge.To("identities", PersonIdentity.Type).
			Comment("Source-native identities and handles that resolve to this person."),
	}
}

// Indexes supports common source-identity lookups without making those handles
// globally required.
func (Person) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("primary_email"),
		index.Fields("github_login"),
		index.Fields("jira_account_id"),
		index.Fields("slack_user_id"),
		index.Fields("google_account_id"),
	}
}
