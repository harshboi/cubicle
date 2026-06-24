package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workownerloadsnapshot"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) applyOwnerLoadFilters(query *genent.WorkOwnerLoadSnapshotQuery, workstreamKey *string, ownerKey *string, sourceInstance *string) *genent.WorkOwnerLoadSnapshotQuery {
	query = query.Where(
		workownerloadsnapshot.SourceSystemEQ("cubicle_analytics"),
		workownerloadsnapshot.ExternalKindEQ("tpm_owner_load_snapshot"),
	)
	if sourceInstance != nil && strings.TrimSpace(*sourceInstance) != "" {
		query = query.Where(workownerloadsnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceInstance)))
	}
	if workstreamKey != nil && *workstreamKey != "" {
		filterKey := strings.TrimSpace(*workstreamKey)
		filterKeys := []string{filterKey}
		if strings.HasPrefix(filterKey, "workstream:") {
			filterKeys = append(filterKeys, strings.TrimPrefix(filterKey, "workstream:"))
		} else {
			filterKeys = append(filterKeys, "workstream:"+filterKey)
		}
		query = query.Where(workownerloadsnapshot.Or(
			workownerloadsnapshot.WorkstreamKeyIn(filterKeys...),
			workownerloadsnapshot.HasWorkstreamWith(workstream.KeyIn(filterKeys...)),
		))
	}
	if ownerKey != nil && *ownerKey != "" {
		query = query.Where(workownerloadsnapshot.OwnerKeyIn(ownerLoadOwnerKeyFilterValues(*ownerKey)...))
	}
	return query
}

func ownerLoadRunPrefix(externalID string, ownerKey string) string {
	if externalID == "" || ownerKey == "" {
		return ""
	}
	return strings.TrimSuffix(externalID, ":"+ownerKey)
}

type ownerLoadActionGroup struct {
	sourceInstance    string
	workstreamKey     string
	generatedAt       time.Time
	createdFromRunKey string
	ownerKeys         map[string]bool
	rowIDsByOwner     map[string][]int
}

func (r *queryResolver) ownerLoadSnapshotModels(ctx context.Context, rows []*genent.WorkOwnerLoadSnapshot) ([]*model.WorkOwnerLoadSnapshot, error) {
	topActionsByOwnerLoadID, err := r.ownerLoadTopActions(ctx, rows, 3)
	if err != nil {
		return nil, err
	}
	out := make([]*model.WorkOwnerLoadSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, workOwnerLoadSnapshotModel(row, topActionsByOwnerLoadID[row.ID]))
	}
	return out, nil
}

func (r *queryResolver) ownerLoadTopActions(ctx context.Context, rows []*genent.WorkOwnerLoadSnapshot, perOwnerLimit int) (map[int][]*genent.WorkAction, error) {
	out := map[int][]*genent.WorkAction{}
	if len(rows) == 0 || perOwnerLimit <= 0 {
		return out, nil
	}
	groups := map[string]*ownerLoadActionGroup{}
	for _, row := range rows {
		if row == nil || row.OwnerKey == "(clear)" {
			continue
		}
		actionOwnerKey := ownerLoadWorkActionOwnerKey(row.OwnerKey)
		createdFromRunKey := ownerLoadCreatedFromRunKey(row)
		groupKey := row.SourceInstance + "\x00" + row.WorkstreamKey + "\x00" + createdFromRunKey + "\x00" + row.GeneratedAt.UTC().Format(time.RFC3339Nano)
		group := groups[groupKey]
		if group == nil {
			group = &ownerLoadActionGroup{
				sourceInstance:    row.SourceInstance,
				workstreamKey:     row.WorkstreamKey,
				generatedAt:       row.GeneratedAt,
				createdFromRunKey: createdFromRunKey,
				ownerKeys:         map[string]bool{},
				rowIDsByOwner:     map[string][]int{},
			}
			groups[groupKey] = group
		}
		group.ownerKeys[actionOwnerKey] = true
		group.rowIDsByOwner[actionOwnerKey] = append(group.rowIDsByOwner[actionOwnerKey], row.ID)
	}
	for _, group := range groups {
		for ownerKey := range group.ownerKeys {
			actions, err := r.ownerLoadTopActionsForOwner(ctx, group, ownerKey, perOwnerLimit)
			if err != nil {
				return nil, err
			}
			for _, rowID := range group.rowIDsByOwner[ownerKey] {
				out[rowID] = append(out[rowID], actions...)
			}
		}
	}
	return out, nil
}

