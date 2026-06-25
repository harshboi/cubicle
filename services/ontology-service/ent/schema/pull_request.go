package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequest is a code change or review object from a source such as GitHub.
//
// Association:
//
//	Ticket -> TicketPullRequest -> PullRequest
//	Person -> PullRequestAuthorship/PullRequestReview -> PullRequest
//
// Pull-request people links stay typed so authorship and review do not blur.
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
			field.Time("source_created_at").
				Optional().
				Comment("Source-reported pull-request creation time, used for age and cycle analysis."),
			field.Time("merged_at").
				Optional().
				Comment("Time the pull request merged, when available."),
			field.Time("closed_at").
				Optional().
				Comment("Time the pull request closed without necessarily merging, when available."),
			field.Int("additions").
				Optional().
				Nillable().
				Comment("Source-reported lines added in the pull request diff."),
			field.Int("deletions").
				Optional().
				Nillable().
				Comment("Source-reported lines deleted in the pull request diff."),
			field.Int("changed_files_count").
				Optional().
				Nillable().
				Comment("Source-reported number of files changed by the pull request."),
			field.Int("commit_count").
				Optional().
				Nillable().
				Comment("Source-reported number of commits in the pull request."),
			field.Int("issue_comment_count").
				Optional().
				Nillable().
				Comment("Source-reported number of issue-thread comments on the pull request."),
			field.Int("review_comment_count").
				Optional().
				Nillable().
				Comment("Source-reported number of code review comments on the pull request."),
			field.Bool("is_draft").
				Optional().
				Nillable().
				Comment("Whether the pull request is marked draft by the source."),
			field.Bool("is_mergeable").
				Optional().
				Nillable().
				Comment("Whether the source currently reports the pull request as mergeable; unset means unknown."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges exposes tickets implemented by this pull request.
func (PullRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tickets", Ticket.Type).
			Ref("pull_requests").
			Comment("Tickets this pull request implements."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this pull request state."),
		edge.From("insights", WorkInsight.Type).
			Ref("pull_request").
			Comment("Generated TPM/product insights about this pull request."),
		edge.From("actions", WorkAction.Type).
			Ref("pull_request").
			Comment("Gated TPM actions about this pull request."),
		edge.From("state_snapshots", WorkItemStateSnapshot.Type).
			Ref("pull_request").
			Comment("Observed state snapshots for this pull request."),
		edge.From("state_transitions", WorkItemStateTransition.Type).
			Ref("pull_request").
			Comment("Observed state transitions for this pull request."),
		edge.From("milestones", WorkProgramMilestone.Type).
			Ref("pull_request").
			Comment("Source-backed milestone and date signals for this pull request."),
	}
}

// Indexes supports repository/number lookups and PR state filtering.
func (PullRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("repository", "number"),
		index.Fields("state", "last_activity_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
