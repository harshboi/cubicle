package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogrammilestone"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramMilestonePacketSeparatesReleaseTargetsFromCommitments(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 23, 11, 13, 38, 0, time.UTC)
	targetDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dueDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	outcomeDate := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)

	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)
	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:FLINK-1").
		SetTitle("Fix release target").
		SetStatus("open").
		SetSourceSystem("jira").
		SetSourceInstance(source).
		SetExternalKind("jira_issue").
		SetExternalID("FLINK-1").
		SetSourceURL("https://issues.apache.org/jira/browse/FLINK-1").
		SaveX(ctx)

	releaseTarget := store.Client().WorkProgramMilestone.Create().
		SetKey("work-program-milestone:release-target").
		SetTicketID(ticketRow.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogrammilestone.SubjectKindTicket).
		SetSubjectKey("FLINK-1").
		SetMilestoneKind(workprogrammilestone.MilestoneKindReleaseTarget).
		SetMilestoneName("2.4.0").
		SetTargetDate(targetDate).
		SetOutcomeDate(outcomeDate).
		SetMilestoneState(workprogrammilestone.MilestoneStateResolvedBeforeTarget).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthReleaseSignal).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(false).
		SetClaimGateReason("source_release_target_not_owner_commitment").
		SetSourceField("jira.fields.fixVersions.releaseDate").
		SetSourcePayloadKey("FLINK-1:fields.fixVersions:2.4.0").
		SetCapturedAt(generatedAt).
		SetGeneratedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_milestone").
		SetExternalID(workstream + "|2026-06-23T11:13:38Z|release").
		SetSourceURL("https://issues.apache.org/jira/browse/FLINK-1").
		SetRankScore(75).
		SaveX(ctx)
	explicitDueDate := store.Client().WorkProgramMilestone.Create().
		SetKey("work-program-milestone:due-date").
		SetTicketID(ticketRow.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogrammilestone.SubjectKindTicket).
		SetSubjectKey("FLINK-1").
		SetMilestoneKind(workprogrammilestone.MilestoneKindExplicitDueDate).
		SetMilestoneName("Jira due date").
		SetTargetDate(dueDate).
		SetMilestoneState(workprogrammilestone.MilestoneStatePlanned).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthExplicitCommitment).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(true).
		SetClaimGateReason("source_native_due_date").
		SetSourceField("jira.fields.duedate").
		SetSourcePayloadKey("FLINK-1:fields.duedate").
		SetCapturedAt(generatedAt).
		SetGeneratedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_milestone").
		SetExternalID(workstream + "|2026-06-23T11:13:38Z|due").
		SetSourceURL("https://issues.apache.org/jira/browse/FLINK-1").
		SetRankScore(100).
		SaveX(ctx)
	outcome := store.Client().WorkProgramMilestone.Create().
		SetKey("work-program-milestone:outcome").
		SetTicketID(ticketRow.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogrammilestone.SubjectKindTicket).
		SetSubjectKey("FLINK-1").
		SetMilestoneKind(workprogrammilestone.MilestoneKindResolutionOutcome).
		SetMilestoneName("Jira resolution").
		SetOutcomeDate(outcomeDate).
		SetMilestoneState(workprogrammilestone.MilestoneStateOutcomeOnly).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthOutcomeEvidence).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(false).
		SetClaimGateReason("source_outcome_date_only").
		SetSourceField("jira.fields.resolutiondate").
		SetSourcePayloadKey("FLINK-1:fields.resolutiondate").
		SetCapturedAt(generatedAt).
		SetGeneratedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_milestone").
		SetExternalID(workstream + "|2026-06-23T11:13:38Z|outcome").
		SetSourceURL("https://issues.apache.org/jira/browse/FLINK-1").
		SetRankScore(50).
		SaveX(ctx)
	store.Client().WorkProgramMilestone.Create().
		SetKey("work-program-milestone:noise").
		SetTicketID(ticketRow.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogrammilestone.SubjectKindTicket).
		SetSubjectKey("FLINK-1").
		SetMilestoneKind(workprogrammilestone.MilestoneKindReleaseTarget).
		SetMilestoneName("Noise target").
		SetTargetDate(targetDate.AddDate(0, 1, 0)).
		SetMilestoneState(workprogrammilestone.MilestoneStatePlanned).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthReleaseSignal).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(false).
		SetClaimGateReason("same_timestamp_outside_run_member").
		SetSourceField("jira.fields.fixVersions.releaseDate").
		SetGeneratedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_milestone").
		SetExternalID(workstream + "|2026-06-23T11:13:38Z|noise").
		SetRankScore(1000).
		SaveX(ctx)

	run := store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:milestone-packet").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("ready").
		SetReadinessScore(90).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(3).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|2026-06-23T11:13:38Z|run").
		SetRankScore(90).
		SaveX(ctx)
	for _, member := range []*struct {
		id           int
		key          string
		externalKind string
		externalID   string
		memberRank   float64
	}{
		{explicitDueDate.ID, explicitDueDate.Key, explicitDueDate.ExternalKind, explicitDueDate.ExternalID, explicitDueDate.RankScore},
		{releaseTarget.ID, releaseTarget.Key, releaseTarget.ExternalKind, releaseTarget.ExternalID, releaseTarget.RankScore},
		{outcome.ID, outcome.Key, outcome.ExternalKind, outcome.ExternalID, outcome.RankScore},
	} {
		store.Client().WorkProgramRunMember.Create().
			SetWorkProgramRunID(run.ID).
			SetRunKey(run.Key).
			SetMemberTable(workProgramRunMemberTableMilestones).
			SetMemberID(member.id).
			SetMemberKey(member.key).
			SetMemberExternalKind(member.externalKind).
			SetMemberExternalID(member.externalID).
			SetMemberRankScore(member.memberRank).
			SetCreatedAt(generatedAt).
			SaveX(ctx)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	limit := 10
	packet, err := resolver.WorkProgramMilestonePacket(ctx, "workstream:flink-kubernetes-operator", &limit, &sourceArg)
	if err != nil {
		t.Fatalf("work program milestone packet: %v", err)
	}
	if packet.TotalCount != 3 || len(packet.Milestones) != 3 {
		t.Fatalf("packet rows = %d/%d, want 3/3: %#v", len(packet.Milestones), packet.TotalCount, packet)
	}
	if packet.ReleaseTargetCount != 1 || packet.DatedReleaseTargetCount != 1 {
		t.Fatalf("release target counts = %d/%d, want 1/1", packet.ReleaseTargetCount, packet.DatedReleaseTargetCount)
	}
	if packet.ExplicitDueDateCount != 1 || packet.DeliveryCommitmentCount != 1 || packet.DeliveryCommitmentAllowedCount != 1 {
		t.Fatalf("commitment counts = due %d delivery %d allowed %d, want 1/1/1", packet.ExplicitDueDateCount, packet.DeliveryCommitmentCount, packet.DeliveryCommitmentAllowedCount)
	}
	if packet.OutcomeFactCount != 1 || packet.DateClaimAllowedCount != 3 {
		t.Fatalf("outcome/date-claim counts = %d/%d, want 1/3", packet.OutcomeFactCount, packet.DateClaimAllowedCount)
	}
	if packet.Milestones[0].MilestoneKind != "explicit_due_date" || !packet.Milestones[0].DeliveryCommitmentAllowed {
		t.Fatalf("top milestone should be explicit due-date commitment: %#v", packet.Milestones[0])
	}
	release := packet.Milestones[1]
	if release.MilestoneKind != "release_target" || release.DeliveryCommitmentAllowed || release.CommitmentStrength != "release_signal" {
		t.Fatalf("release target leaked as a commitment: %#v", release)
	}
	if release.ClaimGateReason != "source_release_target_not_owner_commitment" {
		t.Fatalf("release target gate reason = %q", release.ClaimGateReason)
	}
	for _, badge := range packet.Badges {
		if badge.Key == "milestone:no_due_dates" || badge.Key == "milestone:no_delivery_commitments" {
			t.Fatalf("packet included false absence badge %q: %#v", badge.Key, packet.Badges)
		}
	}
}

func TestWorkProgramMilestonePacketBadgesAvoidAbsenceClaims(t *testing.T) {
	badges := workProgramMilestonePacketBadges(&model.WorkProgramMilestonePacket{})

	for _, badge := range badges {
		if strings.Contains(badge.Label, "No due-date commitments") || strings.Contains(badge.Label, "No delivery commitments") {
			t.Fatalf("badge makes unsupported absence claim: %#v", badge)
		}
		if strings.Contains(badge.Label, "observed") {
			if badge.Detail == nil || !strings.Contains(*badge.Detail, "Not an absence claim") {
				t.Fatalf("observed-only badge should carry absence gate detail: %#v", badge)
			}
		}
	}
}
