package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

const (
	evidenceAnchorPreviewMaxLen     = 512  // evidenceAnchorPreviewMaxLen bounds source text shown in local answers.
	evidenceAnchorFingerprintMaxLen = 2048 // evidenceAnchorFingerprintMaxLen bounds primitive Day 0 lexical lookup text.
)

// EvidenceAnchor is an exact citeable location inside a SourceObservation.
//
// It is source-neutral: a Google Doc paragraph, Slack reply, Jira comment, and
// GitHub review comment all become anchors without making search chunks or
// document blocks canonical truth.
type EvidenceAnchor struct {
	ent.Schema
}

// Annotations declares EvidenceAnchor as an internal citation schema.
func (EvidenceAnchor) Annotations() []entschema.Annotation {
	return nil
}

// Fields defines the source-local citation address plus bounded preview text.
func (EvidenceAnchor) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("source_observation_id").
				Immutable().
				Comment("SourceObservation row containing this exact evidence anchor."),
			field.String("anchor_kind").
				NotEmpty().
				Immutable().
				Comment("Source-neutral anchor kind such as doc_span, slack_message, jira_comment, or pr_review_comment."),
			field.String("anchor_locator").
				NotEmpty().
				Immutable().
				Comment("Source-local locator such as paragraph ID, message timestamp, comment ID, or review comment ID."),
			field.String("source_span_key").
				NotEmpty().
				Immutable().
				Comment("Normalized source span identity used for anchor dedupe within one observation."),
			field.Int("ordinal").
				Optional().
				Comment("Stable display order for anchors inside the observed source item."),
			field.String("text_hash").
				NotEmpty().
				Comment("Hash of the exact cited text span for drift detection and dedupe."),
			field.String("text_preview").
				Optional().
				MaxLen(evidenceAnchorPreviewMaxLen).
				Comment("Bounded source-derived snippet for inspectable local answers."),
			field.Bool("text_preview_truncated").
				Default(false).
				Comment("Whether text_preview omits part of the cited source span."),
			field.String("lexical_fingerprint").
				Optional().
				MaxLen(evidenceAnchorFingerprintMaxLen).
				Comment("Bounded normalized token fingerprint for primitive Day 0 lookup, not a search index."),
		},
		timestampFields(),
	)
}

// Edges connects the anchor to its source observation and graph evidence rows.
func (EvidenceAnchor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source_observation", SourceObservation.Type).
			Ref("evidence_anchors").
			Unique().
			Required().
			Immutable().
			Field("source_observation_id").
			Comment("Observed source item that contains this anchor."),
		edge.From("evidences", Evidence.Type).
			Ref("evidence_anchor").
			Comment("Graph evidence rows that cite this anchor."),
	}
}

// Indexes deduplicates anchors and supports ordered reads inside observations.
func (EvidenceAnchor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_observation_id", "anchor_kind", "source_span_key", "text_hash").Unique(),
		index.Fields("source_observation_id", "ordinal"),
	}
}
