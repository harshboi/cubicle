package flink

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

func TestMapperBuildsEvidenceBackedFlinkGraph(t *testing.T) {
	records, err := LoadFlinkFixtureSnapshots(context.Background(), "testdata/flink-fixture", snapshotstore.NewStore(t.TempDir()))
	if err != nil {
		t.Fatalf("load fixture snapshots: %v", err)
	}
	mapper := FlinkFixtureMapper{MapperVersion: FlinkFixtureMapperVersion}

	batches, err := mapper.Map(records, MapOptions{RunKeyPrefix: "run:test"})
	if err != nil {
		t.Fatalf("map records: %v", err)
	}
	if len(batches) != 4 {
		t.Fatalf("batch count = %d", len(batches))
	}
	all := flattenBatches(batches)
	assertMappedObject(t, all, "workstream:flink-autoscaler")
	assertMappedObject(t, all, "ticket:FLINK-39743")
	assertMappedObject(t, all, "pr:apache/flink-kubernetes-operator#1234")
	assertMappedObject(t, all, "code_file:apache/flink-kubernetes-operator:src/main/java/org/apache/flink/kubernetes/operator/autoscaler/Autoscaler.java")
	assertMappedObject(t, all, "document_fragment:apache/flink-kubernetes-operator:docs/content/docs/custom-resource/autoscaler.md#flink-39743")
	assertMappedObject(t, all, "message:ponymail:dev:20260607:flink-39743")
	assertMappedAssociation(t, all, ontology.AssocContains)
	assertMappedAssociation(t, all, ontology.AssocImplementedBy)
	assertMappedAssociation(t, all, ontology.AssocChangesFile)
	assertMappedAssociation(t, all, ontology.AssocSupports)
	assertMappedAssociation(t, all, ontology.AssocDiscussedIn)
	assertMappedAssociation(t, all, ontology.AssocNeedsAction)
}

func flattenBatches(batches []domain.IngestBatch) domain.IngestBatch {
	var all domain.IngestBatch
	for _, batch := range batches {
		all.Objects = append(all.Objects, batch.Objects...)
		all.Associations = append(all.Associations, batch.Associations...)
	}
	return all
}

func assertMappedObject(t *testing.T, batch domain.IngestBatch, key string) {
	t.Helper()
	for _, object := range batch.Objects {
		if object.Key == key {
			return
		}
	}
	t.Fatalf("missing object %q", key)
}

func assertMappedAssociation(t *testing.T, batch domain.IngestBatch, associationType domain.AssociationType) {
	t.Helper()
	for _, association := range batch.Associations {
		if association.AssociationType == associationType {
			return
		}
	}
	t.Fatalf("missing association association_type %q", associationType)
}
