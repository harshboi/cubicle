package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceSyncRun is one connector execution against a bounded SourceScope. It
// records run coverage and failures without creating edges to every source item.
type SourceSyncRun struct {
	ent.Schema
}

// Annotations declares that SourceSyncRun is storage-visible for ops tools.
func (SourceSyncRun) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines sync-run metadata at scope granularity.
func (SourceSyncRun) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("source_scope_id").
				Immutable().
				Comment("SourceScope this run processed."),
			field.String("run_key").
				NotEmpty().
				Comment("Idempotency key for this sync execution."),
			field.Enum("sync_mode").
				Values(sourceSyncModeValues()...).
				Default(sourceSyncModeSnapshot).
				Comment("How the connector processed this scope."),
			field.Enum("coverage_mode").
				Values(sourceCoverageValues()...).
				Default(sourceCoverageUnknown).
				Comment("Coverage semantics produced by this sync execution."),
			field.Enum("status").
				Values(sourceSyncStatusValues()...).
				Default(sourceSyncStatusRunning).
				Comment("Terminal or active sync status."),
			field.Time("started_at").
				Optional().
				Comment("Time this sync run started."),
			field.Time("completed_at").
				Optional().
				Comment("Time this sync run reached a terminal state."),
			field.Time("coverage_start_at").
				Optional().
				Comment("Inclusive lower source-time bound covered by this run."),
			field.Time("coverage_end_at").
				Optional().
				Comment("Exclusive upper source-time bound covered by this run."),
			field.String("checkpoint_token").
				Optional().
				Comment("Connector cursor produced by this run."),
			field.Int("objects_seen_count").
				Default(0).
				Comment("Number of source objects observed or referenced by this run."),
			field.Int("objects_created_count").
				Default(0).
				Comment("Number of typed product objects first materialized by this run."),
			field.Int("objects_updated_count").
				Default(0).
				Comment("Number of typed product objects updated by this run."),
			field.Int("objects_deleted_count").
				Default(0).
				Comment("Number of typed product objects marked deleted by this run."),
			field.Int("relationships_created_count").
				Default(0).
				Comment("Number of typed relationship rows first materialized by this run."),
			field.Int("relationships_updated_count").
				Default(0).
				Comment("Number of typed relationship rows updated by this run."),
			field.Int("relationships_deleted_count").
				Default(0).
				Comment("Number of typed relationship rows marked deleted by this run."),
			field.Int("evidence_created_count").
				Default(0).
				Comment("Number of Evidence rows materialized by this run."),
			field.Int("issues_created_count").
				Default(0).
				Comment("Number of SourceSyncIssue rows emitted by this run."),
			field.String("error_code").
				Optional().
				Comment("Run-level connector error code."),
			field.Text("error_message").
				Optional().
				Comment("Run-level connector error message."),
		},
		timestampFields(),
	)
}

// Edges connects a sync run to its scope and issues.
func (SourceSyncRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("scope", SourceScope.Type).
			Ref("sync_runs").
			Unique().
			Required().
			Immutable().
			Field("source_scope_id").
			Comment("Scope processed by this sync run."),
		edge.To("issues", SourceSyncIssue.Type).
			Comment("Issues emitted during this sync run."),
	}
}

// Indexes supports run lookup without per-item fan-out.
func (SourceSyncRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_scope_id", "run_key").Unique(),
		index.Fields("source_scope_id", "status", "started_at"),
	}
}
