package opengraphfixture

import (
	"context"
	"path/filepath"
	"testing"

	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/opengraphassociation"
	"cubicle/services/ontology-service/ent/opengraphobject"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestOpenGraphFixtureQualityDefaultsAreUnknown(t *testing.T) {
	for _, value := range []string{"", " ", "typo"} {
		if got := objectFreshnessState(value); got != opengraphobject.FreshnessStateUnknown {
			t.Fatalf("objectFreshnessState(%q) = %q, want unknown", value, got)
		}
		if got := associationFreshnessState(value); got != opengraphassociation.FreshnessStateUnknown {
			t.Fatalf("associationFreshnessState(%q) = %q, want unknown", value, got)
		}
		if got := evidenceFreshnessState(value); got != evidence.FreshnessStateUnknown {
			t.Fatalf("evidenceFreshnessState(%q) = %q, want unknown", value, got)
		}
		if got := objectVisibility(value); got != opengraphobject.VisibilityUnknown {
			t.Fatalf("objectVisibility(%q) = %q, want unknown", value, got)
		}
		if got := associationVisibility(value); got != opengraphassociation.VisibilityUnknown {
			t.Fatalf("associationVisibility(%q) = %q, want unknown", value, got)
		}
		if got := evidenceVisibility(value); got != evidence.VisibilityUnknown {
			t.Fatalf("evidenceVisibility(%q) = %q, want unknown", value, got)
		}
	}
}

func TestOpenGraphFixtureQualityParsesExplicitSafeValues(t *testing.T) {
	if got := objectFreshnessState("fresh"); got != opengraphobject.FreshnessStateFresh {
		t.Fatalf("objectFreshnessState(fresh) = %q", got)
	}
	if got := associationFreshnessState("fresh"); got != opengraphassociation.FreshnessStateFresh {
		t.Fatalf("associationFreshnessState(fresh) = %q", got)
	}
	if got := evidenceFreshnessState("fresh"); got != evidence.FreshnessStateFresh {
		t.Fatalf("evidenceFreshnessState(fresh) = %q", got)
	}
	if got := objectVisibility("public"); got != opengraphobject.VisibilityPublic {
		t.Fatalf("objectVisibility(public) = %q", got)
	}
	if got := associationVisibility("public"); got != opengraphassociation.VisibilityPublic {
		t.Fatalf("associationVisibility(public) = %q", got)
	}
	if got := evidenceVisibility("public"); got != evidence.VisibilityPublic {
		t.Fatalf("evidenceVisibility(public) = %q", got)
	}
}

func TestOpenGraphFixtureLoadMapsExplicitVisibilityToCurrentACL(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	fixture := Fixture{
		SourceInstance: "workspace-acl",
		ObservedAt:     "2026-06-24T12:00:00Z",
		Objects: []ObjectRow{
			{
				ObjectType:   "customer_account",
				Key:          "customer:acme",
				Title:        "Acme",
				SourceSystem: "crm",
				ExternalKind: "account",
				ExternalID:   "acme",
			},
			{
				ObjectType:     "incident",
				Key:            "incident:private",
				Title:          "Private incident",
				SourceSystem:   "pagerduty",
				ExternalKind:   "incident",
				ExternalID:     "private",
				Visibility:     "private",
				ACLPolicyKey:   "pagerduty:workspace-acl:policy",
				VisibilityHash: "pagerduty-acl-hash-private",
			},
		},
		Associations: []AssociationRow{
			{
				From:            Ref{ObjectType: "customer_account", Key: "customer:acme"},
				To:              Ref{ObjectType: "incident", Key: "incident:private"},
				AssociationType: "affected_by",
				SourceSystem:    "pagerduty",
				ExternalKind:    "incident_link",
				ExternalID:      "acme:private",
				LocatorKind:     "incident_link",
				Visibility:      "restricted",
				ACLPolicyKey:    "pagerduty:workspace-acl:relationship-policy",
				VisibilityHash:  "pagerduty-acl-hash-relationship",
			},
		},
	}

	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("load open graph fixture: %v", err)
	}

	publicObject, err := store.Client().OpenGraphObject.Query().
		Where(opengraphobject.KeyEQ("customer:acme")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query public object: %v", err)
	}
	if publicObject.ACLState != opengraphobject.ACLStateUnknown || publicObject.ACLPolicyKey != "" || publicObject.VisibilityHash != "" {
		t.Fatalf("object without explicit visibility got ACL metadata: %+v", publicObject)
	}

	privateObject, err := store.Client().OpenGraphObject.Query().
		Where(opengraphobject.KeyEQ("incident:private")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query private object: %v", err)
	}
	if privateObject.ACLState != opengraphobject.ACLStateCurrent || privateObject.ACLPolicyKey != "pagerduty:workspace-acl:policy" || privateObject.VisibilityHash != "pagerduty-acl-hash-private" {
		t.Fatalf("private object ACL metadata = state=%s policy=%q hash=%q", privateObject.ACLState, privateObject.ACLPolicyKey, privateObject.VisibilityHash)
	}
	if privateObject.ACLCheckedAt.IsZero() {
		t.Fatalf("private object acl_checked_at was not set")
	}

	association, err := store.Client().OpenGraphAssociation.Query().
		Where(opengraphassociation.AssociationTypeEQ("affected_by")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query association: %v", err)
	}
	if association.ACLState != opengraphassociation.ACLStateCurrent || association.ACLPolicyKey != "pagerduty:workspace-acl:relationship-policy" || association.VisibilityHash != "pagerduty-acl-hash-relationship" {
		t.Fatalf("association ACL metadata = state=%s policy=%q hash=%q", association.ACLState, association.ACLPolicyKey, association.VisibilityHash)
	}

	evidenceRow, err := store.Client().Evidence.Query().
		Where(evidence.KeyEQ("evidence:affected_by:customer:acme->incident:private")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query evidence: %v", err)
	}
	if evidenceRow.ACLState != evidence.ACLStateCurrent || evidenceRow.ACLPolicyKey != "pagerduty:workspace-acl:relationship-policy" || evidenceRow.VisibilityHash != "pagerduty-acl-hash-relationship" {
		t.Fatalf("evidence ACL metadata = state=%s policy=%q hash=%q", evidenceRow.ACLState, evidenceRow.ACLPolicyKey, evidenceRow.VisibilityHash)
	}
	if evidenceRow.ACLPolicyKeySnapshot != "pagerduty:workspace-acl:relationship-policy" || evidenceRow.VisibilityHashSnapshot != "pagerduty-acl-hash-relationship" {
		t.Fatalf("evidence ACL snapshot = policy=%q hash=%q", evidenceRow.ACLPolicyKeySnapshot, evidenceRow.VisibilityHashSnapshot)
	}
}

