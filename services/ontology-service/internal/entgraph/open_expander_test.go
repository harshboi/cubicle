package entgraph

import (
	"context"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/opengraphassociation"
	"cubicle/services/ontology-service/ent/opengraphobject"
	"cubicle/services/ontology-service/internal/domain"
)

func TestOpenGraphExpanderReadFilterRunsBeforeTraversalAndFanout(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedOpenGraphACLFixture(t, ctx, client)

	expander := NewOpenGraphExpander(client)
	publicOnly := visibilityReadFilter("user:public-only", domain.VisibilityPublic)
	publicGraph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{ObjectType: domain.ObjectType("customer_account"), Key: "customer:acl"},
		AssociationTypes: []domain.AssociationType{
			domain.AssociationType("affected_by"),
			domain.AssociationType("has_runbook"),
			domain.AssociationType("updated_in"),
		},
		Depth:          2,
		LimitPerObject: 1,
		ReadFilter:     publicOnly,
	})
	if err != nil {
		t.Fatalf("expand public-only open graph: %v", err)
	}
	if !hasObject(publicGraph.Objects, domain.ObjectType("incident"), "incident:public-direct") {
		t.Fatalf("public-only objects = %#v, want lower-ranked public direct incident", publicGraph.Objects)
	}
	for _, forbidden := range []struct {
		objectType domain.ObjectType
		key        string
	}{
		{domain.ObjectType("incident"), "incident:private-hub"},
		{domain.ObjectType("runbook_document"), "runbook:behind-private-hub"},
		{domain.ObjectType("runbook_document"), "runbook:team-only"},
		{domain.ObjectType("slack_message"), "slack:team-descendant"},
	} {
		if hasObject(publicGraph.Objects, forbidden.objectType, forbidden.key) {
			t.Fatalf("public-only objects = %#v, leaked %s/%s", publicGraph.Objects, forbidden.objectType, forbidden.key)
		}
	}
	if len(publicGraph.Associations) != 1 || publicGraph.Associations[0].To.Key != "incident:public-direct" {
		t.Fatalf("public-only associations = %#v, want private/team candidates skipped before fanout", publicGraph.Associations)
	}

	teamAllowed := visibilityReadFilter("user:team-member", domain.VisibilityPublic, "team")
	teamGraph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{ObjectType: domain.ObjectType("customer_account"), Key: "customer:acl"},
		AssociationTypes: []domain.AssociationType{
			domain.AssociationType("affected_by"),
			domain.AssociationType("has_runbook"),
			domain.AssociationType("updated_in"),
		},
		Depth:          2,
		LimitPerObject: 1,
		ReadFilter:     teamAllowed,
	})
	if err != nil {
		t.Fatalf("expand team-allowed open graph: %v", err)
	}
	if hasObject(teamGraph.Objects, domain.ObjectType("incident"), "incident:private-hub") {
		t.Fatalf("team-allowed objects = %#v, should not include private hub", teamGraph.Objects)
	}
	for _, want := range []struct {
		objectType domain.ObjectType
		key        string
	}{
		{domain.ObjectType("runbook_document"), "runbook:team-only"},
		{domain.ObjectType("slack_message"), "slack:team-descendant"},
	} {
		if !hasObject(teamGraph.Objects, want.objectType, want.key) {
			t.Fatalf("team-allowed objects = %#v, want %s/%s", teamGraph.Objects, want.objectType, want.key)
		}
	}
	if hasObject(teamGraph.Objects, domain.ObjectType("incident"), "incident:public-direct") {
		t.Fatalf("team-allowed objects = %#v, lower-ranked public edge should not consume fanout before team edge", teamGraph.Objects)
	}

	allowAll := domain.ExpandReadFilter{
		PrincipalKey:       "user:allow-all",
		ObjectAllowed:      func(domain.Object) bool { return true },
		AssociationAllowed: func(domain.Association) bool { return true },
	}
	allowedGraph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: domain.ObjectType("customer_account"), Key: "customer:acl"},
		Depth:          1,
		LimitPerObject: 1,
		ReadFilter:     allowAll,
	})
	if err != nil {
		t.Fatalf("expand allow-all open graph: %v", err)
	}
	if !hasObject(allowedGraph.Objects, domain.ObjectType("incident"), "incident:private-hub") {
		t.Fatalf("allow-all objects = %#v, want high-ranked private hub", allowedGraph.Objects)
	}
	if len(allowedGraph.Associations) != 1 || allowedGraph.Associations[0].To.Key != "incident:private-hub" {
		t.Fatalf("allow-all associations = %#v, want private high-ranked relationship", allowedGraph.Associations)
	}
}

