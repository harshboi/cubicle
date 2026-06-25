package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkActionObservation records the current source evidence behind a WorkAction.
//
// Association:
//
//	WorkAction -> WorkActionObservation -> optional Evidence
//
// Observations explain whether a generated action is supported by current
// source state, CI evidence, model/rule QA, suppression evidence, or source
// coverage repair needs. They do not turn connector errors into product claims.
type WorkActionObservation struct {
	ent.Schema
}

// Annotations declares that WorkActionObservation is intended for future public GraphQL reads.
func (WorkActionObservation) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the source observation and whether it supports product action.
func (WorkActionObservation) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_action_id").
				Immutable().
				Comment("WorkAction this observation supports or qualifies."),
			field.Enum("observation_kind").
				Values(workActionObservationKindValues()...).
				Default(workActionObservationSourceState).
				Comment("Kind of observation attached to the action."),
			field.String("source_coverage_state").
				Optional().
				Comment("Coverage or freshness state of the source read used for this observation."),
			field.String("auth_state").
				Optional().
				Comment("Authentication or authorization state for the source read."),
			field.String("current_state").
				Optional().
				Comment("Current source lifecycle state observed for the subject."),
			field.String("ci_signal").
				Optional().
				Comment("Current CI signal summary, when this observation came from checks/statuses."),
			field.String("ci_required_check_coverage_state").
				Optional().
				Comment("Whether required-check configuration was observed for the PR branch."),
			field.String("ci_required_check_match_state").
				Optional().
				Comment("How required check contexts match the current head status/check payload."),
			field.Int("ci_required_context_count").
				Default(0).
				Comment("Required check contexts configured for the PR branch when observed."),
			field.Int("ci_failing_required_context_count").
				Default(0).
				Comment("Required check contexts currently failing."),
			field.Int("ci_pending_required_context_count").
				Default(0).
				Comment("Required check contexts currently pending."),
			field.Int("ci_missing_required_context_count").
				Default(0).
				Comment("Required check contexts missing from the observed head payload."),
			field.Text("ci_failing_required_contexts").
				Optional().
				Comment("Names of failing required contexts, comma-separated from source evidence."),
			field.Text("ci_pending_required_contexts").
				Optional().
				Comment("Names of pending required contexts, comma-separated from source evidence."),
			field.Text("ci_missing_required_contexts").
				Optional().
				Comment("Names of missing required contexts, comma-separated from source evidence."),
			field.Int("ci_failing_context_count").
				Default(0).
				Comment("All observed failing status/check contexts, required or not."),
			field.Int("ci_pending_context_count").
				Default(0).
				Comment("All observed pending status/check contexts, required or not."),
			field.Text("ci_failing_contexts").
				Optional().
				Comment("Names of all failing observed contexts, comma-separated from source evidence."),
			field.Text("ci_pending_contexts").
				Optional().
				Comment("Names of all pending observed contexts, comma-separated from source evidence."),
			field.Bool("supports_action").
				Default(false).
				Comment("Whether this observation currently supports a measurement-backed product action."),
			field.Time("observed_at").
				Optional().
				Comment("Time Cubicle observed this state."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects the observation to its action and optional evidence row.
func (WorkActionObservation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("action", WorkAction.Type).
			Ref("observations").
			Unique().
			Required().
			Immutable().
			Field("work_action_id").
			Comment("Action this observation supports or qualifies."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this action observation."),
	}
}

// Indexes supports action-ledger reads and source coverage audits.
func (WorkActionObservation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_action_id", "observation_kind", "observed_at"),
		index.Fields("observation_kind", "supports_action", "observed_at"),
		index.Fields("source_coverage_state", "auth_state"),
		index.Fields("ci_required_check_match_state", "ci_required_check_coverage_state"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