func TestOpenGraphFixtureLoadAttachesSourceScopeProvenanceWhenExplicit(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	fixture := Fixture{
		SourceSystem:   "pypi",
		SourceInstance: "pypi.org/project/requests",
		ConnectorKind:  "pypi_json_api",
		ObservedAt:     "2026-06-24T16:53:04Z",
		SourceScope: &SourceScopeRow{
			ScopeKind:       "project",
			ScopeKey:        "requests",
			DisplayName:     "PyPI project requests",
			CrawlPolicy:     "pypi_project_release_minimum",
			RunKey:          "source-sync-run:pypi:project:requests:20260624T165304Z",
			Status:          "complete",
			FreshnessState:  "fresh",
			CoverageMode:    "partial_scope",
			StartedAt:       "2026-06-24T16:53:00Z",
			CompletedAt:     "2026-06-24T16:53:04Z",
			CoverageStartAt: "2026-06-24T16:53:00Z",
			CoverageEndAt:   "2026-06-24T16:53:04Z",
			CheckpointToken: "pypi-json:requests",
		},
		Objects: []ObjectRow{
			{
				ObjectType:     "pypi_project",
				Key:            "pypi:project:requests",
				Title:          "requests",
				SourceSystem:   "pypi",
				ExternalKind:   "project",
				ExternalID:     "requests",
				SourceURL:      "https://pypi.org/project/requests/",
				Visibility:     "public",
				FreshnessState: "fresh",
			},
			{
				ObjectType:     "pypi_release",
				Key:            "pypi:release:requests:2.34.2",
				Title:          "requests 2.34.2",
				SourceSystem:   "pypi",
				ExternalKind:   "release",
				ExternalID:     "requests==2.34.2",
				SourceURL:      "https://pypi.org/project/requests/2.34.2/",
				Visibility:     "public",
				FreshnessState: "fresh",
			},
		},
		Associations: []AssociationRow{
			{
				From:            Ref{ObjectType: "pypi_project", Key: "pypi:project:requests"},
				To:              Ref{ObjectType: "pypi_release", Key: "pypi:release:requests:2.34.2"},
				AssociationType: "has_release",
				SourceSystem:    "pypi",
				ExternalKind:    "project_release",
				ExternalID:      "requests==2.34.2",
				SourceURL:       "https://pypi.org/pypi/requests/json",
				LocatorKind:     "project_version_field",
				Locator:         "info.version=2.34.2",
				EvidenceKey:     "evidence:pypi:requests:has_release:2.34.2",
				Visibility:      "public",
				FreshnessState:  "fresh",
			},
		},
	}

	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("load open graph fixture: %v", err)
	}

	connection, err := store.Client().SourceConnection.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query source connection: %v", err)
	}
	if connection.SourceSystem != "pypi" || connection.SourceInstance != "pypi.org/project/requests" || connection.ConnectorKind != "pypi_json_api" {
		t.Fatalf("source connection = system=%q instance=%q kind=%q", connection.SourceSystem, connection.SourceInstance, connection.ConnectorKind)
	}
	scope, err := store.Client().SourceScope.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query source scope: %v", err)
	}
	if scope.SourceConnectionID != connection.ID || scope.ScopeKind != "project" || scope.ScopeKey != "requests" || scope.CrawlPolicy != "pypi_project_release_minimum" {
		t.Fatalf("source scope = conn=%d kind=%q key=%q policy=%q", scope.SourceConnectionID, scope.ScopeKind, scope.ScopeKey, scope.CrawlPolicy)
	}
	state, err := store.Client().SourceScopeState.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query source scope state: %v", err)
	}
	run, err := store.Client().SourceSyncRun.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query source sync run: %v", err)
	}
	if run.SourceScopeID != scope.ID || run.RunKey != "source-sync-run:pypi:project:requests:20260624T165304Z" {
		t.Fatalf("source sync run identity = scope=%d key=%q", run.SourceScopeID, run.RunKey)
	}
	if run.Status != sourcesyncrun.StatusComplete || run.CoverageMode != sourcesyncrun.CoverageModePartialScope || run.ObjectsSeenCount != 2 || run.ObjectsCreatedCount != 2 || run.RelationshipsCreatedCount != 1 || run.EvidenceCreatedCount != 1 {
		t.Fatalf("source sync run counters/state = status=%s coverage=%s seen=%d objects_created=%d relationships_created=%d evidence_created=%d", run.Status, run.CoverageMode, run.ObjectsSeenCount, run.ObjectsCreatedCount, run.RelationshipsCreatedCount, run.EvidenceCreatedCount)
	}
	if state.SourceScopeID != scope.ID || state.FreshnessState != sourcescopestate.FreshnessStateFresh || state.CoverageMode != sourcescopestate.CoverageModePartialScope || state.LastSuccessfulSyncRunID != run.ID {
		t.Fatalf("source scope state = scope=%d freshness=%s coverage=%s latest_run=%d", state.SourceScopeID, state.FreshnessState, state.CoverageMode, state.LastSuccessfulSyncRunID)
	}

	project, err := store.Client().OpenGraphObject.Query().
		Where(opengraphobject.KeyEQ("pypi:project:requests")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query project object: %v", err)
	}
	if project.SourceScopeStateID != state.ID {
		t.Fatalf("project source_scope_state_id = %d, want %d", project.SourceScopeStateID, state.ID)
	}
	association, err := store.Client().OpenGraphAssociation.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query open graph association: %v", err)
	}
	if association.SourceScopeStateID != state.ID {
		t.Fatalf("association source_scope_state_id = %d, want %d", association.SourceScopeStateID, state.ID)
	}
	evidenceRow, err := store.Client().Evidence.Query().
		Where(evidence.KeyEQ("evidence:pypi:requests:has_release:2.34.2")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query evidence: %v", err)
	}
	if evidenceRow.SourceScopeStateID != state.ID {
		t.Fatalf("evidence source_scope_state_id = %d, want %d", evidenceRow.SourceScopeStateID, state.ID)
	}
}

