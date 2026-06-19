package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceScopeState is the current coverage/freshness state for a bounded source
// scope. Product objects point to this by ID only when they need a coverage
// explanation; product graph traversals should not enter here.
//
// Association:
//
//	SourceScope -> SourceScopeState -> latest successful SourceSyncRun
//	product rows -. optional coverage explanation .-> SourceScopeState
//
// This row explains source coverage without becoming normal product adjacency.
type SourceScopeState struct {
	ent.Schema
}

// Annotations declares that SourceScopeState is storage-visible for ops tools.
func (SourceScopeState) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines current scope-level coverage state.
func (SourceScopeState) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("source_scope_id").
				Immutable().
				Comment("SourceScope this state row describes."),
			field.Enum("freshness_state").
				Values(freshnessValues()...).
				Default(freshnessUnknown).
				Comment("Current freshness state for the entire scope."),
			field.Enum("coverage_mode").
				Values(sourceCoverageValues()...).
				Default(sourceCoverageUnknown).
				Comment("Coverage semantics for the latest successful or attempted sync."),
			field.Int("last_successful_sync_run_id").
				Optional().
				Comment("Latest successful SourceSyncRun for this scope."),
			field.Time("last_successful_at").
				Optional().
				Comment("Time the scope last completed successfully."),
			field.Time("last_attempted_at").
				Optional().
				Comment("Time the scope was last attempted by a sync worker."),
			field.String("checkpoint_token").
				Optional().
				Comment("Connector cursor used to resume the scope."),
			field.String("error_code").
				Optional().
				Comment("Most recent scope-level connector error code."),
			field.Text("error_message").
				Optional().
				Comment("Most recent scope-level connector error message."),
		},
		timestampFields(),
	)
}

// Edges connects state to its scope and latest successful run.
func (SourceScopeState) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("scope", SourceScope.Type).
			Ref("states").
			Unique().
			Required().
			Immutable().
			Field("source_scope_id").
			Comment("Scope whose current state this row records."),
		edge.To("last_successful_sync_run", SourceSyncRun.Type).
			Unique().
			Field("last_successful_sync_run_id").
			Comment("Latest successful sync run for this scope."),
	}
}

// Indexes keeps one current state row per scope.
func (SourceScopeState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_scope_id").Unique(),
		index.Fields("freshness_state", "last_attempted_at"),
	}
}
