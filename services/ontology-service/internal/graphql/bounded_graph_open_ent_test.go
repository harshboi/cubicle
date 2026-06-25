package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entgraph"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
	"cubicle/services/ontology-service/internal/sampledata"
)

func TestBoundedGraphContextCanUseOpenEntGraphWithConnectorAuthority(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close ent store: %v", err)
		}
	})
	client := store.Client()
	if err := sampledata.SeedOpenCustomerIncidentGraph(ctx, client); err != nil {
		t.Fatalf("seed open customer incident graph: %v", err)
	}

	depth := 2
	limit := 3
	defaultPolicyResolver := (&Resolver{
		EntClient:     client,
		GraphExpander: entgraph.NewOpenGraphExpander(client),
	}).Query()
	defaultPolicyContext, err := defaultPolicyResolver.BoundedGraphContext(
		ctx,
		"customer_account",
		"customer:acme",
		[]string{"affected_by", "mitigated_by", "updated_in"},
		&depth,
		&limit,
	)
	if err != nil {
		t.Fatalf("boundedGraphContext with default policy: %v", err)
	}
	if !hasBoundedAssociation(defaultPolicyContext, "affected_by", false, "relationship_authority_policy_missing") {
		t.Fatalf("default authority context = %s, want open relationship blocked by central company-work policy", boundedGraphContextSearchText(defaultPolicyContext))
	}

	connectorPolicyResolver := (&Resolver{
		EntClient:                   client,
		GraphExpander:               entgraph.NewOpenGraphExpander(client),
		BoundedGraphSourceAuthority: sampledata.OpenCustomerIncidentSourceAuthorityPolicy(),
	}).Query()
	graphContext, err := connectorPolicyResolver.BoundedGraphContext(
		ctx,
		"customer_account",
		"customer:acme",
		[]string{"affected_by", "mitigated_by", "updated_in"},
		&depth,
		&limit,
	)
	if err != nil {
		t.Fatalf("boundedGraphContext with connector policy: %v", err)
	}

	serialized := boundedGraphContextSearchText(graphContext)
	for _, want := range []string{
		"customer_account customer:acme",
		"incident incident:sev-42",
		"runbook_document runbook:failover",
		"slack_message slack:incident-update-1",
	} {
		if !strings.Contains(serialized, want) {
			t.Fatalf("context = %s, want %q", serialized, want)
		}
	}
	if strings.Contains(serialized, "incident:hidden-private") {
		t.Fatalf("context = %s, private high-rank incident should be filtered before fanout accounting", serialized)
	}
	for _, associationType := range []string{"affected_by", "mitigated_by", "updated_in"} {
		if !hasBoundedAssociation(graphContext, associationType, true, "source_evidence_full_confidence") {
			t.Fatalf("context = %s, want %s claimable through connector authority", serialized, associationType)
		}
	}
}

func hasBoundedAssociation(context *model.BoundedGraphContext, associationType string, claimAllowed bool, gateReason string) bool {
	for _, association := range context.Associations {
		if association.AssociationType == associationType &&
			association.ClaimAllowed == claimAllowed &&
			association.ClaimGateReason == gateReason {
			return true
		}
	}
	return false
}