func visibilityReadFilter(principal string, allowedClasses ...string) domain.ExpandReadFilter {
	allowed := map[string]bool{"": true}
	for _, visibility := range allowedClasses {
		allowed[visibility] = true
	}
	return domain.ExpandReadFilter{
		PrincipalKey: principal,
		ObjectAllowed: func(object domain.Object) bool {
			return allowed[object.Visibility]
		},
		AssociationAllowed: func(association domain.Association) bool {
			return allowed[association.Metadata.Visibility]
		},
	}
}

func seedOpenGraphACLFixture(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 15, 0, 0, 0, time.UTC)
	account := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "customer_account",
		Key:        "customer:acl",
		Title:      "ACL Fixture Customer",
		Source:     "crm",
		Visibility: opengraphobject.VisibilityPublic,
		ObservedAt: observedAt,
		RankScore:  100,
	})
	privateHub := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "incident",
		Key:        "incident:private-hub",
		Title:      "Private high-ranked incident",
		Source:     "pagerduty",
		Visibility: opengraphobject.VisibilityPrivate,
		ObservedAt: observedAt,
		RankScore:  100,
	})
	publicDirect := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "incident",
		Key:        "incident:public-direct",
		Title:      "Public direct incident",
		Source:     "pagerduty",
		Visibility: opengraphobject.VisibilityPublic,
		ObservedAt: observedAt,
		RankScore:  10,
	})
	behindPrivate := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "runbook_document",
		Key:        "runbook:behind-private-hub",
		Title:      "Public runbook behind private hub",
		Source:     "docs",
		Visibility: opengraphobject.VisibilityPublic,
		ObservedAt: observedAt,
		RankScore:  80,
	})
	teamRunbook := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "runbook_document",
		Key:        "runbook:team-only",
		Title:      "Team runbook",
		Source:     "docs",
		Visibility: opengraphobject.VisibilityPublic,
		ObservedAt: observedAt,
		RankScore:  90,
	})
	teamDescendant := createOpenGraphTestObject(t, ctx, client, openGraphTestObjectSeed{
		ObjectType: "slack_message",
		Key:        "slack:team-descendant",
		Title:      "Team incident update",
		Source:     "slack",
		Visibility: opengraphobject.VisibilityPublic,
		ObservedAt: observedAt,
		RankScore:  80,
	})

	createOpenGraphTestAssociation(t, ctx, client, openGraphTestAssociationSeed{
		From:            account,
		To:              privateHub,
		AssociationType: "affected_by",
		Source:          "pagerduty",
		LocatorKind:     "incident_link",
		Visibility:      opengraphassociation.VisibilityPrivate,
		ObservedAt:      observedAt,
		RankScore:       100,
	})
	createOpenGraphTestAssociation(t, ctx, client, openGraphTestAssociationSeed{
		From:            account,
		To:              teamRunbook,
		AssociationType: "has_runbook",
		Source:          "docs",
		LocatorKind:     "runbook_link",
		Visibility:      opengraphassociation.VisibilityTeam,
		ObservedAt:      observedAt,
		RankScore:       90,
	})
	createOpenGraphTestAssociation(t, ctx, client, openGraphTestAssociationSeed{
		From:            account,
		To:              publicDirect,
		AssociationType: "affected_by",
		Source:          "pagerduty",
		LocatorKind:     "incident_link",
		Visibility:      opengraphassociation.VisibilityPublic,
		ObservedAt:      observedAt,
		RankScore:       10,
	})
	createOpenGraphTestAssociation(t, ctx, client, openGraphTestAssociationSeed{
		From:            privateHub,
		To:              behindPrivate,
		AssociationType: "mitigated_by",
		Source:          "docs",
		LocatorKind:     "runbook_link",
		Visibility:      opengraphassociation.VisibilityPublic,
		ObservedAt:      observedAt,
		RankScore:       80,
	})
	createOpenGraphTestAssociation(t, ctx, client, openGraphTestAssociationSeed{
		From:            teamRunbook,
		To:              teamDescendant,
		AssociationType: "updated_in",
		Source:          "slack",
		LocatorKind:     "slack_message",
		Visibility:      opengraphassociation.VisibilityTeam,
		ObservedAt:      observedAt,
		RankScore:       95,
	})
}

