package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

const (
	workLensWindowTable = "work_lens_windows" // workLensWindowTable keeps the SQL table name explicit and plural.
)

// WorkLensWindow is a bounded partition of one WorkLens result set.
//
// Association:
//
//	WorkArea -> WorkLens -> WorkLensWindow -> {Document, PullRequest, Ticket, Message}LensResult
//
// Windows prevent a lens such as messages_authored from becoming the only
// parent for every message a person ever wrote. Writers and GraphQL resolvers
// can page by window first, then load ranked result rows inside that window.
type WorkLensWindow struct {
	ent.Schema
}

// Annotations declares WorkLensWindow's explicit SQL table and future GraphQL exposure.
func (WorkLensWindow) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Annotation{Table: workLensWindowTable})
}

// Fields defines the partition key, source/time bounds, and cached rollups.
func (WorkLensWindow) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_lens_id").
				Immutable().
				Comment("Parent WorkLens row whose result set this window partitions."),
			field.Enum("lens_window_kind").
				Values(lensWindowKindValues()...).
				Default(lensWindowRecent).
				Comment("Partition strategy for this bounded lens window."),
			field.Time("window_start_at").
				Optional().
				Comment("Inclusive source activity lower bound for time-bucket windows."),
			field.Time("window_end_at").
				Optional().
				Comment("Exclusive source activity upper bound for time-bucket windows."),
			field.Int("rank_start").
				Optional().
				Comment("Inclusive ranked offset represented by this window when rank paging is materialized."),
			field.Int("rank_end").
				Optional().
				Comment("Exclusive ranked offset represented by this window when rank paging is materialized."),
			field.String("checkpoint").
				Optional().
				Comment("Materialization cursor used to resume rebuilding this serving window."),
			field.Int("result_count").
				Default(0).
				Comment("Cached number of result rows assigned to this window."),
			field.Bool("is_complete").
				Default(false).
				Comment("Whether this serving window's result rows have been fully materialized."),
			field.Time("last_indexed_at").
				Optional().
				Comment("Time this window's results were last rebuilt or refreshed."),
		},
		sourceIdentityFields(),
		activityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects the window to its owning lens and target-specific result rows.
func (WorkLensWindow) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("lens", WorkLens.Type).
			Ref("windows").
			Unique().
			Required().
			Immutable().
			Field("work_lens_id").
			Comment("Work lens whose results this window bounds."),
		edge.To("document_results", DocumentLensResult.Type).
			Comment("Document results assigned to this bounded window."),
		edge.To("pull_request_results", PullRequestLensResult.Type).
			Comment("Pull request results assigned to this bounded window."),
		edge.To("ticket_results", TicketLensResult.Type).
			Comment("Ticket results assigned to this bounded window."),
		edge.To("message_results", MessageLensResult.Type).
			Comment("Message results assigned to this bounded window."),
	}
}

// Indexes supports locating windows before reading high-cardinality results.
func (WorkLensWindow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_lens_id", "lens_window_kind", "last_activity_at"),
		index.Fields("work_lens_id", "source_system", "window_start_at"),
	}
}
