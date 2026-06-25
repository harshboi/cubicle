package graphql

import (
	"context"
	"fmt"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workResponsibilityFilters struct {
	subjectKind         *string
	subjectKey          *string
	partyKind           *string
	partyKey            *string
	responsibilityKind  *string
	responsibilityState *string
	basisKind           *string
	sourceInstance      *string
}

func (r *queryResolver) workResponsibilityModelsForFilters(ctx context.Context, limit *int, filters workResponsibilityFilters) ([]*model.WorkResponsibility, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workResponsibilities requires an Ent-backed ontology store")
	}
	sourceFilter, err := r.aggregateSourceInstance(ctx, filters.sourceInstance)
	if err != nil {
		return nil, err
	}
	if sourceFilter == nil {
		return []*model.WorkResponsibility{}, nil
	}

	rowLimit := boundedLimit(limit, 50, 200)
	query := r.EntClient.WorkResponsibility.Query().
		WithPerson().
		WithPullRequest().
		WithTicket().
		WithWorkAction(workActionDetails(sourceFilter)).
		WithWorkBlocker(func(q *genent.WorkBlockerQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithWorkProgramItem(func(q *genent.WorkProgramItemQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithWorkProgramEvidenceNeed(func(q *genent.WorkProgramEvidenceNeedQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithLatestEvidence().
		Where(
			workresponsibility.SourceSystemEQ("cubicle_analytics"),
			workresponsibility.SourceInstanceEQ(*sourceFilter),
			workresponsibility.ExternalKindEQ("tpm_work_responsibility"),
		).
		Order(
			workresponsibility.ByRankScore(entsql.OrderDesc()),
			workresponsibility.ByLastActivityAt(entsql.OrderDesc()),
			workresponsibility.ByKey(),
		).
		Limit(rowLimit)

	query, err = applyWorkResponsibilityFilters(query, filters)
	if err != nil {
		return nil, err
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	return workResponsibilityModels(rows), nil
}

func applyWorkResponsibilityFilters(query *genent.WorkResponsibilityQuery, filters workResponsibilityFilters) (*genent.WorkResponsibilityQuery, error) {
	if filters.subjectKind != nil && strings.TrimSpace(*filters.subjectKind) != "" {
		value := workresponsibility.SubjectKind(strings.TrimSpace(*filters.subjectKind))
		if err := workresponsibility.SubjectKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workresponsibility.SubjectKindEQ(value))
	}
	if filters.subjectKey != nil && strings.TrimSpace(*filters.subjectKey) != "" {
		query = query.Where(workresponsibility.SubjectKeyEQ(strings.TrimSpace(*filters.subjectKey)))
	}
	if filters.partyKind != nil && strings.TrimSpace(*filters.partyKind) != "" {
		value := workresponsibility.PartyKind(strings.TrimSpace(*filters.partyKind))
		if err := workresponsibility.PartyKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workresponsibility.PartyKindEQ(value))
	}
	if filters.partyKey != nil && strings.TrimSpace(*filters.partyKey) != "" {
		query = query.Where(workresponsibility.PartyKeyEQ(strings.TrimSpace(*filters.partyKey)))
	}
	if filters.responsibilityKind != nil && strings.TrimSpace(*filters.responsibilityKind) != "" {
		value := workresponsibility.ResponsibilityKind(strings.TrimSpace(*filters.responsibilityKind))
		if err := workresponsibility.ResponsibilityKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workresponsibility.ResponsibilityKindEQ(value))
	}
	if filters.responsibilityState != nil && strings.TrimSpace(*filters.responsibilityState) != "" {
		value := workresponsibility.ResponsibilityState(strings.TrimSpace(*filters.responsibilityState))
		if err := workresponsibility.ResponsibilityStateValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workresponsibility.ResponsibilityStateEQ(value))
	}
	if filters.basisKind != nil && strings.TrimSpace(*filters.basisKind) != "" {
		value := workresponsibility.BasisKind(strings.TrimSpace(*filters.basisKind))
		if err := workresponsibility.BasisKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workresponsibility.BasisKindEQ(value))
	}
	return query, nil
}