func (r *queryResolver) ownerLoadTopActionsForOwner(ctx context.Context, group *ownerLoadActionGroup, ownerKey string, perOwnerLimit int) ([]*genent.WorkAction, error) {
	if group == nil || perOwnerLimit <= 0 {
		return nil, nil
	}
	sourceInstance := group.sourceInstance
	query := r.EntClient.WorkAction.Query().
		WithPullRequest(func(q *genent.PullRequestQuery) {
			q.WithTickets()
		}).
		WithTicket(func(q *genent.TicketQuery) {
			q.WithPullRequests()
		}).
		WithLatestEvidence().
		WithObservations(func(q *genent.WorkActionObservationQuery) {
			q.Where(
				workactionobservation.SourceSystemEQ("cubicle_analytics"),
				workactionobservation.SourceInstanceEQ(sourceInstance),
				workactionobservation.ExternalKindEQ("tpm_work_action_observation"),
			)
			q.WithLatestEvidence()
			q.Order(workactionobservation.ByObservedAt(entsql.OrderDesc()))
		}).
		WithSourceInsights(func(q *genent.WorkInsightQuery) {
			q.Where(
				workinsight.SourceSystemEQ("cubicle_analytics"),
				workinsight.SourceInstanceEQ(sourceInstance),
				workinsight.ExternalKindEQ("tpm_insight"),
			)
			q.WithLatestEvidence()
			q.WithReviews(func(rq *genent.WorkInsightReviewQuery) {
				rq = applyWorkInsightReviewSourceFilter(rq, &sourceInstance)
				rq.Order(
					workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
					workinsightreview.ByReviewedAt(entsql.OrderDesc()),
					workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
				)
			})
			q.Order(workinsight.ByRankScore(entsql.OrderDesc()))
		}).
		Where(
			workaction.SourceSystemEQ("cubicle_analytics"),
			workaction.SourceInstanceEQ(group.sourceInstance),
			workaction.ExternalKindEQ("tpm_work_action"),
			workaction.ActionStateEQ(workaction.ActionStateOpen),
			workaction.OwnerKeyEQ(ownerKey),
		).
		Order(
			workaction.ByRankScore(entsql.OrderDesc()),
			workaction.ByLastActivityAt(entsql.OrderDesc()),
		).
		Limit(perOwnerLimit)
	if group.createdFromRunKey != "" {
		query = query.Where(workaction.CreatedFromRunKeyEQ(group.createdFromRunKey))
	} else if !group.generatedAt.IsZero() {
		query = query.Where(workaction.OpenedAtEQ(group.generatedAt))
	}
	if group.workstreamKey != "" {
		query = query.Where(workaction.SubjectKeyHasPrefix(ownerLoadWorkstreamSubjectPrefix(group.workstreamKey)))
	}
	actions, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.hydrateWorkActionSourceInsightDetails(ctx, actions, &sourceInstance); err != nil {
		return nil, err
	}
	sortWorkActions(actions)
	return actions, nil
}

func ownerLoadWorkActionOwnerKey(ownerKey string) string {
	if ownerKey == "(unassigned)" {
		return ""
	}
	return ownerKey
}

func ownerLoadDisplayOwnerKey(ownerKey string) string {
	if ownerKey == "" {
		return "(unassigned)"
	}
	return ownerKey
}

func ownerLoadOwnerKeyFilterValues(ownerKey string) []string {
	key := strings.TrimSpace(ownerKey)
	if key == "unassigned" {
		return []string{"(unassigned)"}
	}
	return []string{key}
}

func ownerLoadWorkstreamSubjectPrefix(workstreamKey string) string {
	key := strings.TrimPrefix(strings.TrimSpace(workstreamKey), "workstream:")
	switch key {
	case "flink-kubernetes-operator":
		return "apache/flink-kubernetes-operator#"
	default:
		return key
	}
}

func ownerLoadCreatedFromRunKey(row *genent.WorkOwnerLoadSnapshot) string {
	if row == nil {
		return ""
	}
	runAt := ownerLoadRunGeneratedAt(row.ExternalID, row.WorkstreamKey, row.OwnerKey)
	if runAt == "" {
		return ""
	}
	return "tpm-action-brief:" + runAt
}

func ownerLoadRunGeneratedAt(externalID string, workstreamKey string, ownerKey string) string {
	prefix := ownerLoadRunPrefix(externalID, ownerKey)
	if prefix == "" || workstreamKey == "" {
		return ""
	}
	for _, candidate := range []string{workstreamKey, strings.TrimPrefix(workstreamKey, "workstream:")} {
		if candidate != "" && strings.HasPrefix(prefix, candidate+":") {
			return strings.TrimPrefix(prefix, candidate+":")
		}
	}
	return ""
}