func TestOpenGraphFixtureLoadIsReplayIdempotent(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	fixture := replayOpenGraphFixture()

	firstSummary, err := Load(ctx, store.Client(), fixture)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if firstSummary != (Summary{ObjectCount: 2, AssociationCount: 1, EvidenceCount: 1}) {
		t.Fatalf("first summary = %+v", firstSummary)
	}
	firstObject, err := store.Client().OpenGraphObject.Query().
		Where(opengraphobject.ObjectTypeEQ("incident"), opengraphobject.KeyEQ("incident:sev1")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query first object: %v", err)
	}

	fixture.ObservedAt = "2026-06-24T12:05:00Z"
	fixture.Objects[0].Title = "Checkout outage - mitigated"
	fixture.Objects[0].SourceVersion = "v2"
	fixture.Objects[0].RankScore = 55
	fixture.Associations[0].SourceVersion = "v2"
	fixture.Associations[0].RankScore = 80

	secondSummary, err := Load(ctx, store.Client(), fixture)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if secondSummary != (Summary{ObjectCount: 2, AssociationCount: 1, EvidenceCount: 1}) {
		t.Fatalf("second summary = %+v", secondSummary)
	}
	objectCount, err := store.Client().OpenGraphObject.Query().Count(ctx)
	assertOpenGraphCount(t, "objects", 2, objectCount, err)
	associationCount, err := store.Client().OpenGraphAssociation.Query().Count(ctx)
	assertOpenGraphCount(t, "associations", 1, associationCount, err)
	evidenceCount, err := store.Client().Evidence.Query().Count(ctx)
	assertOpenGraphCount(t, "evidence", 1, evidenceCount, err)

	updatedObject, err := store.Client().OpenGraphObject.Query().
		Where(opengraphobject.ObjectTypeEQ("incident"), opengraphobject.KeyEQ("incident:sev1")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query updated object: %v", err)
	}
	if updatedObject.ID != firstObject.ID {
		t.Fatalf("object ID changed on replay: first=%d second=%d", firstObject.ID, updatedObject.ID)
	}
	if updatedObject.FirstSeenAt != firstObject.FirstSeenAt {
		t.Fatalf("first seen changed on replay: first=%s second=%s", firstObject.FirstSeenAt, updatedObject.FirstSeenAt)
	}
	if updatedObject.Title != "Checkout outage - mitigated" || updatedObject.SourceVersion != "v2" || updatedObject.RankScore != 55 {
		t.Fatalf("object was not updated from replay: %+v", updatedObject)
	}

	updatedAssociation, err := store.Client().OpenGraphAssociation.Query().
		Where(opengraphassociation.AssociationTypeEQ("mitigated_by")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query association: %v", err)
	}
	if updatedAssociation.EvidenceCount != 1 || updatedAssociation.SourceVersion != "v2" || updatedAssociation.RankScore != 80 {
		t.Fatalf("association was not idempotently updated: %+v", updatedAssociation)
	}
	updatedEvidence, err := store.Client().Evidence.Query().
		Where(evidence.KeyEQ("evidence:sev1:checkout-recovery")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query evidence: %v", err)
	}
	if updatedEvidence.SourceVersion != "v2" || updatedAssociation.LatestEvidenceID != updatedEvidence.ID {
		t.Fatalf("evidence was not reused and promoted: evidence=%+v association=%+v", updatedEvidence, updatedAssociation)
	}
}

