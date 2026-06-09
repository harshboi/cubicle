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
// Ent schema hooks can validate same-row fields, but pane link validation needs
// to load the parent WorkPane. Keep cross-row checks in this generated-client
// hook layer and call Register for every Ent client used by ontology writers.
func Register(client *genent.Client) {
	if client == nil {
		return
	}
	client.WorkPane.Use(enthook.On(validateWorkPanePlacement(), genent.OpCreate))
	client.PaneDocumentLink.Use(enthook.On(validatePaneDocumentLink(), genent.OpCreate))
	client.PanePullRequestLink.Use(enthook.On(validatePanePullRequestLink(), genent.OpCreate))
	client.PaneTicketLink.Use(enthook.On(validatePaneTicketLink(), genent.OpCreate))
	client.PaneMessageLink.Use(enthook.On(validatePaneMessageLink(), genent.OpCreate))
}

func validateWorkPanePlacement() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.WorkPaneFunc(func(ctx context.Context, mutation *genent.WorkPaneMutation) (genent.Value, error) {
			if err := validateWorkPaneMutation(ctx, mutation); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func validatePaneDocumentLink() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.PaneDocumentLinkFunc(func(ctx context.Context, mutation *genent.PaneDocumentLinkMutation) (genent.Value, error) {
			paneID, ok := mutation.PaneID()
			if !ok {
				return nil, missingFieldError("pane_document_links", "pane_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("pane_document_links", "relation_kind")
			}
			if err := validatePaneLink(ctx, mutation.Client(), paneID, ontology.TargetDocument, ontology.RelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func validatePanePullRequestLink() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.PanePullRequestLinkFunc(func(ctx context.Context, mutation *genent.PanePullRequestLinkMutation) (genent.Value, error) {
			paneID, ok := mutation.PaneID()
			if !ok {
				return nil, missingFieldError("pane_pull_request_links", "pane_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("pane_pull_request_links", "relation_kind")
			}
			if err := validatePaneLink(ctx, mutation.Client(), paneID, ontology.TargetPullRequest, ontology.RelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func validatePaneTicketLink() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.PaneTicketLinkFunc(func(ctx context.Context, mutation *genent.PaneTicketLinkMutation) (genent.Value, error) {
			paneID, ok := mutation.PaneID()
			if !ok {
				return nil, missingFieldError("pane_ticket_links", "pane_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("pane_ticket_links", "relation_kind")
			}
			if err := validatePaneLink(ctx, mutation.Client(), paneID, ontology.TargetTicket, ontology.RelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func validatePaneMessageLink() genent.Hook {
	return func(next genent.Mutator) genent.Mutator {
		return enthook.PaneMessageLinkFunc(func(ctx context.Context, mutation *genent.PaneMessageLinkMutation) (genent.Value, error) {
			paneID, ok := mutation.PaneID()
			if !ok {
				return nil, missingFieldError("pane_message_links", "pane_id")
			}
			relationKind, ok := mutation.RelationKind()
			if !ok {
				return nil, missingFieldError("pane_message_links", "relation_kind")
			}
			if err := validatePaneLink(ctx, mutation.Client(), paneID, ontology.TargetMessage, ontology.RelationKind(relationKind)); err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func validateWorkPaneMutation(ctx context.Context, mutation *genent.WorkPaneMutation) error {
	surfaceID, ok := mutation.WorkSurfaceID()
	if !ok {
		return missingFieldError("work_panes", "work_surface_id")
	}
	paneKind, ok := mutation.PaneKind()
	if !ok {
		return missingFieldError("work_panes", "pane_kind")
	}
	targetKind, ok := mutation.TargetKind()
	if !ok {
		return missingFieldError("work_panes", "target_kind")
	}
	surface, err := mutation.Client().WorkSurface.Get(ctx, surfaceID)
	if err != nil {
		return fmt.Errorf("load work surface %d for pane validation: %w", surfaceID, err)
	}
	return ontology.ValidatePanePlacement(
		ontology.PaneKind(paneKind),
		ontology.SurfaceKind(surface.SurfaceKind),
		ontology.TargetKind(targetKind),
	)
}

func validatePaneLink(ctx context.Context, client *genent.Client, paneID int, linkTargetKind ontology.TargetKind, relationKind ontology.RelationKind) error {
	pane, err := client.WorkPane.Get(ctx, paneID)
	if err != nil {
		return fmt.Errorf("load work pane %d for link validation: %w", paneID, err)
	}
	return ontology.ValidatePaneLink(
		ontology.PaneKind(pane.PaneKind),
		ontology.TargetKind(pane.TargetKind),
		linkTargetKind,
		relationKind,
	)
}

func missingFieldError(tableName string, fieldName string) error {
	return fmt.Errorf("%s requires %s before ontology hook validation", tableName, fieldName)
}
