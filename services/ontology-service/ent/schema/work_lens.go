package schema

import (
	"context"
	"fmt"

	"cubicle/services/ontology-service/internal/ontology"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

const (
	workLensTable = "work_lenses" // workLensTable keeps SQL storage plural because Ent's default for "lens" is singular.
)

// WorkLens is a bounded, ranked view within a person's work area.
//
// A lens is the cardinality-control node between a broad person work area and
// high-volume target activity. Result tables are added in later PRs so each
// target association can be reviewed independently.
type WorkLens struct {
	ent.Schema
}

// Annotations declares that WorkLens is part of the future public entgql API.
func (WorkLens) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Annotation{Table: workLensTable})
}

// Fields defines lens identity, target declaration, and cached rollups.
func (WorkLens) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_area_id").
				Immutable().
				Comment("Parent WorkArea row that owns this lens."),
			field.Enum("work_lens_kind").
				Values(workLensKindValues()...).
				Immutable().
				Comment("Specific bounded view represented by this lens."),
			field.Enum("lens_target_kind").
				Values(lensTargetKindValues()...).
				Immutable().
				Comment("Only target kind this lens is allowed to expose."),
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable lens label, such as Documents Commented On."),
			field.Text("description").
				Optional().
				Comment("Short explanation of the lens's semantic relationship."),
			field.Int("result_count").
				Default(0).
				Comment("Cached number of result rows behind this lens."),
			field.Int("source_count").
				Default(0).
				Comment("Number of source systems that contributed to this lens."),
			field.Bool("is_complete").
				Default(false).
				Comment("Whether all configured sources for this lens have been crawled."),
			field.Time("last_indexed_at").
				Optional().
				Comment("Time this lens's rollups were last rebuilt or updated."),
		},
		activityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Hooks rejects WorkLens rows whose lens kind and target kind disagree.
func (WorkLens) Hooks() []ent.Hook {
	return []ent.Hook{
		validateWorkLensTargetKind(),
	}
}

// validateWorkLensTargetKind returns the same-row invariant hook for WorkLens.
func validateWorkLensTargetKind() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			lensKindValue, lensKindWasSet := mutation.Field("work_lens_kind")
			lensTargetKindValue, lensTargetKindWasSet := mutation.Field("lens_target_kind")
			if lensKindWasSet && lensTargetKindWasSet {
				lensKind := ontology.WorkLensKind(fmt.Sprint(lensKindValue))
				lensTargetKind := ontology.LensTargetKind(fmt.Sprint(lensTargetKindValue))
				if err := ontology.ValidateWorkLensTargetKind(lensKind, lensTargetKind); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// Edges connects a lens to its parent work area and typed result targets.
func (WorkLens) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("area", WorkArea.Type).
			Ref("lenses").
			Unique().
			Required().
			Immutable().
			Field("work_area_id").
			Comment("Parent work area that owns this lens."),
		edge.To("windows", WorkLensWindow.Type).
			Comment("Bounded windows that partition this lens before result rows are loaded."),
		edge.To("documents", Document.Type).
			Through("document_results", DocumentLensResult.Type).
			Comment("Documents ranked under this lens."),
		edge.To("pull_requests", PullRequest.Type).
			Through("pull_request_results", PullRequestLensResult.Type).
			Comment("Pull requests ranked under this lens."),
		edge.To("tickets", Ticket.Type).
			Through("ticket_results", TicketLensResult.Type).
			Comment("Tickets ranked under this lens."),
		edge.To("messages", Message.Type).
			Through("message_results", MessageLensResult.Type).
			Comment("Messages ranked under this lens."),
	}
}

// Indexes prevents duplicate lenses under a work area and supports fast lens
// discovery for person-centered graph traversal.
func (WorkLens) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_area_id", "work_lens_kind").Unique(),
		index.Fields("work_area_id", "lens_target_kind", "rank_score", "last_activity_at"),
	}
}
