package ontologyhooks

import (
	"context"
	"fmt"

	genent "cubicle/services/ontology-service/ent"
	enthook "cubicle/services/ontology-service/ent/hook"
	"cubicle/services/ontology-service/internal/ontology"
)

// Register installs Cubicle ontology invariants on a generated Ent client.
//
// Ent schema hooks can validate same-row fields. WorkLens placement needs to
// load the parent WorkArea, so it lives in this generated-client hook layer.
func Register(client *genent.Client) {
	if client == nil {
		return
	}
	client.WorkLens.Use(enthook.On(validateWorkLensPlacement(), genent.OpCreate))
	client.DocumentLensResult.Use(enthook.On(validateDocumentLensResult(), genent.OpCreate))
	client.PullRequestLensResult.Use(enthook.On(validatePullRequestLensResult(), genent.OpCreate))
	client.TicketLensResult.Use(enthook.On(validateTicketLensResult(), genent.OpCreate))
	client.MessageLensResult.Use(enthook.On(validateMessageLensResult(), genent.OpCreate))
}

// validateWorkLensPlacement returns the create hook that checks parent area fit.
func validateWorkLensPlacement() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.WorkLensFunc(func(ctx context.Context, mutation *genent.WorkLensMutation) (genent.Value, error) {
			if err := validateWorkLensMutation(ctx, mutation); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// validateWorkLensMutation checks that the lens is under its canonical work area.
func validateWorkLensMutation(ctx context.Context, mutation *genent.WorkLensMutation) error {
	workAreaID, ok := mutation.WorkAreaID()
	if !ok {
		return missingFieldError("work_lenses", "work_area_id")
	}
	lensKind, ok := mutation.WorkLensKind()
	if !ok {
		return missingFieldError("work_lenses", "work_lens_kind")
	}
	lensTargetKind, ok := mutation.LensTargetKind()
	if !ok {
		return missingFieldError("work_lenses", "lens_target_kind")
	}
	workArea, err := mutation.Client().WorkArea.Get(ctx, workAreaID)
	if err != nil {
		return fmt.Errorf("load work area %d for lens validation: %w", workAreaID, err)
	}
	return ontology.ValidateWorkLensPlacement(
		ontology.WorkLensKind(lensKind),
		ontology.WorkAreaKind(workArea.WorkAreaKind),
		ontology.LensTargetKind(lensTargetKind),
	)
}

// validateDocumentLensResult returns the create hook for document result writes.
func validateDocumentLensResult() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.DocumentLensResultFunc(func(ctx context.Context, mutation *genent.DocumentLensResultMutation) (genent.Value, error) {
			workLensID, ok := mutation.WorkLensID()
			if !ok {
				return nil, missingFieldError("document_lens_results", "work_lens_id")
			}
			workLensWindowID, ok := mutation.WorkLensWindowID()
			if !ok {
				return nil, missingFieldError("document_lens_results", "work_lens_window_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("document_lens_results", "relation_kind")
			}
			if err := validateLensResult(ctx, mutation.Client(), workLensID, workLensWindowID, ontology.LensTargetDocument, ontology.WorkRelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// validatePullRequestLensResult returns the create hook for pull-request result writes.
func validatePullRequestLensResult() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.PullRequestLensResultFunc(func(ctx context.Context, mutation *genent.PullRequestLensResultMutation) (genent.Value, error) {
			workLensID, ok := mutation.WorkLensID()
			if !ok {
				return nil, missingFieldError("pull_request_lens_results", "work_lens_id")
			}
			workLensWindowID, ok := mutation.WorkLensWindowID()
			if !ok {
				return nil, missingFieldError("pull_request_lens_results", "work_lens_window_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("pull_request_lens_results", "relation_kind")
			}
			if err := validateLensResult(ctx, mutation.Client(), workLensID, workLensWindowID, ontology.LensTargetPullRequest, ontology.WorkRelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// validateTicketLensResult returns the create hook for ticket result writes.
func validateTicketLensResult() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.TicketLensResultFunc(func(ctx context.Context, mutation *genent.TicketLensResultMutation) (genent.Value, error) {
			workLensID, ok := mutation.WorkLensID()
			if !ok {
				return nil, missingFieldError("ticket_lens_results", "work_lens_id")
			}
			workLensWindowID, ok := mutation.WorkLensWindowID()
			if !ok {
				return nil, missingFieldError("ticket_lens_results", "work_lens_window_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("ticket_lens_results", "relation_kind")
			}
			if err := validateLensResult(ctx, mutation.Client(), workLensID, workLensWindowID, ontology.LensTargetTicket, ontology.WorkRelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// validateMessageLensResult returns the create hook for message result writes.
func validateMessageLensResult() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.MessageLensResultFunc(func(ctx context.Context, mutation *genent.MessageLensResultMutation) (genent.Value, error) {
			workLensID, ok := mutation.WorkLensID()
			if !ok {
				return nil, missingFieldError("message_lens_results", "work_lens_id")
			}
			workLensWindowID, ok := mutation.WorkLensWindowID()
			if !ok {
				return nil, missingFieldError("message_lens_results", "work_lens_window_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("message_lens_results", "relation_kind")
			}
			if err := validateLensResult(ctx, mutation.Client(), workLensID, workLensWindowID, ontology.LensTargetMessage, ontology.WorkRelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

// validateLensResult checks that a lens result table matches its parent lens and window.
func validateLensResult(ctx context.Context, client *genent.Client, workLensID int, workLensWindowID int, resultLensTargetKind ontology.LensTargetKind, relationKind ontology.WorkRelationKind) error {
	lens, err := client.WorkLens.Get(ctx, workLensID)
	if err != nil {
		return fmt.Errorf("load work lens %d for result validation: %w", workLensID, err)
	}
	window, err := client.WorkLensWindow.Get(ctx, workLensWindowID)
	if err != nil {
		return fmt.Errorf("load work lens window %d for result validation: %w", workLensWindowID, err)
	}
	if window.WorkLensID != workLensID {
		return fmt.Errorf("work lens window %d belongs to work lens %d, not %d", workLensWindowID, window.WorkLensID, workLensID)
	}
	return ontology.ValidateLensResult(
		ontology.WorkLensKind(lens.WorkLensKind),
		ontology.LensTargetKind(lens.LensTargetKind),
		resultLensTargetKind,
		relationKind,
	)
}

// missingFieldError explains which required field was absent before validation.
func missingFieldError(tableName string, fieldName string) error {
	return fmt.Errorf("%s requires %s before ontology hook validation", tableName, fieldName)
}
