package flink

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

func TestMapperBuildsEvidenceBackedFlinkGraph(t *testing.T) {
	records, err := LoadFixtureSnapshots(context.Background(), "testdata/flink-fixture", snapshotstore.NewStore(t.TempDir()))
	if err != nil {
		t.Fatalf("load fixture snapshots: %v", err)
	}
	mapper := Mapper{MapperVersion: FixtureMapperVersion}

	batches, err := mapper.Map(records, MapOptions{RunKeyPrefix: "run:test"})
	if err != nil {
		t.Fatalf("map records: %v", err)
	}
	if len(batches) != 4 {
		t.Fatalf("batch count = %d", len(batches))
	}
	all := flattenBatches(batches)
	assertMappedNode(t, all, "workstream:flink-autoscaler")
	assertMappedNode(t, all, "ticket:FLINK-39743")
	assertMappedNode(t, all, "pr:apache/flink-kubernetes-operator#1234")
	assertMappedNode(t, all, "code_file:apache/flink-kubernetes-operator:src/main/java/org/apache/flink/kubernetes/operator/autoscaler/Autoscaler.java")
	assertMappedNode(t, all, "document_fragment:apache/flink-kubernetes-operator:docs/content/docs/custom-resource/autoscaler.md#flink-39743")
	assertMappedNode(t, all, "message:ponymail:dev:20260607:flink-39743")
	assertMappedEdge(t, all, domain.PredicateContains)
	assertMappedEdge(t, all, domain.PredicateImplementedBy)
	assertMappedEdge(t, all, domain.PredicateChangesFile)
	assertMappedEdge(t, all, domain.PredicateSupports)
	assertMappedEdge(t, all, domain.PredicateDiscussedIn)
	assertMappedEdge(t, all, domain.PredicateNeedsAction)
}

func flattenBatches(batches []domain.IngestBatch) domain.IngestBatch {
	var all domain.IngestBatch
	for _, batch := range batches {
		all.Nodes = append(all.Nodes, batch.Nodes...)
		all.Edges = append(all.Edges, batch.Edges...)
	}
	return all
}

func assertMappedNode(t *testing.T, batch domain.IngestBatch, key string) {
	t.Helper()
	for _, node := range batch.Nodes {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %q", key)
}

func assertMappedEdge(t *testing.T, batch domain.IngestBatch, predicate domain.Predicate) {
	t.Helper()
	for _, edge := range batch.Edges {
		if edge.Metadata.Predicate == predicate {
			return
		}
	}
	t.Fatalf("missing edge predicate %q", predicate)
}
