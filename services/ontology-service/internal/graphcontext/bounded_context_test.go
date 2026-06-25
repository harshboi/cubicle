package graphcontext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestBuildBoundedGraphContextSupportsDocumentMessageTicketGraph(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 24, 9, 30, 0, 0, time.UTC)

	objects := []domain.Object{
		{
			ObjectType:     ontology.ObjectDocument,
			Key:            "doc:architecture-note",
			Title:          "Architecture note",
			Source:         "fixture",
			SourceInstance: "generic-bounded-graph-test",
			ExternalID:     "architecture-note",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectMessage,
			Key:            "message:standup-1",
			Title:          "Standup mention of rollout risk",
			Source:         "fixture",
			SourceInstance: "generic-bounded-graph-test",
			ExternalID:     "standup-1",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectTicket,
			Key:            "ticket:SUP-101",
			Title:          "Support escalation",
			Source:         "fixture",
			SourceInstance: "generic-bounded-graph-test",
			ExternalID:     "SUP-101",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	for _, association := range []domain.Association{
		testAssociation(objects[0].Ref(), objects[1].Ref(), "mentions", "evidence:doc-message", 1, observedAt),
		testAssociation(objects[1].Ref(), objects[2].Ref(), "possible_followup_for", "evidence:message-ticket-candidate", 0.4, observedAt),
	} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	graphContext, err := Build(ctx, store, domain.ExpandRequest{
		Start:          objects[0].Ref(),
		Depth:          2,
		LimitPerObject: 4,
	}, Options{
		Coverage: CoveragePolicy{
			CoverageState:          "sparse",
			AbsenceClaimsAllowed:   false,
			AbsenceClaimGateReason: "partial_message_history",
			Summary:                "Only selected document and message rows were loaded.",
		},
		Guardrails: []string{"Do not claim missing replies, owners, or blockers from sparse message history."},
	})
	if err != nil {
		t.Fatalf("build bounded graph context: %v", err)
	}

	if graphContext.ContextHash == "" {
		t.Fatal("expected context hash")
	}
	if graphContext.Seed.ObjectType != string(ontology.ObjectDocument) || graphContext.Seed.Key != "doc:architecture-note" {
		t.Fatalf("seed = %#v, want architecture document", graphContext.Seed)
	}
	if graphContext.ScopeMode != "bounded_graph_context" {
		t.Fatalf("scope mode = %q", graphContext.ScopeMode)
	}
	if graphContext.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("sparse coverage should not allow absence claims: %#v", graphContext.Coverage)
	}
	if !containsString(graphContext.Guardrails, "Source coverage gates absence claims; missing neighbors are unknown, not absent.") {
		t.Fatalf("missing automatic sparse coverage guardrail: %#v", graphContext.Guardrails)
	}
	if len(graphContext.Objects) != 3 {
		t.Fatalf("objects = %#v, want three reached objects", graphContext.Objects)
	}
	if len(graphContext.Associations) != 2 {
		t.Fatalf("associations = %#v, want two reached associations", graphContext.Associations)
	}

	claimable := associationByType(t, graphContext.Associations, "mentions")
	if !claimable.ClaimAllowed || claimable.ProofState != "source_observed" {
		t.Fatalf("source-backed association should be claimable: %#v", claimable)
	}
	candidate := associationByType(t, graphContext.Associations, "possible_followup_for")
	if candidate.ClaimAllowed || candidate.ClaimGateReason != "candidate_link_requires_human_review" || candidate.ProofState != "candidate" {
		t.Fatalf("low-confidence association should stay a validation lead: %#v", candidate)
	}

	encoded, err := json.Marshal(Envelope{BoundedGraphContext: graphContext})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	text := string(encoded)
	for _, expected := range []string{
		`"boundedGraphContext"`,
		`"objectType":"document"`,
		`"claimAllowed":false`,
		`"absenceClaimsAllowed":false`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("encoded context missing %s: %s", expected, text)
		}
	}
	for _, forbidden := range []string{"WorkProgram", "tpm_", "flink"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("generic bounded graph context leaked %q: %s", forbidden, text)
		}
	}
}

func TestBuildBoundedGraphContextDefaultReadFilterRejectsBlankVisibility(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	start := domain.Object{
		ObjectType:     ontology.ObjectDocument,
		Key:            "doc:public-start",
		Title:          "Public start",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	blank := domain.Object{
		ObjectType:     ontology.ObjectMessage,
		Key:            "message:blank-visibility",
		Title:          "Blank visibility message",
		FreshnessState: domain.FreshnessFresh,
	}
	for _, object := range []domain.Object{start, blank} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	association := testAssociation(start.Ref(), blank.Ref(), "mentions", "evidence:blank-visibility", 1, time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC))
	association.Metadata.Visibility = ""
	if err := store.UpsertAssociation(ctx, association); err != nil {
		t.Fatalf("upsert association: %v", err)
	}

	graphContext, err := Build(ctx, store, domain.ExpandRequest{
		Start:          start.Ref(),
		Depth:          1,
		LimitPerObject: 4,
	}, Options{Coverage: CoveragePolicy{CoverageState: "sparse"}})
	if err != nil {
		t.Fatalf("build bounded graph context: %v", err)
	}
	if len(graphContext.Objects) != 1 || graphContext.Objects[0].Key != start.Key {
		t.Fatalf("objects = %#v, want only the public start object", graphContext.Objects)
	}
	if len(graphContext.Associations) != 0 {
		t.Fatalf("associations = %#v, want blank-visibility edge filtered out", graphContext.Associations)
	}
}

