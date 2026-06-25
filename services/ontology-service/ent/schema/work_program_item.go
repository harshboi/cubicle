package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramItem is one durable row in an AI-TPM operating register.
//
// Association:
//
//	Workstream -> WorkProgramItem -> WorkAction
//	WorkProgramItem -> optional typed PullRequest/Ticket subject
//
// WorkAction is the gated action ledger. WorkProgramItem is the product-facing
// register projection that carries owner, TPM bucket, dependency, coverage, and
// next-action context without forcing UI reads back through analytics tables.
type WorkProgramItem struct {
	ent.Schema
}

// Annotations declares WorkProgramItem as a public product graph object.
func (WorkProgramItem) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_program_items_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines the denormalized TPM program-register projection.
func (WorkProgramItem) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this register item belongs to."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional WorkAction row that produced this register item."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the register item is about a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the register item is about a ticket."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key for grouping program items."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Typed product kind this program item is about."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key or source-neutral subject key this item is about."),
			field.Text("linked_ticket_keys").
				Optional().
				Comment("Display list of ticket keys linked to this item."),
			field.Text("linked_pull_request_keys").
				Optional().
				Comment("Display list of pull request keys linked to this item."),
			field.Text("title").
				NotEmpty().
				Comment("Human-readable register row title."),
			field.Enum("program_status").
				Values(workProgramStatusValues()...).
				Default(workProgramStatusUnknown).
				Comment("TPM operating status for this register item."),
			field.Enum("tpm_bucket").
				Values(workProgramBucketValues()...).
				Default(workProgramBucketUnknown).
				Comment("TPM bucket used for operating review and filtering."),
			field.String("owner_key").
				Optional().
				Comment("Current accountable person/team key."),
			field.String("owner_source").
				Optional().
				Comment("How owner_key was chosen."),
			field.String("author_dri").
				Optional().
				Comment("Author-derived DRI hint, when available."),
			field.Text("requested_reviewer_keys").
				Optional().
				Comment("Requested reviewer keys from the source subject."),
			field.Text("reviewer_or_approver").
				Optional().
				Comment("Reviewer or approver display hint for TPM routing."),
			field.Text("next_action").
				Optional().
				Comment("Concrete TPM next action generated for this item."),
			field.Text("decision_needed").
				Optional().
				Comment("Decision or validation needed before the item can move."),
			field.Enum("decision_state").
				Values(workActionDecisionStateValues()...).
				Default(workActionDecisionPendingReview).
				Comment("Underlying WorkAction gate state."),
			field.Text("decision_gate_reason").
				Optional().
				Comment("Why this item was gated into its current decision state."),
			field.Enum("due_bucket").
				Values(workActionDueBucketValues()...).
				Default(workActionDueUnscheduled).
				Comment("Operating due bucket for this register row."),
			field.Time("last_source_update_at").
				Optional().
				Comment("Latest source update time for the subject, when known."),
			field.Float("age_days").
				Optional().
				Comment("Subject age in days at analytics generation time."),
			field.Float("stale_days").
				Optional().
				Comment("Subject staleness in days at analytics generation time."),
			field.Float("risk_score").
				Default(0).
				Comment("Generated priority/risk score."),
			field.String("blocker_label_state").
				Optional().
				Comment("Current blocker label or measurement state."),
			field.String("ci_signal").
				Optional().
				Comment("Current CI signal summary, when relevant."),
			field.String("transition_state").
				Optional().
				Comment("State-transition signal associated with this item."),
			field.Text("dependency_summary").
				Optional().
				Comment("Small display summary of linked dependencies."),
			field.Text("source_coverage_state").
				Optional().
				Comment("Latest source coverage state supporting this row."),
			field.Text("label_quality").
				Optional().
				Comment("Measurement label quality or not-required state."),
			field.Time("register_updated_at").
				Optional().
				Comment("Analytics timestamp for this program-register row."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the register projection to its typed operating/product rows.
func (WorkProgramItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream that owns this register item."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Action ledger row represented by this program item."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request subject, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket subject, when resolved."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this register item."),
		edge.From("links", WorkProgramItemLink.Type).
			Ref("work_program_item").
			Comment("Structured product and open-graph links attached to this register item."),
	}
}

// Indexes supports bounded TPM register reads by workstream, status, owner, and subject.
func (WorkProgramItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "program_status", "due_bucket", "risk_score", "last_activity_at"),
		index.Fields("workstream_id", "program_status", "due_bucket", "risk_score"),
		index.Fields("subject_kind", "subject_key", "program_status"),
		index.Fields("subject_object_type", "subject_key", "program_status"),
		index.Fields("owner_key", "program_status", "due_bucket", "risk_score"),
		index.Fields("work_action_id").Unique(),
		index.Fields("pull_request_id", "program_status", "risk_score"),
		index.Fields("ticket_id", "program_status", "risk_score"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
