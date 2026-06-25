package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramOwnerResponsibilitySummary struct {
	total      int
	active     int
	candidate  int
	unassigned int
	rows       []*model.WorkResponsibility
}

func (r *queryResolver) workProgramOwnerResponsibilitySummary(ctx context.Context, sourceFilter *string, workstreamKey string, ownerKey string, limit int) (workProgramOwnerResponsibilitySummary, error) {
	var out workProgramOwnerResponsibilitySummary
	if r.EntClient == nil || limit <= 0 {
		return out, nil
	}
	predicates := workProgramOwnerResponsibilityPredicates(sourceFilter, workstreamKey, ownerKey)
	if len(predicates) == 0 {
		return out, nil
	}
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
		Where(predicates...).
		Order(
			workresponsibility.ByResponsibilityState(entsql.OrderAsc()),
			workresponsibility.ByRankScore(entsql.OrderDesc()),
			workresponsibility.ByLastActivityAt(entsql.OrderDesc()),
			workresponsibility.ByKey(),
		).
		Limit(limit)

	rows, err := query.All(ctx)
	if err != nil {
		return out, err
	}
	models := workResponsibilityModels(rows)
	out.rows = models
	out.total = len(models)
	for _, row := range models {
		if row == nil {
			continue
		}
		switch row.ResponsibilityState {
		case "active":
			out.active++
		case "candidate":
			out.candidate++
		}
		if row.PartyKind == "unassigned" {
			out.unassigned++
		}
	}
	return out, nil
}

func workProgramOwnerResponsibilityPredicates(sourceFilter *string, workstreamKey string, ownerKey string) []predicate.WorkResponsibility {
	partyKeys := workProgramOwnerResponsibilityPartyKeys(ownerKey)
	if len(partyKeys) == 0 {
		return nil
	}
	predicates := []predicate.WorkResponsibility{
		workresponsibility.SourceSystemEQ("cubicle_analytics"),
		workresponsibility.ExternalKindEQ("tpm_work_responsibility"),
		workresponsibility.PartyKeyIn(partyKeys...),
		workresponsibility.ResponsibilityStateIn(
			workresponsibility.ResponsibilityStateActive,
			workresponsibility.ResponsibilityStateCandidate,
		),
	}
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		predicates = append(predicates, workresponsibility.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	scope := workProgramOwnerResponsibilityScopePredicate(workstreamKey)
	if scope != nil {
		predicates = append(predicates, scope)
	}
	return predicates
}

func workProgramOwnerResponsibilityScopePredicate(workstreamKey string) predicate.WorkResponsibility {
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil
	}
	filterKeys := workProgramWorkstreamFilterKeys(workstreamKey)
	prefix := ownerLoadWorkstreamSubjectPrefix(workstreamKey)
	scopes := []predicate.WorkResponsibility{
		workresponsibility.WorkstreamKeyIn(filterKeys...),
	}
	if prefix != "" {
		scopes = append(scopes,
			workresponsibility.SubjectKeyHasPrefix(prefix),
			workresponsibility.HasWorkActionWith(workaction.SubjectKeyHasPrefix(prefix)),
			workresponsibility.HasWorkBlockerWith(workblocker.SubjectKeyHasPrefix(prefix)),
		)
	}
	return workresponsibility.Or(scopes...)
}

func workProgramOwnerResponsibilityPartyKeys(ownerKey string) []string {
	key := strings.TrimSpace(ownerKey)
	if key == "" {
		return nil
	}
	if key == "unassigned" || key == "(unassigned)" {
		return []string{"unassigned", "(unassigned)"}
	}
	keys := []string{key}
	if strings.HasPrefix(key, "github:") {
		keys = append(keys, strings.TrimPrefix(key, "github:"))
	} else if !strings.Contains(key, ":") && !strings.Contains(key, "@") {
		keys = append(keys, "github:"+key)
	}
	return uniqueOwnerResponsibilityKeys(keys)
}

func uniqueOwnerResponsibilityKeys(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
