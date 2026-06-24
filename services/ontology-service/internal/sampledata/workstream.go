package sampledata

import (
	"context"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

// NewFakeFlinkAutoscalerMemoryStore returns a memory store seeded with sample data.
//
// This helper is test/dev scaffolding only. It gives tests a stable graph shape
// without implying that Flink data is part of the generic service contract.
func NewFakeFlinkAutoscalerMemoryStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	if err := SeedFakeFlinkAutoscalerWorkstream(ctx, store); err != nil {
		panic(err)
	}
	return store
}

// NewGenericDocumentMessageTicketMemoryStore returns a small non-WorkProgram graph.
func NewGenericDocumentMessageTicketMemoryStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	if err := SeedGenericDocumentMessageTicket(ctx, store); err != nil {
		panic(err)
	}
	return store
}

// NewCustomerIncidentRunbookMemoryStore returns a non-work-management graph.
func NewCustomerIncidentRunbookMemoryStore() *graphstore.MemoryStore {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	if err := SeedCustomerIncidentRunbook(ctx, store); err != nil {
		panic(err)
	}
	return store
}

// SeedFakeFlinkAutoscalerWorkstream writes a tiny fake workstream graph.
//
// The data is deliberately small and deterministic. Use it for tests, examples,
// and explicitly enabled local demo seeding; do not treat it as authoritative
// source data for the generic ontology service.
func SeedFakeFlinkAutoscalerWorkstream(ctx context.Context, store graphstore.Writer) error {
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	objects := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "fake_sampledata", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "fake_sampledata", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectBlocker, Key: "blocker:missing-review", Title: "Missing review", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectActionCandidate, Key: "action:request-review", Title: "Request review", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			return err
		}
	}

	associations := []domain.Association{
		fakeAssociation("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		fakeAssociation("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		fakeAssociation("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
		fakeAssociation("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocBlockedBy, "blocker:missing-review", ontology.ObjectBlocker, "evidence:review-gap", observedAt),
		fakeAssociation("blocker:missing-review", ontology.ObjectBlocker, ontology.AssocNeedsAction, "action:request-review", ontology.ObjectActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			return err
		}
	}
	return nil
}

// SeedGenericDocumentMessageTicket writes a source-neutral graph for AI-context demos.
//
// The second relationship is intentionally low-confidence so prompt/eval paths
// can prove candidate links are not promoted into confirmed product facts.
func SeedGenericDocumentMessageTicket(ctx context.Context, store graphstore.Writer) error {
	observedAt := time.Date(2026, 6, 24, 9, 30, 0, 0, time.UTC)

	objects := []domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:architecture-note", Title: "Architecture note", Source: "generic_sampledata", SourceInstance: "generic-doc-message-ticket", ExternalID: "architecture-note", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectMessage, Key: "message:standup-1", Title: "Standup mention of rollout risk", Source: "generic_sampledata", SourceInstance: "generic-doc-message-ticket", ExternalID: "standup-1", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:SUP-101", Title: "Support escalation", Source: "generic_sampledata", SourceInstance: "generic-doc-message-ticket", ExternalID: "SUP-101", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			return err
		}
	}

	associations := []domain.Association{
		genericAssociation(objects[0].Ref(), objects[1].Ref(), "mentions", "evidence:doc-message", 1, observedAt),
		genericAssociation(objects[1].Ref(), objects[2].Ref(), "possible_followup_for", "evidence:message-ticket-candidate", 0.4, observedAt),
	}
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			return err
		}
	}
	return nil
}

