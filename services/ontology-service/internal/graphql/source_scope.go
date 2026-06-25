package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workdecisiontargetevaluation"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/ent/workitemstatesnapshot"
	"cubicle/services/ontology-service/ent/workitemstatetransition"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/ent/workstreamhealthsnapshot"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) aggregateSourceInstance(ctx context.Context, sourceInstance *string) (*string, error) {
	sourceFilter, err := optionalSourceInstanceArgument(sourceInstance, "sourceInstance")
	if err != nil {
		return nil, err
	}
	if sourceFilter != nil {
		return sourceFilter, nil
	}
	row, err := r.EntClient.WorkForecastEvaluation.Query().
		Where(
			workforecastevaluation.SourceSystemEQ("cubicle_analytics"),
			workforecastevaluation.ExternalKindEQ("tpm_forecast_evaluation"),
			workforecastevaluation.EvaluationKindEQ(workforecastevaluation.EvaluationKindSummary),
		).
		Order(
			workforecastevaluation.ByEvaluatedAt(entsql.OrderDesc()),
			workforecastevaluation.ByUpdatedAt(entsql.OrderDesc()),
		).
		First(ctx)
	if err == nil {
		if source := strings.TrimSpace(row.SourceInstance); source != "" {
			return &source, nil
		}
	} else if !genent.IsNotFound(err) {
		return nil, err
	}
	action, err := r.EntClient.WorkAction.Query().
		Where(
			workaction.SourceSystemEQ("cubicle_analytics"),
			workaction.ExternalKindEQ("tpm_work_action"),
		).
		Order(
			workaction.ByUpdatedAt(entsql.OrderDesc()),
			workaction.ByLastActivityAt(entsql.OrderDesc()),
		).
		First(ctx)
	if err == nil {
		if source := strings.TrimSpace(action.SourceInstance); source != "" {
			return &source, nil
		}
		return nil, nil
	}
	if genent.IsNotFound(err) {
		programItem, err := r.EntClient.WorkProgramItem.Query().
			Where(
				workprogramitem.SourceSystemEQ("cubicle_analytics"),
				workprogramitem.ExternalKindEQ("tpm_program_item"),
			).
			Order(
				workprogramitem.ByUpdatedAt(entsql.OrderDesc()),
				workprogramitem.ByLastActivityAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(programItem.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		blocker, err := r.EntClient.WorkBlocker.Query().
			Where(
				workblocker.SourceSystemEQ("cubicle_analytics"),
				workblocker.ExternalKindEQ("tpm_work_blocker"),
			).
			Order(
				workblocker.ByUpdatedAt(entsql.OrderDesc()),
				workblocker.ByLastActivityAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(blocker.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		dependency, err := r.EntClient.WorkDependencyEdge.Query().
			Where(
				workdependencyedge.SourceSystemEQ("cubicle_analytics"),
				workdependencyedge.ExternalKindEQ("tpm_work_dependency_edge"),
			).
			Order(
				workdependencyedge.ByUpdatedAt(entsql.OrderDesc()),
				workdependencyedge.ByLastActivityAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(dependency.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		impact, err := r.EntClient.WorkBlockerImpact.Query().
			Where(
				workblockerimpact.SourceSystemEQ("cubicle_analytics"),
				workblockerimpact.ExternalKindEQ("tpm_work_blocker_impact"),
			).
			Order(
				workblockerimpact.ByUpdatedAt(entsql.OrderDesc()),
				workblockerimpact.ByLastActivityAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(impact.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		snapshot, err := r.EntClient.WorkItemStateSnapshot.Query().
			Where(
				workitemstatesnapshot.SourceSystemEQ("cubicle_analytics"),
				workitemstatesnapshot.ExternalKindIn("tpm_pr_state_snapshot", "tpm_ticket_state_snapshot"),
			).
			Order(
				workitemstatesnapshot.ByObservedAt(entsql.OrderDesc()),
				workitemstatesnapshot.ByUpdatedAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(snapshot.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		transition, err := r.EntClient.WorkItemStateTransition.Query().
			Where(
				workitemstatetransition.SourceSystemEQ("cubicle_analytics"),
				workitemstatetransition.ExternalKindEQ("tpm_state_transition_candidate"),
			).
			Order(
				workitemstatetransition.ByToObservedAt(entsql.OrderDesc()),
				workitemstatetransition.ByUpdatedAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(transition.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		health, err := r.EntClient.WorkstreamHealthSnapshot.Query().
			Where(
				workstreamhealthsnapshot.SourceSystemEQ("cubicle_analytics"),
				workstreamhealthsnapshot.ExternalKindEQ("tpm_workstream_health_snapshot"),
			).
			Order(
				workstreamhealthsnapshot.ByGeneratedAt(entsql.OrderDesc()),
				workstreamhealthsnapshot.ByUpdatedAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(health.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		decisionTarget, err := r.EntClient.WorkDecisionTargetEvaluation.Query().
			Where(
				workdecisiontargetevaluation.SourceSystemEQ("cubicle_analytics"),
				workdecisiontargetevaluation.ExternalKindEQ("tpm_decision_target_evaluation"),
			).
			Order(
				workdecisiontargetevaluation.ByEvaluatedAt(entsql.OrderDesc()),
				workdecisiontargetevaluation.ByUpdatedAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(decisionTarget.SourceInstance); source != "" {
				return &source, nil
			}
		} else if !genent.IsNotFound(err) {
			return nil, err
		}
		stream, err := r.EntClient.Workstream.Query().
			Where(
				workstream.SourceSystemEQ("cubicle_analytics"),
				workstream.ExternalKindEQ("tpm_workstream"),
			).
			Order(
				workstream.ByUpdatedAt(entsql.OrderDesc()),
				workstream.ByLastActivityAt(entsql.OrderDesc()),
			).
			First(ctx)
		if err == nil {
			if source := strings.TrimSpace(stream.SourceInstance); source != "" {
				return &source, nil
			}
			return nil, nil
		}
		if genent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return nil, err
}
