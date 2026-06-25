package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkResponsibility is a typed accountability edge in the work graph.
//
// It keeps owner routing out of ad hoc owner_key strings without forcing every
// party hint to resolve to a Person. Evidence proves the row; this row is the
// durable relationship product reads and graph analytics traverse.
type WorkResponsibility struct {
	ent.Schema
}

// Annotations declares the one-of subject invariant for responsibility rows.
func (WorkResponsibility) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_responsibilities_subject_pointer_matches_kind": "((" +
			"subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL AND workstream_id IS NULL AND work_action_id IS NULL AND work_blocker_id IS NULL AND work_program_evidence_need_id IS NULL" +
			") OR (" +
			"subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL AND workstream_id IS NULL AND work_action_id IS NULL AND work_blocker_id IS NULL AND work_program_evidence_need_id IS NULL" +
			") OR (" +
			"subject_kind = 'workstream' AND workstream_id IS NOT NULL AND pull_request_id IS NULL AND ticket_id IS NULL AND work_action_id IS NULL AND work_blocker_id IS NULL AND work_program_evidence_need_id IS NULL" +
			") OR (" +
			"subject_kind = 'work_action' AND work_action_id IS NOT NULL AND pull_request_id IS NULL AND ticket_id IS NULL AND workstream_id IS NULL AND work_blocker_id IS NULL AND work_program_evidence_need_id IS NULL" +
			") OR (" +
			"subject_kind = 'work_blocker' AND work_blocker_id IS NOT NULL AND pull_request_id IS NULL AND ticket_id IS NULL AND workstream_id IS NULL AND work_action_id IS NULL AND work_program_evidence_need_id IS NULL" +
			") OR (" +
			"subject_kind = 'work_program_evidence_need' AND work_program_evidence_need_id IS NOT NULL AND pull_request_id IS NULL AND ticket_id IS NULL AND workstream_id IS NULL AND work_action_id IS NULL AND work_blocker_id IS NULL" +
			"))",
		"work_responsibilities_person_party_consistent": "((party_kind = 'person' AND person_id IS NOT NULL) OR (party_kind != 'person' AND person_id IS NULL))",
	}))
}

// Fields defines accountable party routing for work objects.
func (WorkResponsibility) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("person_id").
				Optional().
				Comment("Optional canonical Person row when party_key resolved without guessing."),
			field.Int("workstream_id").
				Optional().
				Comment("Resolved Workstream subject when subject_kind is workstream."),
			field.Int("pull_request_id").
				Optional().
				Comment("Resolved PullRequest subject when subject_kind is pull_request."),
			field.Int("ticket_id").
				Optional().
				Comment("Resolved Ticket subject when subject_kind is ticket."),
			field.Int("work_action_id").
				Optional().
				Comment("Resolved WorkAction subject when subject_kind is work_action."),
			field.Int("work_blocker_id").
				Optional().
				Comment("Resolved WorkBlocker subject when subject_kind is work_blocker."),
			field.Int("work_program_evidence_need_id").
				Optional().
				Comment("Resolved WorkProgramEvidenceNeed subject when subject_kind is work_program_evidence_need."),
			field.Int("work_program_item_id").
				Optional().
				Comment("Optional WorkProgramItem projection context that surfaced this responsibility."),
			field.String("workstream_key").
				Optional().
				Comment("Stable workstream key this responsibility participates in."),
			field.Enum("subject_kind").
				Values(workResponsibilitySubjectKindValues()...).
				Comment("Typed work subject this responsibility points at."),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable key of the subject work object."),
			field.Enum("party_kind").
				Values(workResponsibilityPartyKindValues()...).
				Comment("Type of accountable party represented by party_key."),
			field.String("party_key").
				NotEmpty().
				Comment("Source-neutral party, team, unresolved identity, or unassigned placeholder key."),
			field.String("party_source").
				Optional().
				Comment("How party_key was selected, such as github.pr.author, jira.issue.assignee, or generated.owner_hint."),
			field.Enum("responsibility_kind").
				Values(workResponsibilityKindValues()...).
				Comment("Semantic accountability role for this party-subject relationship."),
			field.Enum("basis_kind").
				Values(workResponsibilityBasisKindValues()...).
				Default(workResponsibilityBasisGeneratedCandidate).
				Comment("Authority or derivation path for this responsibility."),
			field.String("basis_detail").
				Optional().
				Comment("Source field, rule, or relationship that produced this responsibility."),
			field.Enum("responsibility_state").
				Values(workResponsibilityStateValues()...).
				Default(workResponsibilityStateCandidate).
				Comment("Lifecycle state for this responsibility claim."),
			field.String("responsibility_state_reason").
				Optional().
				Comment("Short explanation for the current responsibility state."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this responsibility row."),
			field.Time("valid_from").
				Optional().
				Comment("First time this responsibility became valid in the source or generated graph."),
			field.Time("valid_until").
				Optional().
				Comment("Time this responsibility stopped being valid, when known."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects responsibility rows to optional person identity, typed subject,
// optional projection context, and evidence.
func (WorkResponsibility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Field("person_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Canonical person resolved for party_key, if known."),
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target Workstream when the responsibility is on a workstream."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target PullRequest when the responsibility is on a PR."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target Ticket when the responsibility is on a ticket."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target WorkAction when the responsibility is on an action ledger row."),
		edge.To("work_blocker", WorkBlocker.Type).
			Unique().
			Field("work_blocker_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target WorkBlocker when the responsibility is on a blocker claim."),
		edge.To("work_program_item", WorkProgramItem.Type).
			Unique().
			Field("work_program_item_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Optional WorkProgramItem projection context."),
		edge.To("work_program_evidence_need", WorkProgramEvidenceNeed.Type).
			Unique().
			Field("work_program_evidence_need_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Target WorkProgramEvidenceNeed when the responsibility is on an evidence gap."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this responsibility row."),
	}
}

// Indexes supports owner traversal, target reverse lookup, run scoping, and source upserts.
func (WorkResponsibility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("party_key", "responsibility_kind", "responsibility_state", "rank_score", "last_activity_at"),
		index.Fields("person_id", "responsibility_kind", "responsibility_state", "rank_score", "last_activity_at"),
		index.Fields("party_kind", "party_key"),
		index.Fields("subject_kind", "subject_key", "party_key", "responsibility_kind", "basis_kind").Unique(),
		index.Fields("workstream_key", "responsibility_kind", "responsibility_state", "rank_score", "last_activity_at"),
		index.Fields("workstream_id", "responsibility_kind"),
		index.Fields("pull_request_id", "responsibility_kind"),
		index.Fields("ticket_id", "responsibility_kind"),
		index.Fields("work_action_id", "responsibility_kind"),
		index.Fields("work_blocker_id", "responsibility_kind"),
		index.Fields("work_program_item_id", "responsibility_kind"),
		index.Fields("work_program_evidence_need_id", "responsibility_kind"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