func TestBuildBoundedGraphContextOmitsRestrictedRowsBeforePromptProjection(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	objects := []domain.Object{
		{
			ObjectType:     ontology.ObjectDocument,
			Key:            "doc:public-plan",
			Title:          "Public plan",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectMessage,
			Key:            "message:public-standup",
			Title:          "Public standup",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectTicket,
			Key:            "ticket:SECRET-1",
			Title:          "Private acquisition work",
			Visibility:     "private",
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectTicket,
			Key:            "ticket:PUBLIC-THROUGH-PRIVATE-EDGE",
			Title:          "Public endpoint reached only through private edge",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	publicAssociation := testAssociation(objects[0].Ref(), objects[1].Ref(), "mentions", "evidence:public-doc-message", 1, observedAt)
	privateTargetAssociation := testAssociation(objects[1].Ref(), objects[2].Ref(), "possible_followup_for", "evidence:private-target", 1, observedAt)
	privateEdgeAssociation := testAssociation(objects[1].Ref(), objects[3].Ref(), "possible_followup_for", "evidence:private-edge", 1, observedAt)
	privateEdgeAssociation.Metadata.Visibility = "private"
	for _, association := range []domain.Association{publicAssociation, privateTargetAssociation, privateEdgeAssociation} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	graphContext, err := Build(ctx, store, domain.ExpandRequest{
		Start:          objects[0].Ref(),
		Depth:          2,
		LimitPerObject: 8,
	}, Options{
		Coverage: CoveragePolicy{CoverageState: "sparse", AbsenceClaimsAllowed: false},
	})
	if err != nil {
		t.Fatalf("build bounded graph context: %v", err)
	}
	encoded, err := json.Marshal(Envelope{BoundedGraphContext: graphContext})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	text := string(encoded)
	if len(graphContext.Objects) != 2 || len(graphContext.Associations) != 1 {
		t.Fatalf("graph context = objects:%#v associations:%#v, want only public seed component", graphContext.Objects, graphContext.Associations)
	}
	for _, forbidden := range []string{"ticket:SECRET-1", "Private acquisition", "ticket:PUBLIC-THROUGH-PRIVATE-EDGE", "evidence:private-edge", "evidence:private-target"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("bounded graph context leaked restricted term %q: %s", forbidden, text)
		}
	}

	privateStart, err := Build(ctx, store, domain.ExpandRequest{
		Start:          objects[2].Ref(),
		Depth:          1,
		LimitPerObject: 4,
	}, Options{})
	if err == nil {
		t.Fatalf("private start returned graph %#v, want pre-expansion authorization miss", privateStart)
	}
	if !strings.Contains(err.Error(), "missing object") {
		t.Fatalf("private start error = %q, want pre-expansion authorization miss", err.Error())
	}
}

func TestBuildBoundedGraphContextUsesReadFilterBeforeTraversal(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 24, 14, 0, 0, 0, time.UTC)
	objects := []domain.Object{
		{
			ObjectType:     ontology.ObjectDocument,
			Key:            "doc:public-seed",
			Title:          "Public seed",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectMessage,
			Key:            "message:public-direct",
			Title:          "Public direct message",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectPerson,
			Key:            "person:secret-alice",
			Title:          "Secret Alice",
			Visibility:     "private",
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     ontology.ObjectTicket,
			Key:            "ticket:public-through-private-hub",
			Title:          "Public descendant through private hub",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	privateFirst := testAssociation(objects[0].Ref(), objects[2].Ref(), "mentions", "evidence:private-first", 1, observedAt)
	privateFirst.Key = "a-private-first"
	privateFirst.Metadata.Visibility = "private"
	publicSecond := testAssociation(objects[0].Ref(), objects[1].Ref(), "mentions", "evidence:public-second", 1, observedAt)
	publicSecond.Key = "z-public-second"
	privateHubToPublic := testAssociation(objects[2].Ref(), objects[3].Ref(), "mentions", "evidence:private-hub-public-descendant", 1, observedAt)
	privateHubToPublic.Key = "private-hub-to-public-descendant"
	for _, association := range []domain.Association{privateFirst, publicSecond, privateHubToPublic} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	publicOnly := domain.ExpandReadFilter{
		PrincipalKey: "user:public-only",
		ObjectAllowed: func(object domain.Object) bool {
			return object.Visibility == "" || object.Visibility == domain.VisibilityPublic
		},
		AssociationAllowed: func(association domain.Association) bool {
			return association.Metadata.Visibility == "" || association.Metadata.Visibility == domain.VisibilityPublic
		},
	}
	blocked, err := Build(ctx, store, domain.ExpandRequest{
		Start:          objects[0].Ref(),
		Depth:          2,
		LimitPerObject: 1,
		ReadFilter:     publicOnly,
	}, Options{Coverage: CoveragePolicy{CoverageState: "sparse", AbsenceClaimsAllowed: false}})
	if err != nil {
		t.Fatalf("build public-only context: %v", err)
	}
	blockedText := mustJSON(t, Envelope{BoundedGraphContext: blocked})
	for _, expected := range []string{"doc:public-seed", "message:public-direct", "z-public-second"} {
		if !strings.Contains(blockedText, expected) {
			t.Fatalf("public-only context missing %q: %s", expected, blockedText)
		}
	}
	for _, forbidden := range []string{"person:secret-alice", "Secret Alice", "ticket:public-through-private-hub", "evidence:private-first", "private-hub-to-public-descendant"} {
		if strings.Contains(blockedText, forbidden) {
			t.Fatalf("public-only context leaked %q: %s", forbidden, blockedText)
		}
	}
	if len(blocked.Associations) != 1 || blocked.Associations[0].Key != "z-public-second" {
		t.Fatalf("public-only associations = %#v, want private edge skipped before fanout limit", blocked.Associations)
	}

	allowAll := domain.ExpandReadFilter{
		PrincipalKey:       "user:allowed",
		ObjectAllowed:      func(domain.Object) bool { return true },
		AssociationAllowed: func(domain.Association) bool { return true },
	}
	allowed, err := Build(ctx, store, domain.ExpandRequest{
		Start:          objects[0].Ref(),
		Depth:          2,
		LimitPerObject: 2,
		ReadFilter:     allowAll,
	}, Options{Coverage: CoveragePolicy{CoverageState: "sparse", AbsenceClaimsAllowed: false}})
	if err != nil {
		t.Fatalf("build allowed context: %v", err)
	}
	allowedText := mustJSON(t, Envelope{BoundedGraphContext: allowed})
	for _, expected := range []string{"person:secret-alice", "ticket:public-through-private-hub", "private-hub-to-public-descendant"} {
		if !strings.Contains(allowedText, expected) {
			t.Fatalf("allowed context missing %q: %s", expected, allowedText)
		}
	}
}

func TestBuildBoundedGraphContextRequiresRelationScopedCoverageForAbsenceClaims(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	object := domain.Object{
		ObjectType:     ontology.ObjectTicket,
		Key:            "ticket:COVERAGE-1",
		Title:          "Coverage fixture",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	if err := store.UpsertObject(ctx, object); err != nil {
		t.Fatalf("upsert object: %v", err)
	}
	req := domain.ExpandRequest{
		Start:            object.Ref(),
		AssociationTypes: []domain.AssociationType{"implemented_by"},
		Depth:            1,
		LimitPerObject:   4,
	}

	incomplete, err := Build(ctx, store, req, Options{
		Coverage: CoveragePolicy{
			CoverageState:                "sparse",
			AbsenceClaimsAllowed:         true,
			AbsenceClaimGateReason:       "test_complete_seed_only",
			AbsenceClaimAssociationTypes: []string{"implemented_by"},
		},
	})
	if err != nil {
		t.Fatalf("build incomplete coverage: %v", err)
	}
	if incomplete.Coverage.AbsenceClaimsAllowed || incomplete.Coverage.AbsenceClaimGateReason != "source_coverage_not_complete" {
		t.Fatalf("incomplete coverage = %#v, want absence claims clamped", incomplete.Coverage)
	}

	seedOnly, err := Build(ctx, store, req, Options{
		Coverage: CoveragePolicy{
			CoverageState:          "complete",
			AbsenceClaimsAllowed:   true,
			AbsenceClaimGateReason: "complete_source_coverage",
		},
	})
	if err != nil {
		t.Fatalf("build seed-only coverage: %v", err)
	}
	if seedOnly.Coverage.AbsenceClaimsAllowed || seedOnly.Coverage.AbsenceClaimGateReason != "relation_path_coverage_required" {
		t.Fatalf("seed-only coverage = %#v, want relation/path clamp", seedOnly.Coverage)
	}

	relationOnly, err := Build(ctx, store, req, Options{
		Coverage: CoveragePolicy{
			CoverageState:                "complete",
			AbsenceClaimsAllowed:         true,
			AbsenceClaimGateReason:       "complete_relation_path_coverage",
			AbsenceClaimAssociationTypes: []string{"implemented_by"},
		},
	})
	if err != nil {
		t.Fatalf("build relation-only coverage: %v", err)
	}
	if relationOnly.Coverage.AbsenceClaimsAllowed || relationOnly.Coverage.AbsenceClaimGateReason != "source_scope_coverage_required" {
		t.Fatalf("relation-only coverage = %#v, want source-scope clamp", relationOnly.Coverage)
	}

	sourceScoped, err := Build(ctx, store, req, Options{
		Coverage: CoveragePolicy{
			CoverageState:                "complete",
			AbsenceClaimsAllowed:         true,
			AbsenceClaimGateReason:       "complete_relation_path_coverage",
			AbsenceClaimAssociationTypes: []string{"implemented_by"},
			SourceSystem:                 "jira",
			SourceInstance:               "company",
		},
	})
	if err != nil {
		t.Fatalf("build source-scoped coverage: %v", err)
	}
	if sourceScoped.Coverage.AbsenceClaimsAllowed || sourceScoped.Coverage.AbsenceClaimGateReason != "source_time_window_required" {
		t.Fatalf("source-scoped coverage = %#v, want time-window clamp", sourceScoped.Coverage)
	}

	matchingPath, err := Build(ctx, store, req, Options{
		Coverage: CoveragePolicy{
			CoverageState:                "complete",
			AbsenceClaimsAllowed:         true,
			AbsenceClaimGateReason:       "complete_relation_path_coverage",
			AbsenceClaimAssociationTypes: []string{"implemented_by"},
			SourceSystem:                 "jira",
			SourceInstance:               "company",
			CoverageWindowStart:          "2026-06-24T00:00:00Z",
			CoverageWindowEnd:            "2026-06-24T01:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("build matching path/source/time coverage: %v", err)
	}
	if !matchingPath.Coverage.AbsenceClaimsAllowed || matchingPath.Coverage.AbsenceClaimGateReason != "complete_relation_path_coverage" {
		t.Fatalf("matching path coverage = %#v, want absence claims allowed", matchingPath.Coverage)
	}

	unfiltered, err := Build(ctx, store, domain.ExpandRequest{
		Start:          object.Ref(),
		Depth:          1,
		LimitPerObject: 4,
	}, Options{
		Coverage: CoveragePolicy{
			CoverageState:                "complete",
			AbsenceClaimsAllowed:         true,
			AbsenceClaimGateReason:       "complete_relation_path_coverage",
			AbsenceClaimAssociationTypes: []string{"implemented_by"},
			SourceSystem:                 "jira",
			SourceInstance:               "company",
			CoverageWindowStart:          "2026-06-24T00:00:00Z",
			CoverageWindowEnd:            "2026-06-24T01:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("build unfiltered coverage: %v", err)
	}
	if unfiltered.Coverage.AbsenceClaimsAllowed || unfiltered.Coverage.AbsenceClaimGateReason != "relation_path_coverage_required" {
		t.Fatalf("unfiltered coverage = %#v, want relation/path clamp for broad traversal", unfiltered.Coverage)
	}
}

func TestBuildBoundedGraphContextHashIsStable(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	object := domain.Object{
		ObjectType:     ontology.ObjectDocument,
		Key:            "doc:stable",
		Title:          "Stable doc",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	if err := store.UpsertObject(ctx, object); err != nil {
		t.Fatalf("upsert object: %v", err)
	}
	req := domain.ExpandRequest{Start: object.Ref(), Depth: 1, LimitPerObject: 2}
	opts := Options{Coverage: CoveragePolicy{CoverageState: "complete", AbsenceClaimsAllowed: true, AbsenceClaimGateReason: "complete_source_coverage"}}

	first, err := Build(ctx, store, req, opts)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := Build(ctx, store, req, opts)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first.ContextHash != second.ContextHash {
		t.Fatalf("context hash changed across identical inputs: %s != %s", first.ContextHash, second.ContextHash)
	}
}

func TestBuildBoundedGraphContextGatesConflictingMultiEvidenceAssociation(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 24, 15, 0, 0, 0, time.UTC)
	document := domain.Object{
		ObjectType:     ontology.ObjectDocument,
		Key:            "doc:multi-evidence",
		Title:          "Multi evidence doc",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	ticket := domain.Object{
		ObjectType:     ontology.ObjectTicket,
		Key:            "ticket:MULTI-1",
		Title:          "Multi evidence ticket",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	for _, object := range []domain.Object{document, ticket} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	current := testAssociation(document.Ref(), ticket.Ref(), "documented_by", "evidence:current-doc-ticket", 1, observedAt)
	current.Key = "association:doc-ticket-current"
	stale := testAssociation(document.Ref(), ticket.Ref(), "documented_by", "evidence:stale-doc-ticket", 1, observedAt.Add(-24*time.Hour))
	stale.Key = "association:doc-ticket-stale"
	stale.Metadata.EvidenceProofState = "stale"
	stale.Metadata.FreshnessState = "stale"
	for _, association := range []domain.Association{current, stale} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	graphContext, err := Build(ctx, store, domain.ExpandRequest{
		Start:          document.Ref(),
		Depth:          1,
		LimitPerObject: 4,
	}, Options{
		Coverage: CoveragePolicy{CoverageState: "sparse", AbsenceClaimsAllowed: false},
	})
	if err != nil {
		t.Fatalf("build bounded graph context: %v", err)
	}
	if len(graphContext.Associations) != 2 {
		t.Fatalf("associations = %#v, want both evidence rows visible for audit", graphContext.Associations)
	}
	for _, association := range graphContext.Associations {
		if association.ClaimAllowed || association.ClaimGateReason != "relationship_multi_evidence_requires_review" {
			t.Fatalf("association = %#v, want multi-evidence review gate", association)
		}
	}
	if len(graphContext.Evidence) != 2 {
		t.Fatalf("evidence rows = %#v, want both current and stale evidence refs preserved", graphContext.Evidence)
	}
}

func TestBuildBoundedGraphContextGatesDuplicateCurrentAssociation(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	observedAt := time.Date(2026, 6, 24, 15, 30, 0, 0, time.UTC)
	document := domain.Object{
		ObjectType:     ontology.ObjectDocument,
		Key:            "doc:duplicate-current",
		Title:          "Duplicate current doc",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	ticket := domain.Object{
		ObjectType:     ontology.ObjectTicket,
		Key:            "ticket:DUP-1",
		Title:          "Duplicate current ticket",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
	}
	for _, object := range []domain.Object{document, ticket} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	first := testAssociation(document.Ref(), ticket.Ref(), "documented_by", "evidence:duplicate-doc-ticket-a", 1, observedAt)
	first.Key = "association:doc-ticket-duplicate-a"
	second := testAssociation(document.Ref(), ticket.Ref(), "documented_by", "evidence:duplicate-doc-ticket-b", 1, observedAt.Add(time.Minute))
	second.Key = "association:doc-ticket-duplicate-b"
	for _, association := range []domain.Association{first, second} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	graphContext, err := Build(ctx, store, domain.ExpandRequest{
		Start:          document.Ref(),
		Depth:          1,
		LimitPerObject: 4,
	}, Options{
		Coverage: CoveragePolicy{CoverageState: "sparse", AbsenceClaimsAllowed: false},
	})
	if err != nil {
		t.Fatalf("build bounded graph context: %v", err)
	}
	if len(graphContext.Associations) != 2 {
		t.Fatalf("associations = %#v, want both duplicate rows visible for merge audit", graphContext.Associations)
	}
	for _, association := range graphContext.Associations {
		if association.ClaimAllowed || association.ClaimGateReason != "relationship_multi_evidence_requires_review" {
			t.Fatalf("association = %#v, want duplicate relationship merge gate", association)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(encoded)
}

func TestAssociationClaimPolicyChecksEvidenceMetadata(t *testing.T) {
	base := domain.Association{
		From:            domain.ObjectRef{ObjectType: ontology.ObjectDocument, Key: "doc:policy"},
		To:              domain.ObjectRef{ObjectType: ontology.ObjectMessage, Key: "message:policy"},
		AssociationType: "mentions",
		Metadata: domain.AssociationMetadata{
			EvidenceKey:              "evidence:policy",
			EvidenceClaimKind:        "relationship",
			EvidenceRelationshipKind: "mentions",
			EvidenceProofState:       "current",
			Confidence:               1,
			Visibility:               domain.VisibilityPublic,
			FreshnessState:           domain.FreshnessFresh,
		},
	}
	endpoints := domainObjectsByRef([]domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:policy", FreshnessState: domain.FreshnessFresh},
		{ObjectType: ontology.ObjectMessage, Key: "message:policy", FreshnessState: domain.FreshnessFresh},
	})
	if allowed, reason := associationClaimPolicy(base, endpoints, 1, SourceAuthorityPolicy{}); !allowed || reason != "source_evidence_full_confidence" {
		t.Fatalf("current relationship evidence policy = %v/%s, want claim allowed", allowed, reason)
	}

	sourceAuthority := SourceAuthorityPolicy{
		RelationshipAuthority: map[string]RelationshipSourceAuthority{
			"mentions": {
				PresenceSources:      []string{"fixture"},
				PresenceLocatorKinds: map[string][]string{"fixture": []string{"fixture_locator"}},
			},
		},
	}
	authoritative := base
	authoritative.Metadata.EvidenceSource = "fixture"
	authoritative.Metadata.EvidenceLocatorKind = "fixture_locator"
	if allowed, reason := associationClaimPolicy(authoritative, endpoints, 1, sourceAuthority); !allowed || reason != "source_evidence_full_confidence" {
		t.Fatalf("authoritative source policy = %v/%s, want claim allowed", allowed, reason)
	}

	missingEvidenceSource := base
	if allowed, reason := associationClaimPolicy(missingEvidenceSource, endpoints, 1, sourceAuthority); allowed || reason != "relationship_source_authority_missing_evidence_source" {
		t.Fatalf("missing evidence source policy = %v/%s, want evidence-source gate", allowed, reason)
	}

	wrongEvidenceSource := authoritative
	wrongEvidenceSource.Metadata.EvidenceSource = "chat"
	if allowed, reason := associationClaimPolicy(wrongEvidenceSource, endpoints, 1, sourceAuthority); allowed || reason != "relationship_source_not_authoritative_for_presence" {
		t.Fatalf("wrong source policy = %v/%s, want source-authority gate", allowed, reason)
	}

	instanceAuthority := SourceAuthorityPolicy{
		RelationshipAuthority: map[string]RelationshipSourceAuthority{
			"mentions": {
				PresenceSources:         []string{"fixture"},
				PresenceSourceInstances: map[string][]string{"fixture": []string{"fixture-a"}},
				PresenceLocatorKinds:    map[string][]string{"fixture": []string{"fixture_locator"}},
			},
		},
	}
	instanceAuthoritative := authoritative
	instanceAuthoritative.Metadata.EvidenceSourceInstance = "fixture-a"
	if allowed, reason := associationClaimPolicy(instanceAuthoritative, endpoints, 1, instanceAuthority); !allowed || reason != "source_evidence_full_confidence" {
		t.Fatalf("authoritative source instance policy = %v/%s, want claim allowed", allowed, reason)
	}
	missingEvidenceSourceInstance := authoritative
	if allowed, reason := associationClaimPolicy(missingEvidenceSourceInstance, endpoints, 1, instanceAuthority); allowed || reason != "relationship_source_authority_missing_evidence_source_instance" {
		t.Fatalf("missing evidence source instance policy = %v/%s, want evidence-source-instance gate", allowed, reason)
	}
	wrongEvidenceSourceInstance := instanceAuthoritative
	wrongEvidenceSourceInstance.Metadata.EvidenceSourceInstance = "fixture-b"
	if allowed, reason := associationClaimPolicy(wrongEvidenceSourceInstance, endpoints, 1, instanceAuthority); allowed || reason != "relationship_source_instance_not_authoritative_for_presence" {
		t.Fatalf("wrong source instance policy = %v/%s, want source-instance-authority gate", allowed, reason)
	}

	mapperVersionAuthority := SourceAuthorityPolicy{
		RelationshipAuthority: map[string]RelationshipSourceAuthority{
			"mentions": {
				PresenceSources:        []string{"fixture"},
				PresenceMapperVersions: map[string][]string{"fixture": []string{"mapper:v1"}},
				PresenceLocatorKinds:   map[string][]string{"fixture": []string{"fixture_locator"}},
			},
		},
	}
	mapperVersionAuthoritative := authoritative
	mapperVersionAuthoritative.Metadata.MapperVersion = "mapper:v1"
	if allowed, reason := associationClaimPolicy(mapperVersionAuthoritative, endpoints, 1, mapperVersionAuthority); !allowed || reason != "source_evidence_full_confidence" {
		t.Fatalf("authoritative mapper version policy = %v/%s, want claim allowed", allowed, reason)
	}
	missingMapperVersion := authoritative
	if allowed, reason := associationClaimPolicy(missingMapperVersion, endpoints, 1, mapperVersionAuthority); allowed || reason != "relationship_source_authority_missing_evidence_mapper_version" {
		t.Fatalf("missing mapper version policy = %v/%s, want mapper-version gate", allowed, reason)
	}
	wrongMapperVersion := mapperVersionAuthoritative
	wrongMapperVersion.Metadata.MapperVersion = "mapper:v2"
	if allowed, reason := associationClaimPolicy(wrongMapperVersion, endpoints, 1, mapperVersionAuthority); allowed || reason != "relationship_mapper_version_not_authoritative_for_presence" {
		t.Fatalf("wrong mapper version policy = %v/%s, want mapper-version-authority gate", allowed, reason)
	}

	missingLocator := authoritative
	missingLocator.Metadata.EvidenceLocatorKind = ""
	if allowed, reason := associationClaimPolicy(missingLocator, endpoints, 1, sourceAuthority); allowed || reason != "relationship_source_authority_missing_evidence_locator_kind" {
		t.Fatalf("missing locator policy = %v/%s, want locator-kind gate", allowed, reason)
	}

	wrongLocator := authoritative
	wrongLocator.Metadata.EvidenceLocatorKind = "weak_mention"
	if allowed, reason := associationClaimPolicy(wrongLocator, endpoints, 1, sourceAuthority); allowed || reason != "relationship_locator_not_authoritative_for_presence" {
		t.Fatalf("wrong locator policy = %v/%s, want locator-authority gate", allowed, reason)
	}

	missingAuthorityPolicy := authoritative
	missingAuthorityPolicy.AssociationType = "unknown_relation"
	missingAuthorityPolicy.Metadata.EvidenceRelationshipKind = "unknown_relation"
	if allowed, reason := associationClaimPolicy(missingAuthorityPolicy, endpoints, 1, sourceAuthority); allowed || reason != "relationship_authority_policy_missing" {
		t.Fatalf("missing relation policy = %v/%s, want policy-missing gate", allowed, reason)
	}

	missingEndpoint := domainObjectsByRef([]domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:policy", FreshnessState: domain.FreshnessFresh},
	})
	if allowed, reason := associationClaimPolicy(base, missingEndpoint, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_endpoint_missing_requires_hydration" {
		t.Fatalf("missing endpoint policy = %v/%s, want endpoint hydration gate", allowed, reason)
	}

	partialEndpoint := domainObjectsByRef([]domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:policy", FreshnessState: domain.FreshnessFresh},
		{ObjectType: ontology.ObjectMessage, Key: "message:policy", FreshnessState: "partial"},
	})
	if allowed, reason := associationClaimPolicy(base, partialEndpoint, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_endpoint_partial_requires_hydration" {
		t.Fatalf("partial endpoint policy = %v/%s, want endpoint hydration gate", allowed, reason)
	}

	unknownEndpoint := domainObjectsByRef([]domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:policy", FreshnessState: domain.FreshnessFresh},
		{ObjectType: ontology.ObjectMessage, Key: "message:policy", FreshnessState: "unknown"},
	})
	if allowed, reason := associationClaimPolicy(base, unknownEndpoint, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_endpoint_freshness_unknown" {
		t.Fatalf("unknown endpoint policy = %v/%s, want endpoint freshness gate", allowed, reason)
	}

	staleEndpoint := domainObjectsByRef([]domain.Object{
		{ObjectType: ontology.ObjectDocument, Key: "doc:policy", FreshnessState: domain.FreshnessFresh},
		{ObjectType: ontology.ObjectMessage, Key: "message:policy", FreshnessState: "stale"},
	})
	if allowed, reason := associationClaimPolicy(base, staleEndpoint, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_endpoint_not_current" {
		t.Fatalf("stale endpoint policy = %v/%s, want endpoint current-state gate", allowed, reason)
	}

	multipleEvidence := base
	multipleEvidence.Metadata.EvidenceCount = 2
	if allowed, reason := associationClaimPolicy(multipleEvidence, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_multi_evidence_requires_review" {
		t.Fatalf("multiple evidence policy = %v/%s, want multi-evidence review gate", allowed, reason)
	}
	authoritativeMultipleEvidence := authoritative
	authoritativeMultipleEvidence.Metadata.EvidenceCount = 2
	if allowed, reason := associationClaimPolicy(authoritativeMultipleEvidence, endpoints, 1, sourceAuthority); !allowed || reason != "source_evidence_full_confidence" {
		t.Fatalf("authoritative multiple evidence policy = %v/%s, want latest authoritative evidence to allow consolidated row", allowed, reason)
	}

	wrongClaimKind := base
	wrongClaimKind.Metadata.EvidenceClaimKind = "object_state"
	if allowed, reason := associationClaimPolicy(wrongClaimKind, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_claim_kind_mismatch" {
		t.Fatalf("wrong claim kind policy = %v/%s, want claim-kind mismatch", allowed, reason)
	}

	missingClaimKind := base
	missingClaimKind.Metadata.EvidenceClaimKind = ""
	if allowed, reason := associationClaimPolicy(missingClaimKind, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_claim_kind_missing" {
		t.Fatalf("missing claim kind policy = %v/%s, want claim-kind missing", allowed, reason)
	}

	wrongRelationshipKind := base
	wrongRelationshipKind.Metadata.EvidenceRelationshipKind = "documents"
	if allowed, reason := associationClaimPolicy(wrongRelationshipKind, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_kind_mismatch" {
		t.Fatalf("wrong relationship kind policy = %v/%s, want relationship-kind mismatch", allowed, reason)
	}

	missingRelationshipKind := base
	missingRelationshipKind.Metadata.EvidenceRelationshipKind = ""
	if allowed, reason := associationClaimPolicy(missingRelationshipKind, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_kind_missing" {
		t.Fatalf("missing relationship kind policy = %v/%s, want relationship-kind missing", allowed, reason)
	}

	generatedProof := base
	generatedProof.Metadata.EvidenceProofState = "generated"
	if allowed, reason := associationClaimPolicy(generatedProof, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_not_current" {
		t.Fatalf("generated proof policy = %v/%s, want non-current proof gate", allowed, reason)
	}

	missingProof := base
	missingProof.Metadata.EvidenceProofState = ""
	if allowed, reason := associationClaimPolicy(missingProof, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_evidence_proof_state_missing" {
		t.Fatalf("missing proof policy = %v/%s, want proof-state missing", allowed, reason)
	}

	missingFreshness := base
	missingFreshness.Metadata.FreshnessState = ""
	if allowed, reason := associationClaimPolicy(missingFreshness, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_freshness_missing" {
		t.Fatalf("missing freshness policy = %v/%s, want freshness missing", allowed, reason)
	}

	unknownFreshness := base
	unknownFreshness.Metadata.FreshnessState = "unknown"
	if allowed, reason := associationClaimPolicy(unknownFreshness, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_freshness_unknown" {
		t.Fatalf("unknown freshness policy = %v/%s, want freshness unknown", allowed, reason)
	}

	partialFreshness := base
	partialFreshness.Metadata.FreshnessState = "partial"
	if allowed, reason := associationClaimPolicy(partialFreshness, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_partial_requires_hydration" {
		t.Fatalf("partial freshness policy = %v/%s, want partial relationship gate", allowed, reason)
	}

	staleFreshness := base
	staleFreshness.Metadata.FreshnessState = "stale"
	if allowed, reason := associationClaimPolicy(staleFreshness, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_not_current" {
		t.Fatalf("stale freshness policy = %v/%s, want relationship current-state gate", allowed, reason)
	}

	missingVisibility := base
	missingVisibility.Metadata.Visibility = ""
	if allowed, reason := associationClaimPolicy(missingVisibility, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_visibility_missing" {
		t.Fatalf("missing visibility policy = %v/%s, want visibility missing", allowed, reason)
	}

	restrictedVisibility := base
	restrictedVisibility.Metadata.Visibility = "private"
	if allowed, reason := associationClaimPolicy(restrictedVisibility, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_visibility_restricted" {
		t.Fatalf("restricted visibility policy = %v/%s, want visibility restricted", allowed, reason)
	}

	generatedSource := base
	generatedSource.Metadata.Source = "cubicle_ai"
	if allowed, reason := associationClaimPolicy(generatedSource, endpoints, 1, SourceAuthorityPolicy{}); allowed || reason != "relationship_generated_requires_source_evidence" {
		t.Fatalf("generated relationship source policy = %v/%s, want writer gate", allowed, reason)
	}
}

func TestDefaultSourceAuthorityPolicyLoadsCanonicalMatrix(t *testing.T) {
	policy := DefaultSourceAuthorityPolicy()
	implementedBy, ok := policy.RelationshipAuthority["implemented_by"]
	if !ok {
		t.Fatalf("default source authority missing implemented_by: %#v", policy.RelationshipAuthority)
	}
	if !sourceAllowed("jira", implementedBy.PresenceSources) || !sourceAllowed("github", implementedBy.PresenceSources) || sourceAllowed("chat", implementedBy.PresenceSources) {
		t.Fatalf("implemented_by presence sources = %#v, want jira/github only", implementedBy.PresenceSources)
	}
	if !sourceAllowed("remote_link", implementedBy.PresenceLocatorKinds["jira"]) || !sourceAllowed("github_pull_request", implementedBy.PresenceLocatorKinds["github"]) || sourceAllowed("chat_message", implementedBy.PresenceLocatorKinds["jira"]) {
		t.Fatalf("implemented_by presence locators = %#v, want source-specific locator kinds", implementedBy.PresenceLocatorKinds)
	}
	linksTo, ok := policy.RelationshipAuthority["links_to"]
	if !ok || !sourceAllowed("docs", linksTo.PresenceSources) || sourceAllowed("jira", linksTo.PresenceSources) {
		t.Fatalf("links_to presence sources = %#v, want docs only", linksTo.PresenceSources)
	}
}

func TestObjectClaimPolicyChecksFreshnessVisibilityAndWriter(t *testing.T) {
	base := domain.Object{
		ObjectType:     ontology.ObjectPullRequest,
		Key:            "pull-request:repo/example#101",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
		Source:         "github",
	}
	if allowed, reason := objectClaimPolicy(base); !allowed || reason != "typed_graph_object" {
		t.Fatalf("fresh public source object policy = %v/%s, want claim allowed", allowed, reason)
	}

	missingFreshness := base
	missingFreshness.FreshnessState = ""
	if allowed, reason := objectClaimPolicy(missingFreshness); allowed || reason != "object_freshness_missing" {
		t.Fatalf("missing freshness object policy = %v/%s, want freshness missing gate", allowed, reason)
	}

	unknownFreshness := base
	unknownFreshness.FreshnessState = "unknown"
	if allowed, reason := objectClaimPolicy(unknownFreshness); allowed || reason != "object_freshness_unknown" {
		t.Fatalf("unknown freshness object policy = %v/%s, want freshness unknown gate", allowed, reason)
	}

	partial := base
	partial.FreshnessState = "partial"
	if allowed, reason := objectClaimPolicy(partial); allowed || reason != "object_partial_requires_hydration" {
		t.Fatalf("partial object policy = %v/%s, want hydration gate", allowed, reason)
	}

	stale := base
	stale.FreshnessState = "stale"
	if allowed, reason := objectClaimPolicy(stale); allowed || reason != "object_not_current" {
		t.Fatalf("stale object policy = %v/%s, want current-state gate", allowed, reason)
	}

	private := base
	private.Visibility = "private"
	if allowed, reason := objectClaimPolicy(private); allowed || reason != "object_visibility_restricted" {
		t.Fatalf("private object policy = %v/%s, want visibility gate", allowed, reason)
	}

	missingVisibility := base
	missingVisibility.Visibility = ""
	if allowed, reason := objectClaimPolicy(missingVisibility); allowed || reason != "object_visibility_missing" {
		t.Fatalf("missing visibility object policy = %v/%s, want visibility missing gate", allowed, reason)
	}

	generated := base
	generated.Source = "cubicle_ai"
	if allowed, reason := objectClaimPolicy(generated); allowed || reason != "object_generated_requires_source_evidence" {
		t.Fatalf("generated object policy = %v/%s, want writer gate", allowed, reason)
	}

	openGraphObject := base
	openGraphObject.ObjectType = domain.ObjectType("incident")
	if allowed, reason := objectClaimPolicy(openGraphObject); allowed || reason != "open_graph_object_context_only" {
		t.Fatalf("open graph object policy = %v/%s, want context-only gate", allowed, reason)
	}
}

func testAssociation(from domain.ObjectRef, to domain.ObjectRef, associationType domain.AssociationType, evidenceKey string, confidence float64, observedAt time.Time) domain.Association {
	return domain.Association{
		From:            from,
		To:              to,
		AssociationType: associationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:              evidenceKey,
			EvidenceClaimKind:        "relationship",
			EvidenceRelationshipKind: string(associationType),
			EvidenceProofState:       "current",
			EvidenceSource:           "fixture",
			EvidenceSourceInstance:   "generic-bounded-graph-test",
			EvidenceLocatorKind:      "fixture_relation",
			Source:                   "fixture",
			SourceInstance:           "generic-bounded-graph-test",
			Confidence:               confidence,
			Visibility:               domain.VisibilityPublic,
			FreshnessState:           domain.FreshnessFresh,
			ObservedAt:               observedAt,
		},
	}
}

func associationByType(t *testing.T, associations []Association, associationType string) Association {
	t.Helper()
	for _, association := range associations {
		if association.AssociationType == associationType {
			return association
		}
	}
	t.Fatalf("association type %q not found in %#v", associationType, associations)
	return Association{}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