type openGraphTestObjectSeed struct {
	ObjectType string
	Key        string
	Title      string
	Source     string
	Visibility opengraphobject.Visibility
	ObservedAt time.Time
	RankScore  float64
}

func createOpenGraphTestObject(t *testing.T, ctx context.Context, client *genent.Client, seed openGraphTestObjectSeed) *genent.OpenGraphObject {
	t.Helper()
	return client.OpenGraphObject.Create().
		SetObjectType(seed.ObjectType).
		SetKey(seed.Key).
		SetTitle(seed.Title).
		SetSourceSystem(seed.Source).
		SetSourceInstance("acl-fixture").
		SetExternalKind(seed.ObjectType).
		SetExternalID(seed.Key).
		SetSourceUpdatedAt(seed.ObservedAt).
		SetFreshnessState(opengraphobject.FreshnessStateFresh).
		SetVisibility(seed.Visibility).
		SetConfidence(1).
		SetFirstSeenAt(seed.ObservedAt).
		SetLastConfirmedAt(seed.ObservedAt).
		SetLastActivityAt(seed.ObservedAt).
		SetRankScore(seed.RankScore).
		SaveX(ctx)
}

type openGraphTestAssociationSeed struct {
	From            *genent.OpenGraphObject
	To              *genent.OpenGraphObject
	AssociationType string
	Source          string
	LocatorKind     string
	Visibility      opengraphassociation.Visibility
	ObservedAt      time.Time
	RankScore       float64
}

func createOpenGraphTestAssociation(t *testing.T, ctx context.Context, client *genent.Client, seed openGraphTestAssociationSeed) *genent.OpenGraphAssociation {
	t.Helper()
	evidenceVisibility := evidence.Visibility(seed.Visibility.String())
	evidenceKey := "evidence:open-acl:" + seed.AssociationType + ":" + seed.From.Key + "->" + seed.To.Key
	evidenceRow := client.Evidence.Create().
		SetKey(evidenceKey).
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("open_graph_association").
		SetRelationshipKind(seed.AssociationType).
		SetLocatorKind(seed.LocatorKind).
		SetLocator(seed.From.Key + " -> " + seed.To.Key).
		SetSourceSystem(seed.Source).
		SetSourceInstance("acl-fixture").
		SetExternalKind(seed.LocatorKind).
		SetExternalID(seed.From.Key + "->" + seed.To.Key).
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidenceVisibility).
		SetConfidence(1).
		SetObservedAt(seed.ObservedAt).
		SaveX(ctx)
	return client.OpenGraphAssociation.Create().
		SetFromObject(seed.From).
		SetToObject(seed.To).
		SetAssociationType(seed.AssociationType).
		SetSourceSystem(seed.Source).
		SetSourceInstance("acl-fixture").
		SetExternalKind(seed.LocatorKind).
		SetExternalID(seed.From.Key + "->" + seed.To.Key).
		SetSourceUpdatedAt(seed.ObservedAt).
		SetFreshnessState(opengraphassociation.FreshnessStateFresh).
		SetVisibility(seed.Visibility).
		SetConfidence(1).
		SetRankScore(seed.RankScore).
		SetFirstSeenAt(seed.ObservedAt).
		SetLastConfirmedAt(seed.ObservedAt).
		SetLastActivityAt(seed.ObservedAt).
		SetEvidenceCount(1).
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
}
