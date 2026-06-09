package flink

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
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
	assertMappedEdge(t, all, ontology.AssocContains)
	assertMappedEdge(t, all, ontology.AssocImplementedBy)
	assertMappedEdge(t, all, ontology.AssocChangesFile)
	assertMappedEdge(t, all, ontology.AssocSupports)
	assertMappedEdge(t, all, ontology.AssocDiscussedIn)
	assertMappedEdge(t, all, ontology.AssocNeedsAction)
}

func flattenBatches(batches []domain.IngestBatch) domain.IngestBatch {
	var all domain.IngestBatch
	for _, batch := range batches {
		all.Objects = append(all.Objects, batch.Objects...)
		all.Associations = append(all.Associations, batch.Associations...)
	}
	return all
}

func assertMappedNode(t *testing.T, batch domain.IngestBatch, key string) {
	t.Helper()
	for _, node := range batch.Objects {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %q", key)
}

func assertMappedEdge(t *testing.T, batch domain.IngestBatch, predicate domain.AssociationType) {
	t.Helper()
	for _, edge := range batch.Associations {
		if edge.AssociationType == predicate {
			return
		}
	}
	t.Fatalf("missing edge predicate %q", predicate)
}
