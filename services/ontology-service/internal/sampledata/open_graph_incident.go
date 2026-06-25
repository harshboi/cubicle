package sampledata

import (
	"context"
	"fmt"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/opengraphassociation"
	"cubicle/services/ontology-service/ent/opengraphobject"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphcontext"
)

const (
	// OpenCustomerIncidentFixture is the Ent-backed generic graph fixture.
	OpenCustomerIncidentFixture        = "open-customer-incident"
	openCustomerIncidentSourceInstance = "workspace-a"
)

// OpenCustomerIncidentSeed returns the root object for the open graph fixture.
func OpenCustomerIncidentSeed() domain.ObjectRef {
	return domain.ObjectRef{ObjectType: domain.ObjectType("customer_account"), Key: "customer:acme"}
}

// OpenCustomerIncidentSourceAuthorityPolicy is the connector-owned authority
// matrix for relationship claims emitted by the open incident fixture.
func OpenCustomerIncidentSourceAuthorityPolicy() graphcontext.SourceAuthorityPolicy {
	return graphcontext.SourceAuthorityPolicy{
		RelationshipAuthority: map[string]graphcontext.RelationshipSourceAuthority{
			"affected_by": {
				PresenceSources: []string{"pagerduty"},
				PresenceLocatorKinds: map[string][]string{
					"pagerduty": {"incident_link"},
				},
			},
			"mitigated_by": {
				PresenceSources: []string{"docs"},
				PresenceLocatorKinds: map[string][]string{
					"docs": {"runbook_link"},
				},
			},
			"updated_in": {
				PresenceSources: []string{"slack"},
				PresenceLocatorKinds: map[string][]string{
					"slack": {"slack_message"},
				},
			},
		},
	}
}

// SeedOpenCustomerIncidentGraph writes a persisted non-work graph into the open
// Ent rows. It proves boundedGraphContext can traverse connector-shaped data
// without first adding customer_account, incident, runbook_document, or
// slack_message as product schemas.
func SeedOpenCustomerIncidentGraph(ctx context.Context, client *genent.Client) error {
	if client == nil {
		return fmt.Errorf("seed open customer incident graph: ent client is required")
	}
	observedAt := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	account, err := createOpenGraphObject(ctx, client, openGraphObjectSeed{
		ObjectType: "customer_account",
		Key:        "customer:acme",
		Title:      "Acme Corp",
		Source:     "crm",
		Kind:       "account",
		ExternalID: "acct-1",
		SourceURL:  "https://crm.example.test/accounts/acme",
		ObservedAt: observedAt,
		RankScore:  90,
	})
	if err != nil {
		return err
	}
	incident, err := createOpenGraphObject(ctx, client, openGraphObjectSeed{
		ObjectType: "incident",
		Key:        "incident:sev-42",
		Title:      "SEV-42 checkout outage",
		Source:     "pagerduty",
		Kind:       "incident",
		ExternalID: "sev-42",
		SourceURL:  "https://pagerduty.example.test/incidents/sev-42",
		ObservedAt: observedAt,
		RankScore:  80,
	})
	if err != nil {
		return err
	}
	hiddenIncident, err := createOpenGraphObject(ctx, client, openGraphObjectSeed{
		ObjectType: "incident",
		Key:        "incident:hidden-private",
		Title:      "Private payroll incident",
		Source:     "pagerduty",
		Kind:       "incident",
		ExternalID: "private-1",
		SourceURL:  "https://pagerduty.example.test/incidents/private-1",
		Visibility: "private",
		ObservedAt: observedAt,
		RankScore:  100,
	})
	if err != nil {
		return err
	}
	runbook, err := createOpenGraphObject(ctx, client, openGraphObjectSeed{
		ObjectType: "runbook_document",
		Key:        "runbook:failover",
		Title:      "Checkout failover runbook",
		Source:     "docs",
		Kind:       "runbook_document",
		ExternalID: "runbook/failover",
		SourceURL:  "https://docs.example.test/runbooks/failover",
		ObservedAt: observedAt,
		RankScore:  70,
	})
	if err != nil {
		return err
	}
	message, err := createOpenGraphObject(ctx, client, openGraphObjectSeed{
		ObjectType: "slack_message",
		Key:        "slack:incident-update-1",
		Title:      "Incident update 1",
		Source:     "slack",
		Kind:       "slack_message",
		ExternalID: "C1/T1",
		SourceURL:  "https://slack.example.test/archives/C1/p1",
		ObservedAt: observedAt,
		RankScore:  65,
	})
	if err != nil {
		return err
	}

	for _, seed := range []openGraphAssociationSeed{
		{
			From:            account,
			To:              hiddenIncident,
			AssociationType: "affected_by",
			Source:          "pagerduty",
			LocatorKind:     "incident_link",
			RankScore:       100,
			Visibility:      "private",
			ObservedAt:      observedAt,
		},
		{
			From:            account,
			To:              incident,
			AssociationType: "affected_by",
			Source:          "pagerduty",
			LocatorKind:     "incident_link",
			RankScore:       10,
			ObservedAt:      observedAt,
		},
		{
			From:            incident,
			To:              runbook,
			AssociationType: "mitigated_by",
			Source:          "docs",
			LocatorKind:     "runbook_link",
			RankScore:       9,
			ObservedAt:      observedAt,
		},
		{
			From:            incident,
			To:              message,
			AssociationType: "updated_in",
			Source:          "slack",
			LocatorKind:     "slack_message",
			RankScore:       8,
			ObservedAt:      observedAt,
		},
	} {
		if _, err := createOpenGraphAssociation(ctx, client, seed); err != nil {
			return err
		}
	}
	return nil
}

