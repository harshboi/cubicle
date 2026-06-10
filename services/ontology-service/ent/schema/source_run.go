package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceRun records one connector pass over a declared source scope.
//
// This row is the authority for crawl coverage. Object rows, lens windows, and
// evidence anchors may expose derived freshness, but only SourceRun can say a
// Slack channel, Jira project, GitHub repository, or Google document scope was
// complete, partial, failed, or rate-limited.
type SourceRun struct {
	ent.Schema
}

// Annotations declares SourceRun as an internal Ent schema for source coverage.
func (SourceRun) Annotations() []entschema.Annotation {
	return nil
}

// Fields defines source scope identity plus terminal crawl status.
func (SourceRun) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("run_key").
				NotEmpty().
				Immutable().
				Comment("Connector-provided idempotency key for this source run."),
			field.String("source_key").
				NotEmpty().
				Immutable().
				Comment("Source family such as slack, jira, github, or google_docs."),
			field.String("source_instance").
				NotEmpty().
				Immutable().
				Comment("Source namespace such as workspace, tenant, account, or repository owner."),
			field.String("scope_kind").
				NotEmpty().
				Immutable().
				Comment("Type of source scope crawled, such as channel, project, repo, document, or folder."),
			field.String("scope_key").
				NotEmpty().
				Immutable().
				Comment("Source-local scope identifier, such as a Slack channel ID or Jira project key."),
			field.Enum("status").
				Values(sourceRunStatusValues()...).
				Default(sourceRunStatusRunning).
				Comment("Terminal or in-progress coverage status for this source scope."),
			field.Time("started_at").
				Optional().
				Comment("Time the connector started crawling this source scope."),
			field.Time("completed_at").
				Optional().
				Comment("Time the connector reached a terminal status for this source scope."),
			field.Time("coverage_start_at").
				Optional().
				Comment("Inclusive lower source-time bound covered by this run when known."),
			field.Time("coverage_end_at").
				Optional().
				Comment("Exclusive upper source-time bound covered by this run when known."),
			field.String("checkpoint_token").
				Optional().
				Comment("Opaque connector cursor used to resume or explain partial coverage."),
			field.String("error_code").
				Optional().
				Comment("Bounded source or connector failure classification for failed or partial runs."),
			field.String("error_message").
				Optional().
				Comment("Short human-readable failure detail for local debugging."),
		},
		timestampFields(),
	)
}

// Edges connects a source run to the item observations it produced.
func (SourceRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("observations", SourceObservation.Type).
			Comment("Source observations produced by this connector run."),
	}
}

// Indexes makes source runs idempotent and supports latest-run coverage checks.
func (SourceRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_key", "source_instance", "run_key").Unique(),
		index.Fields("source_key", "source_instance", "scope_kind", "scope_key", "started_at"),
	}
}
