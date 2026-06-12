package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceSyncIssue records connector warnings/errors at run or scope level. It
// is an ops table, not an edge in the product graph.
type SourceSyncIssue struct {
	ent.Schema
}

// Annotations declares that SourceSyncIssue is storage-visible for ops tools.
func (SourceSyncIssue) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one sync issue with optional source object address.
func (SourceSyncIssue) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("source_scope_id").
				Immutable().
				Comment("Scope where this issue occurred."),
			field.Int("source_sync_run_id").
				Optional().
				Comment("Sync run that emitted this issue."),
			field.Enum("severity").
				Values(sourceIssueSeverityValues()...).
				Default(sourceIssueSeverityWarning).
				Comment("Issue severity for operator triage."),
			field.String("issue_code").
				NotEmpty().
				Comment("Stable machine-readable issue code."),
			field.Text("message").
				Optional().
				Comment("Human-readable issue message."),
		},
		sourceIdentityFields(),
		timestampFields(),
	)
}

// Edges connects sync issues to scope and optional run.
func (SourceSyncIssue) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("scope", SourceScope.Type).
			Ref("sync_issues").
			Unique().
			Required().
			Immutable().
			Field("source_scope_id").
			Comment("Scope where this issue occurred."),
		edge.From("sync_run", SourceSyncRun.Type).
			Ref("issues").
			Unique().
			Field("source_sync_run_id").
			Comment("Sync run that emitted this issue."),
	}
}

// Indexes supports issue triage by scope and source address.
func (SourceSyncIssue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_scope_id", "severity", "created_at"),
		index.Fields("source_sync_run_id"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