type openGraphObjectSeed struct {
	ObjectType string
	Key        string
	Title      string
	Source     string
	Kind       string
	ExternalID string
	SourceURL  string
	Visibility string
	ObservedAt time.Time
	RankScore  float64
}

func createOpenGraphObject(ctx context.Context, client *genent.Client, seed openGraphObjectSeed) (*genent.OpenGraphObject, error) {
	visibility := opengraphobject.VisibilityPublic
	if seed.Visibility == "private" {
		visibility = opengraphobject.VisibilityPrivate
	}
	row, err := client.OpenGraphObject.Create().
		SetObjectType(seed.ObjectType).
		SetKey(seed.Key).
		SetTitle(seed.Title).
		SetSourceSystem(seed.Source).
		SetSourceInstance(openCustomerIncidentSourceInstance).
		SetExternalKind(seed.Kind).
		SetExternalID(seed.ExternalID).
		SetSourceURL(seed.SourceURL).
		SetSourceUpdatedAt(seed.ObservedAt).
		SetFreshnessState(opengraphobject.FreshnessStateFresh).
		SetVisibility(visibility).
		SetConfidence(1).
		SetFirstSeenAt(seed.ObservedAt).
		SetLastConfirmedAt(seed.ObservedAt).
		SetLastActivityAt(seed.ObservedAt).
		SetRankScore(seed.RankScore).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed open graph object %s: %w", seed.Key, err)
	}
	return row, nil
}

type openGraphAssociationSeed struct {
	From            *genent.OpenGraphObject
	To              *genent.OpenGraphObject
	AssociationType string
	Source          string
	LocatorKind     string
	RankScore       float64
	Visibility      string
	ObservedAt      time.Time
}

func createOpenGraphAssociation(ctx context.Context, client *genent.Client, seed openGraphAssociationSeed) (*genent.OpenGraphAssociation, error) {
	if seed.From == nil || seed.To == nil {
		return nil, fmt.Errorf("seed open graph association %s: endpoints are required", seed.AssociationType)
	}
	associationVisibility := opengraphassociation.VisibilityPublic
	evidenceVisibility := evidence.VisibilityPublic
	if seed.Visibility == "private" {
		associationVisibility = opengraphassociation.VisibilityPrivate
		evidenceVisibility = evidence.VisibilityPrivate
	}
	evidenceKey := "evidence:" + seed.AssociationType + ":" + seed.From.Key + "->" + seed.To.Key
	evidenceRow, err := client.Evidence.Create().
		SetKey(evidenceKey).
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("open_graph_association").
		SetRelationshipKind(seed.AssociationType).
		SetLocatorKind(seed.LocatorKind).
		SetLocator(seed.From.Key + " -> " + seed.To.Key).
		SetSourceSystem(seed.Source).
		SetSourceInstance(openCustomerIncidentSourceInstance).
		SetExternalKind(seed.LocatorKind).
		SetExternalID(seed.From.Key + "->" + seed.To.Key).
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidenceVisibility).
		SetConfidence(1).
		SetObservedAt(seed.ObservedAt).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed open graph evidence %s: %w", evidenceKey, err)
	}
	row, err := client.OpenGraphAssociation.Create().
		SetFromObject(seed.From).
		SetToObject(seed.To).
		SetAssociationType(seed.AssociationType).
		SetSourceSystem(seed.Source).
		SetSourceInstance(openCustomerIncidentSourceInstance).
		SetExternalKind(seed.LocatorKind).
		SetExternalID(seed.From.Key + "->" + seed.To.Key).
		SetSourceUpdatedAt(seed.ObservedAt).
		SetFreshnessState(opengraphassociation.FreshnessStateFresh).
		SetVisibility(associationVisibility).
		SetConfidence(1).
		SetRankScore(seed.RankScore).
		SetFirstSeenAt(seed.ObservedAt).
		SetLastConfirmedAt(seed.ObservedAt).
		SetLastActivityAt(seed.ObservedAt).
		SetEvidenceCount(1).
		SetLatestEvidence(evidenceRow).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed open graph association %s %s->%s: %w", seed.AssociationType, seed.From.Key, seed.To.Key, err)
	}
	return row, nil
}
