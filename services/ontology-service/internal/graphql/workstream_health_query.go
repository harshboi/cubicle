package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/ent/workstreamhealthsnapshot"

	entsql "entgo.io/ent/dialect/sql"
)

type workstreamHealthCheckCounts struct {
	failingCheckPullRequestCount     int
	openFailingCheckPullRequestCount int
}

func (r *queryResolver) latestWorkstreamHealthCheckCounts(ctx context.Context, sourceFilter *string, workstreamKey *string) (workstreamHealthCheckCounts, error) {
	query := r.EntClient.WorkstreamHealthSnapshot.Query().
		Where(
			workstreamhealthsnapshot.SourceSystemEQ("cubicle_analytics"),
			workstreamhealthsnapshot.ExternalKindEQ("tpm_workstream_health_snapshot"),
		).
		Order(
			workstreamhealthsnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workstreamhealthsnapshot.ByUpdatedAt(entsql.OrderDesc()),
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workstreamhealthsnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		filterKeys := workProgramWorkstreamFilterKeys(*workstreamKey)
		query = query.Where(workstreamhealthsnapshot.Or(
			workstreamhealthsnapshot.WorkstreamKeyIn(filterKeys...),
			workstreamhealthsnapshot.HasWorkstreamWith(workstream.KeyIn(filterKeys...)),
		))
	}
	row, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return workstreamHealthCheckCounts{}, nil
	}
	if err != nil {
		return workstreamHealthCheckCounts{}, err
	}
	return workstreamHealthCheckCounts{
		failingCheckPullRequestCount:     row.FailingCheckPrCount,
		openFailingCheckPullRequestCount: row.OpenFailingCheckPrCount,
	}, nil
}
