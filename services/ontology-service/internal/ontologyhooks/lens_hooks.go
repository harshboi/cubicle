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

// missingFieldError explains which required field was absent before validation.
func missingFieldError(tableName string, fieldName string) error {
	return fmt.Errorf("%s requires %s before ontology hook validation", tableName, fieldName)
}