// SeedCustomerIncidentRunbook writes a non-ticket/PR graph for generic AI-context demos.
//
// The shared-channel branch is a deliberate near-neighbor distractor. Eval packs
// should use relation filters when they want the customer incident/runbook path
// without crossing a high-degree chat channel into unrelated incidents.
func SeedCustomerIncidentRunbook(ctx context.Context, store graphstore.Writer) error {
	observedAt := time.Date(2026, 6, 24, 11, 15, 0, 0, time.UTC)
	sourceInstance := "customer-incident-runbook"
	objectType := func(value string) domain.ObjectType { return domain.ObjectType(value) }
	associationType := func(value string) domain.AssociationType { return domain.AssociationType(value) }

	objects := []domain.Object{
		{ObjectType: objectType("customer_account"), Key: "customer-account:acme", Title: "Acme account", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "acme", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("incident"), Key: "incident:payments-latency", Title: "Payments latency incident", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "INC-1001", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("slack_message"), Key: "slack-message:payments-update-1", Title: "Payments incident update", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "payments-update-1", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("runbook_document"), Key: "runbook:payments-latency", Title: "Payments latency runbook", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "payments-latency-runbook", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("slack_channel"), Key: "slack-channel:customer-incidents", Title: "Customer incidents channel", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "customer-incidents", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("incident"), Key: "incident:finance-export", Title: "Finance export incident", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "INC-9999", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: objectType("runbook_document"), Key: "runbook:finance-export", Title: "Finance export runbook", Source: "generic_sampledata", SourceInstance: sourceInstance, ExternalID: "finance-export-runbook", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			return err
		}
	}

	associations := []domain.Association{
		fixtureAssociation(objects[0].Ref(), objects[1].Ref(), associationType("reported_incident"), "evidence:acme:payments-latency", sourceInstance, 1, observedAt),
		fixtureAssociation(objects[1].Ref(), objects[2].Ref(), associationType("has_update"), "evidence:incident:update-message", sourceInstance, 1, observedAt),
		fixtureAssociation(objects[1].Ref(), objects[3].Ref(), associationType("has_runbook"), "evidence:incident:payments-runbook", sourceInstance, 1, observedAt),
		fixtureAssociation(objects[1].Ref(), objects[4].Ref(), associationType("shared_channel"), "evidence:incident:shared-channel", sourceInstance, 1, observedAt),
		fixtureAssociation(objects[4].Ref(), objects[5].Ref(), associationType("mentions_incident"), "evidence:channel:finance-incident", sourceInstance, 1, observedAt),
		fixtureAssociation(objects[5].Ref(), objects[6].Ref(), associationType("has_runbook"), "evidence:incident:finance-runbook", sourceInstance, 1, observedAt),
	}
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			return err
		}
	}
	return nil
}

// fakeAssociation builds one evidence-backed association in the sample graph.
func fakeAssociation(fromKey string, fromKind domain.ObjectType, associationType domain.AssociationType, toKey string, toKind domain.ObjectType, evidenceKey string, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            domain.ObjectRef{ObjectType: fromKind, Key: fromKey},
		To:              domain.ObjectRef{ObjectType: toKind, Key: toKey},
		AssociationType: associationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:              evidenceKey,
			EvidenceClaimKind:        "relationship",
			EvidenceRelationshipKind: string(associationType),
			EvidenceProofState:       "current",
			Source:                   "fake_sampledata",
			Confidence:               1,
			Visibility:               "public",
			FreshnessState:           "fresh",
			ObservedAt:               observedAt,
		},
	}
}

func genericAssociation(from domain.ObjectRef, to domain.ObjectRef, associationType domain.AssociationType, evidenceKey string, confidence float64, observedAt time.Time) domain.Association {
	return fixtureAssociation(from, to, associationType, evidenceKey, "generic-doc-message-ticket", confidence, observedAt)
}

func fixtureAssociation(from domain.ObjectRef, to domain.ObjectRef, associationType domain.AssociationType, evidenceKey string, sourceInstance string, confidence float64, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            from,
		To:              to,
		AssociationType: associationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:              evidenceKey,
			EvidenceClaimKind:        "relationship",
			EvidenceRelationshipKind: string(associationType),
			EvidenceProofState:       "current",
			EvidenceSource:           "generic_sampledata",
			EvidenceSourceInstance:   sourceInstance,
			EvidenceLocatorKind:      "generic_relation",
			Source:                   "generic_sampledata",
			SourceInstance:           sourceInstance,
			Confidence:               confidence,
			Visibility:               "public",
			FreshnessState:           "fresh",
			ObservedAt:               observedAt,
		},
	}
}
