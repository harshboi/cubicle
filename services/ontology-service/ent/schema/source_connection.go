package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceConnection is one configured workplace source instance, such as a Jira
// tenant, Slack workspace, GitHub installation, or Google Workspace.
type SourceConnection struct {
	ent.Schema
}

// Annotations declares that SourceConnection is storage-visible for ops tools.
func (SourceConnection) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines source connection configuration without per-object run state.
func (SourceConnection) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("source_system").
				NotEmpty().
				Comment("Source system family, such as jira, slack, github, or google_drive."),
			field.String("source_instance").
				NotEmpty().
				Comment("Tenant, workspace, repository owner, or installation namespace."),
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable connection name for ops screens."),
			field.String("connector_kind").
				Optional().
				Comment("Connector implementation key used by ingestion workers."),
			field.Bool("is_enabled").
				Default(true).
				Comment("Whether sync workers may run this source connection."),
			field.Time("last_synced_at").
				Optional().
				Comment("Most recent sync time across any scope in this connection."),
		},
		timestampFields(),
	)
}

// Edges connects a source connection to bounded scopes.
func (SourceConnection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("scopes", SourceScope.Type).
			Comment("Bounded crawl scopes configured under this source connection."),
	}
}

// Indexes supports connection lookup by source namespace.
func (SourceConnection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_system", "source_instance").Unique(),
		index.Fields("is_enabled", "source_system"),
	}
}
