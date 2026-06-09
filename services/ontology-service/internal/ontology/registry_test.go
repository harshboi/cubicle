package ontology

import "testing"

func TestBuiltinRegistryContainsWorkstreamAssociationTerms(t *testing.T) {
	// registry is the built-in catalog product query code will use to classify
	// known object and association types.
	registry := BuiltinRegistry()

	if !registry.HasObjectType(ObjectWorkstream) || !registry.HasObjectType(ObjectTicket) {
		t.Fatalf("registry missing core object types: %#v", registry.ObjectTypes)
	}
	if !registry.HasAssociationType(AssocContains) || !registry.HasAssociationType(AssocImplementedBy) {
		t.Fatalf("registry missing core association types: %#v", registry.AssociationTypes)
	}
	if registry.HasObjectType("custom_ticket") || registry.HasAssociationType("custom_link") {
		t.Fatal("builtin registry should describe built-ins without treating custom terms as registered")
	}
}
