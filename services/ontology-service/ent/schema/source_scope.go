package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceScope is a bounded crawl/read scope inside one source connection, such
// as a Jira project, Slack channel, GitHub repository, or Drive folder.
type SourceScope struct {
	ent.Schema
}

// Annotations declares that SourceScope is storage-visible for ops tools.
func (SourceScope) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the stable scope identity and policy.
func (SourceScope) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("source_connection_id").
				Immutable().
				Comment("SourceConnection that owns this bounded scope."),
			field.String("scope_kind").
				NotEmpty().
				Comment("Source-local scope kind, such as project, channel, repository, or folder."),
			field.String("scope_key").
				NotEmpty().
				Comment("Source-local scope identifier."),
			field.String("display_name").
				Optional().
				Comment("Human-readable scope name."),
			field.String("crawl_policy").
				Optional().
				Comment("Connector-specific policy key for how this scope should be synced."),
			field.Bool("is_enabled").
				Default(true).
				Comment("Whether sync workers may process this scope."),
		},
		timestampFields(),
	)
}

// Edges connects the scope to current state, sync runs, and issues.
func (SourceScope) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("connection", SourceConnection.Type).
			Ref("scopes").
			Unique().
			Required().
			Immutable().
			Field("source_connection_id").
			Comment("Source connection that owns this scope."),
		edge.To("states", SourceScopeState.Type).
			Comment("Coverage/freshness state rows for this scope."),
		edge.To("sync_runs", SourceSyncRun.Type).
			Comment("Sync runs executed against this scope."),
		edge.To("sync_issues", SourceSyncIssue.Type).
			Comment("Connector issues observed for this scope."),
	}
}

// Indexes supports scope lookup under a connection.
func (SourceScope) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_connection_id", "scope_kind", "scope_key").Unique(),
		index.Fields("is_enabled", "scope_kind"),
	}
}
