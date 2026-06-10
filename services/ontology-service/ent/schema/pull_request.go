package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequest is a code change or review object from a source such as GitHub.
type PullRequest struct {
	ent.Schema
}

// Annotations declares that PullRequest is part of the future public entgql API.
func (PullRequest) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines source-native PR identity plus normalized search fields.
func (PullRequest) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("repository").
				Optional().
				Comment("Repository full name such as owner/name."),
			field.Int("number").
				Optional().
				Comment("Source pull-request number within the repository."),
			field.String("title").
				NotEmpty().
				Comment("Human-readable pull-request title."),
			field.Enum("state").
				Values(pullRequestStateUnknown, pullRequestStateOpen, pullRequestStateMerged, pullRequestStateClosed).
				Default(pullRequestStateUnknown).
				Comment("Normalized pull-request state."),
			field.Time("merged_at").
				Optional().
				Comment("Time the pull request merged, when available."),
		},
		textFields(),
		sourceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges exposes tickets implemented by this pull request and lenses ranking it.
func (PullRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tickets", Ticket.Type).
			Ref("pull_requests").
			Comment("Tickets this pull request implements."),
		edge.From("work_lenses", WorkLens.Type).
			Ref("pull_requests").
			Comment("Work lenses that ranked this pull request as a result."),
	}
}

// Indexes supports repository/number lookups and PR state filtering.
func (PullRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("repository", "number"),
		index.Fields("state", "last_activity_at"),
	}
}
