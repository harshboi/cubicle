package graphql

import (
	"context"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func (r *queryResolver) workstreamRegisterModels(ctx context.Context, actionState string, streams []*genent.Workstream, actions []*genent.WorkAction, forecastRows []*genent.WorkForecastEvaluation) ([]*model.WorkstreamRegister, error) {
	out := make([]*model.WorkstreamRegister, 0, len(streams))
	for _, stream := range streams {
		ticketIDs := ticketIDSet(stream)
		pullRequestIDs, err := r.pullRequestIDsForTickets(ctx, ticketIDs)
		if err != nil {
			return nil, err
		}
		streamActions := actionsForWorkstream(stream, actions, ticketIDs, pullRequestIDs)
		streamForecastRows := forecastRows
		if stream.SourceInstance != "" {
			sourceInstance := stream.SourceInstance
			streamForecastRows, err = r.workForecastEvaluationRows(ctx, &sourceInstance)
			if err != nil {
				return nil, err
			}
		}
		summary := workActionSummaryModel(actionState, optionalString(stream.SourceInstance), streamActions, streamForecastRows...)
		out = append(out, workstreamRegisterModel(stream, summary))
	}
	return out, nil
}

func ticketIDSet(stream *genent.Workstream) map[int]bool {
	out := map[int]bool{}
	for _, ticket := range stream.Edges.Tickets {
		out[ticket.ID] = true
	}
	return out
}

func (r *queryResolver) pullRequestIDsForTickets(ctx context.Context, ticketIDs map[int]bool) (map[int]bool, error) {
	out := map[int]bool{}
	if len(ticketIDs) == 0 {
		return out, nil
	}
	ids := make([]int, 0, len(ticketIDs))
	for id := range ticketIDs {
		ids = append(ids, id)
	}
	edges, err := r.EntClient.TicketPullRequest.Query().
		Where(ticketpullrequest.TicketIDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, edge := range edges {
		if edge.PullRequestID != 0 {
			out[edge.PullRequestID] = true
		}
	}
	return out, nil
}

func actionsForWorkstream(stream *genent.Workstream, actions []*genent.WorkAction, ticketIDs map[int]bool, pullRequestIDs map[int]bool) []*genent.WorkAction {
	out := make([]*genent.WorkAction, 0, len(actions))
	for _, action := range actions {
		if action.TicketID != 0 && ticketIDs[action.TicketID] {
			out = append(out, action)
			continue
		}
		if action.PullRequestID != 0 && pullRequestIDs[action.PullRequestID] {
			out = append(out, action)
			continue
		}
		if stream.SourceInstance != "" && action.SourceInstance == stream.SourceInstance {
			out = append(out, action)
		}
	}
	sortStandupActions(out)
	return out
}

func workstreamRegisterModel(stream *genent.Workstream, summary *model.WorkActionSummary) *model.WorkstreamRegister {
	topActions := summary.TopActions
	if len(topActions) > 5 {
		topActions = topActions[:5]
	}
	topRiskScore := stream.RankScore
	for _, action := range topActions {
		if action.RankScore > topRiskScore {
			topRiskScore = action.RankScore
		}
	}
	return &model.WorkstreamRegister{
		Key:                 stream.Key,
		SourceInstance:      optionalString(stream.SourceInstance),
		Title:               stream.Title,
		Status:              stream.Status.String(),
		Summary:             optionalString(stream.Summary),
		SourceURL:           optionalString(stream.SourceURL),
		TicketCount:         len(stream.Edges.Tickets),
		ActionItemCount:     summary.TotalCount,
		ProductActionCount:  summary.ProductActionCount,
		ValidationLeadCount: summary.ValidationLeadCount,
		CloseoutReviewCount: summary.CloseoutReviewCount,
		ModelOrRuleQaCount:  summary.ModelOrRuleQaCount,
		NowCount:            summary.NowCount,
		TopRiskScore:        topRiskScore,
		ForecastReadiness:   summary.ForecastReadiness,
		Badges:              workstreamRegisterBadges(stream, summary),
		TopActions:          topActions,
	}
}

func workstreamRegisterBadges(stream *genent.Workstream, summary *model.WorkActionSummary) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		countBadge("workstream:tickets", "Tickets", "info", len(stream.Edges.Tickets)),
	}
	if summary.ProductActionCount > 0 {
		badges = append(badges, countBadge("workstream:product_actions", "Product actions", "success", summary.ProductActionCount))
	}
	if summary.ValidationLeadCount > 0 {
		badges = append(badges, countBadge("workstream:validation_leads", "Validation leads", "warning", summary.ValidationLeadCount))
	}
	if summary.ForecastReadiness != nil && !summary.ForecastReadiness.EtaForecastReady {
		badges = append(badges, &model.WorkActionBadge{Key: "workstream:forecast_gated", Label: "Forecast gated", Tone: "warning"})
	}
	return badges
}