func TestOpenGraphFixtureLoadTracksNewAssociationEvidenceOnce(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	fixture := replayOpenGraphFixture()
	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("first load: %v", err)
	}
	firstEvidence, err := store.Client().Evidence.Query().
		Where(evidence.KeyEQ("evidence:sev1:checkout-recovery")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query first evidence: %v", err)
	}

	fixture.ObservedAt = "2026-06-24T12:10:00Z"
	fixture.Associations[0].EvidenceKey = "evidence:sev1:checkout-recovery:field-confirmation"
	fixture.Associations[0].Locator = "runbook field confirmation"
	fixture.Associations[0].SourceVersion = "v2"
	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("second load with new evidence: %v", err)
	}
	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("third load replaying new evidence: %v", err)
	}

	evidenceCount, err := store.Client().Evidence.Query().Count(ctx)
	assertOpenGraphCount(t, "evidence", 2, evidenceCount, err)
	associationCount, err := store.Client().OpenGraphAssociation.Query().Count(ctx)
	assertOpenGraphCount(t, "associations", 1, associationCount, err)
	association, err := store.Client().OpenGraphAssociation.Query().
		Where(opengraphassociation.AssociationTypeEQ("mitigated_by")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query association: %v", err)
	}
	if association.EvidenceCount != 2 {
		t.Fatalf("association evidence count = %d, want 2 after one new evidence key", association.EvidenceCount)
	}
	if association.LatestEvidenceID == firstEvidence.ID {
		t.Fatalf("latest evidence still points at first evidence after new evidence replay: %+v", association)
	}
	latestEvidence, err := store.Client().Evidence.Query().
		Where(evidence.KeyEQ("evidence:sev1:checkout-recovery:field-confirmation")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query latest evidence: %v", err)
	}
	if association.LatestEvidenceID != latestEvidence.ID {
		t.Fatalf("association latest evidence ID = %d, want %d", association.LatestEvidenceID, latestEvidence.ID)
	}
}

