package graphql

import (
	"context"
	"fmt"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workprogrambriefsnapshot"
	"cubicle/services/ontology-service/ent/workprogramrunmember"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

const workProgramGraphBriefBaseModelMethod = "bounded_graph_context_to_cited_brief"

func (r *queryResolver) workProgramGraphBrief(ctx context.Context, workstreamKey string, sourceInstance *string, promptMode *string) (*model.WorkProgramGraphBrief, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramGraphBrief requires an Ent-backed ontology store")
	}
	if strings.TrimSpace(workstreamKey) == "" {
		return nil, fmt.Errorf("workstreamKey cannot be blank")
	}
	sourceFilter, err := optionalSourceInstanceArgument(sourceInstance, "sourceInstance")
	if err != nil {
		return nil, err
	}
	mode, err := workProgramGraphBriefPromptMode(promptMode)
	if err != nil {
		return nil, err
	}
	insight, err := r.latestWorkProgramGraphBriefInsight(ctx, workstreamKey, sourceFilter, mode)
	if err != nil || insight == nil {
		return nil, err
	}
	if sourceFilter == nil && strings.TrimSpace(insight.SourceInstance) != "" {
		source := strings.TrimSpace(insight.SourceInstance)
		sourceFilter = &source
	}
	snapshot, err := r.latestWorkProgramGraphBriefSnapshot(ctx, workstreamKey, sourceFilter, mode)
	if err != nil {
		return nil, err
	}
	runKey, err := r.workProgramGraphBriefRunKey(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return workProgramGraphBriefModel(insight, snapshot, runKey), nil
}

func (r *queryResolver) latestWorkProgramGraphBriefInsight(ctx context.Context, workstreamKey string, sourceFilter *string, promptMode string) (*genent.WorkInsight, error) {
	query := r.EntClient.WorkInsight.Query().
		Where(
			workinsight.SourceSystemEQ("cubicle_ai"),
			workinsight.ExternalKindEQ("ai_graph_brief"),
			workinsight.InsightKindEQ(workinsight.InsightKindAiGraphBrief),
			workinsight.ProducerStateEQ(workinsight.ProducerStateCurrent),
			workinsight.SubjectKeyIn(workProgramWorkstreamFilterKeys(workstreamKey)...),
			workProgramGraphBriefInsightPromptModePredicate(promptMode),
		).
		WithLatestEvidence().
		Order(
			workinsight.ByLastActivityAt(entsql.OrderDesc()),
			workinsight.ByRankScore(entsql.OrderDesc()),
			workinsight.ByUpdatedAt(entsql.OrderDesc()),
		)
	if sourceFilter != nil {
		query = query.Where(workinsight.SourceInstanceEQ(*sourceFilter))
	}
	row, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *queryResolver) latestWorkProgramGraphBriefSnapshot(ctx context.Context, workstreamKey string, sourceFilter *string, promptMode string) (*genent.WorkProgramBriefSnapshot, error) {
	query := r.EntClient.WorkProgramBriefSnapshot.Query().
		Where(
			workprogrambriefsnapshot.SourceSystemEQ("cubicle_ai"),
			workprogrambriefsnapshot.ExternalKindEQ("ai_graph_brief_snapshot"),
			workprogrambriefsnapshot.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(workstreamKey)...),
			workProgramGraphBriefSnapshotPromptModePredicate(promptMode),
		).
		WithLatestEvidence().
		Order(
			workprogrambriefsnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workprogrambriefsnapshot.ByRankScore(entsql.OrderDesc()),
			workprogrambriefsnapshot.ByUpdatedAt(entsql.OrderDesc()),
		)
	if sourceFilter != nil {
		query = query.Where(workprogrambriefsnapshot.SourceInstanceEQ(*sourceFilter))
	}
	row, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func workProgramGraphBriefPromptMode(value *string) (string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "operating", nil
	}
	mode := strings.ToLower(strings.TrimSpace(*value))
	switch mode {
	case "operating", "generic":
		return mode, nil
	default:
		return "", fmt.Errorf("promptMode must be operating or generic")
	}
}

func workProgramGraphBriefModelMethod(promptMode string) string {
	return workProgramGraphBriefBaseModelMethod + ":" + promptMode
}

func workProgramGraphBriefInsightPromptModePredicate(promptMode string) predicate.WorkInsight {
	if promptMode == "generic" {
		return workinsight.Or(
			workinsight.ModelMethodEQ(workProgramGraphBriefModelMethod(promptMode)),
			workinsight.ExternalIDContains("|generic|"),
			workinsight.SourceURLContains("/generic/"),
		)
	}
	return workinsight.Or(
		workinsight.ModelMethodEQ(workProgramGraphBriefModelMethod("operating")),
		workinsight.ExternalIDContains("|operating|"),
		workinsight.SourceURLContains("/operating/"),
		workinsight.And(
			workinsight.Or(
				workinsight.ModelMethodIsNil(),
				workinsight.ModelMethodEQ(""),
				workinsight.ModelMethodEQ(workProgramGraphBriefBaseModelMethod),
			),
			workinsight.Not(workinsight.ExternalIDContains("|generic|")),
			workinsight.Not(workinsight.SourceURLContains("/generic/")),
		),
	)
}