func TestOpenGraphFixtureRelationshipIdentityIncludesAssociationType(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	fixture := replayOpenGraphFixture()
	discussion := fixture.Associations[0]
	discussion.AssociationType = "discussed_in"
	discussion.EvidenceKey = "evidence:sev1:checkout-discussion"
	discussion.Locator = "incident channel discussion"
	discussion.ExternalID = "sev1:checkout-discussion"
	fixture.Associations = append(fixture.Associations, discussion)

	if _, err := Load(ctx, store.Client(), fixture); err != nil {
		t.Fatalf("load with two relationship types: %v", err)
	}
	associationCount, err := store.Client().OpenGraphAssociation.Query().Count(ctx)
	assertOpenGraphCount(t, "associations", 2, associationCount, err)
	evidenceCount, err := store.Client().Evidence.Query().Count(ctx)
	assertOpenGraphCount(t, "evidence", 2, evidenceCount, err)
}

func assertOpenGraphCount(t *testing.T, label string, want int, got int, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("count %s: %v", label, err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", label, got, want)
	}
}

func replayOpenGraphFixture() Fixture {
	return Fixture{
		SourceInstance: "replay-source",
		ObservedAt:     "2026-06-24T12:00:00Z",
		Objects: []ObjectRow{
			{
				ObjectType:     "incident",
				Key:            "incident:sev1",
				Title:          "Checkout outage",
				SourceSystem:   "pagerduty",
				ExternalKind:   "incident",
				ExternalID:     "sev1",
				SourceURL:      "https://example.test/incidents/sev1",
				SourceVersion:  "v1",
				Visibility:     "public",
				FreshnessState: "fresh",
				PropertiesJSON: `{"severity":"sev1"}`,
				RankScore:      40,
			},
			{
				ObjectType:     "runbook_document",
				Key:            "doc:checkout-recovery",
				Title:          "Checkout recovery",
				SourceSystem:   "docs",
				ExternalKind:   "document",
				ExternalID:     "checkout-recovery",
				SourceURL:      "https://example.test/docs/checkout-recovery",
				SourceVersion:  "v1",
				Visibility:     "public",
				FreshnessState: "fresh",
				RankScore:      20,
			},
		},
		Associations: []AssociationRow{
			{
				From:            Ref{ObjectType: "incident", Key: "incident:sev1"},
				To:              Ref{ObjectType: "runbook_document", Key: "doc:checkout-recovery"},
				AssociationType: "mitigated_by",
				SourceSystem:    "pagerduty",
				ExternalKind:    "incident_link",
				ExternalID:      "sev1:checkout-recovery",
				SourceURL:       "https://example.test/incidents/sev1#runbook",
				SourceVersion:   "v1",
				LocatorKind:     "remote_link",
				Locator:         "runbook link",
				EvidenceKey:     "evidence:sev1:checkout-recovery",
				Visibility:      "public",
				FreshnessState:  "fresh",
				PropertiesJSON:  `{"linkType":"runbook"}`,
				RankScore:       70,
			},
		},
	}
}