func workProgramGraphBriefSnapshotPromptModePredicate(promptMode string) predicate.WorkProgramBriefSnapshot {
	if promptMode == "generic" {
		return workprogrambriefsnapshot.Or(
			workprogrambriefsnapshot.ExternalIDContains("|generic|"),
			workprogrambriefsnapshot.SourceURLContains("/generic/"),
		)
	}
	return workprogrambriefsnapshot.Or(
		workprogrambriefsnapshot.ExternalIDContains("|operating|"),
		workprogrambriefsnapshot.SourceURLContains("/operating/"),
		workprogrambriefsnapshot.And(
			workprogrambriefsnapshot.Not(workprogrambriefsnapshot.ExternalIDContains("|generic|")),
			workprogrambriefsnapshot.Not(workprogrambriefsnapshot.SourceURLContains("/generic/")),
		),
	)
}

func (r *queryResolver) workProgramGraphBriefRunKey(ctx context.Context, snapshot *genent.WorkProgramBriefSnapshot) (*string, error) {
	if snapshot == nil {
		return nil, nil
	}
	member, err := r.EntClient.WorkProgramRunMember.Query().
		Where(
			workprogramrunmember.MemberTableEQ(workProgramRunMemberTableBriefSnapshots),
			workprogramrunmember.MemberIDEQ(snapshot.ID),
		).
		Order(workprogramrunmember.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return optionalString(member.RunKey), nil
}

func workProgramGraphBriefModel(insight *genent.WorkInsight, snapshot *genent.WorkProgramBriefSnapshot, runKey *string) *model.WorkProgramGraphBrief {
	sourceInstance := optionalString(insight.SourceInstance)
	generatedAt := optionalTime(insight.LastActivityAt)
	snapshotKey := (*string)(nil)
	if snapshot != nil {
		snapshotKey = optionalString(snapshot.Key)
		if snapshot.GeneratedAt.IsZero() {
			if generatedAt == nil {
				generatedAt = optionalTime(snapshot.UpdatedAt)
			}
		} else {
			generatedAt = optionalTime(snapshot.GeneratedAt)
		}
	}
	evidence := insight.Edges.LatestEvidence
	contextHash := workProgramGraphBriefContextHash(insight, evidence)
	if contextHash == "" && evidence != nil {
		contextHash = strings.TrimPrefix(strings.TrimSpace(evidence.Locator), "context_hash:")
	}
	return &model.WorkProgramGraphBrief{
		SourceInstance:    sourceInstance,
		WorkstreamKey:     insight.SubjectKey,
		BriefMode:         workProgramGraphBriefMode(insight),
		GeneratedAt:       generatedAt,
		ContextHash:       contextHash,
		ModelName:         optionalString(insight.ModelName),
		ModelVersion:      optionalString(insight.ModelVersion),
		ModelMethod:       optionalString(insight.ModelMethod),
		EvaluationSummary: optionalString(insight.ScoreExplanation),
		BriefMarkdown:     workProgramGraphBriefMarkdown(insight.Details),
		SnapshotKey:       snapshotKey,
		RunKey:            runKey,
		Insight:           workInsightSummaryModel(insight),
		Evidence:          workEvidenceSummary(evidence),
		Badges:            workProgramGraphBriefBadges(insight, snapshot, runKey),
	}
}

func workProgramGraphBriefMode(insight *genent.WorkInsight) string {
	if insight == nil {
		return "operating"
	}
	modelMethod := strings.ToLower(strings.TrimSpace(insight.ModelMethod))
	switch {
	case strings.HasSuffix(modelMethod, ":generic"):
		return "generic"
	case strings.HasSuffix(modelMethod, ":operating"):
		return "operating"
	case strings.Contains(strings.ToLower(insight.ExternalID), "|generic|"):
		return "generic"
	case strings.Contains(strings.ToLower(insight.SourceURL), "/generic/"):
		return "generic"
	default:
		return "operating"
	}
}

func workProgramGraphBriefContextHash(insight *genent.WorkInsight, evidence *genent.Evidence) string {
	if insight == nil {
		return ""
	}
	contextHash := strings.TrimSpace(insight.SourceVersion)
	if contextHash != "" {
		return contextHash
	}
	sourceURL := strings.TrimPrefix(strings.TrimSpace(insight.SourceURL), "cubicle://graph-brief/")
	if sourceURL != "" {
		parts := strings.Split(strings.Trim(sourceURL, "/"), "/")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	if evidence != nil {
		return strings.TrimPrefix(strings.TrimSpace(evidence.Locator), "context_hash:")
	}
	return ""
}

func workProgramGraphBriefMarkdown(details string) string {
	marker := "\n\nEvaluation:"
	if idx := strings.Index(details, marker); idx >= 0 {
		return strings.TrimSpace(details[:idx])
	}
	return strings.TrimSpace(details)
}

func workProgramGraphBriefBadges(insight *genent.WorkInsight, snapshot *genent.WorkProgramBriefSnapshot, runKey *string) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "graph_brief:generated", Label: "Generated graph brief", Tone: "info"},
	}
	if insight != nil && insight.Score >= 100 {
		badges = append(badges, &model.WorkActionBadge{Key: "graph_brief:eval_passed", Label: "Eval passed", Tone: "success", Detail: optionalString(insight.ScoreExplanation)})
	}
	if snapshot != nil {
		badges = append(badges, &model.WorkActionBadge{Key: "graph_brief:snapshot", Label: "Run snapshot", Tone: "info", Detail: optionalString(snapshot.ExternalKind)})
	}
	if runKey != nil && strings.TrimSpace(*runKey) != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "graph_brief:run_bound", Label: "Run bound", Tone: "success", Detail: runKey})
	}
	return badges
}
