package ontology

import "cubicle/services/ontology-service/internal/domain"

const (
	ObjectWorkstream       domain.ObjectType = "workstream"
	ObjectTicket           domain.ObjectType = "ticket"
	ObjectPullRequest      domain.ObjectType = "pull_request"
	ObjectCodeFile         domain.ObjectType = "code_file"
	ObjectDocument         domain.ObjectType = "document"
	ObjectDocumentFragment domain.ObjectType = "document_fragment"
	ObjectMessage          domain.ObjectType = "message"
	ObjectDecision         domain.ObjectType = "decision"
	ObjectBlocker          domain.ObjectType = "blocker"
	ObjectRisk             domain.ObjectType = "risk"
	ObjectPerson           domain.ObjectType = "person"
	ObjectTeam             domain.ObjectType = "team"
	ObjectActionCandidate  domain.ObjectType = "action_candidate"
)

const (
	AssocContains      domain.AssociationType = "contains"
	AssocHasComponent  domain.AssociationType = "has_component"
	AssocImplementedBy domain.AssociationType = "implemented_by"
	AssocChangesFile   domain.AssociationType = "changes_file"
	AssocDiscussedIn   domain.AssociationType = "discussed_in"
	AssocDocuments     domain.AssociationType = "documents"
	AssocSupports      domain.AssociationType = "supports"
	AssocBlockedBy     domain.AssociationType = "blocked_by"
	AssocOwnedBy       domain.AssociationType = "owned_by"
	AssocNeedsAction   domain.AssociationType = "needs_action"
	AssocEvidencedBy   domain.AssociationType = "evidenced_by"
)

type ObjectTypeDef struct {
	Type        domain.ObjectType `json:"type"`
	DisplayName string            `json:"display_name"`
	Description string            `json:"description,omitempty"`
}

type AssociationTypeDef struct {
	Type        domain.AssociationType `json:"type"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description,omitempty"`
	Inverse     domain.AssociationType `json:"inverse,omitempty"`
}

type Registry struct {
	ObjectTypes      map[domain.ObjectType]ObjectTypeDef
	AssociationTypes map[domain.AssociationType]AssociationTypeDef
}

func BuiltinRegistry() Registry {
	objects := []ObjectTypeDef{
		{Type: ObjectWorkstream, DisplayName: "Workstream"},
		{Type: ObjectTicket, DisplayName: "Ticket"},
		{Type: ObjectPullRequest, DisplayName: "Pull request"},
		{Type: ObjectCodeFile, DisplayName: "Code file"},
		{Type: ObjectDocument, DisplayName: "Document"},
		{Type: ObjectDocumentFragment, DisplayName: "Document fragment"},
		{Type: ObjectMessage, DisplayName: "Message"},
		{Type: ObjectDecision, DisplayName: "Decision"},
		{Type: ObjectBlocker, DisplayName: "Blocker"},
		{Type: ObjectRisk, DisplayName: "Risk"},
		{Type: ObjectPerson, DisplayName: "Person"},
		{Type: ObjectTeam, DisplayName: "Team"},
		{Type: ObjectActionCandidate, DisplayName: "Action candidate"},
	}
	associations := []AssociationTypeDef{
		{Type: AssocContains, DisplayName: "Contains"},
		{Type: AssocHasComponent, DisplayName: "Has component"},
		{Type: AssocImplementedBy, DisplayName: "Implemented by"},
		{Type: AssocChangesFile, DisplayName: "Changes file"},
		{Type: AssocDiscussedIn, DisplayName: "Discussed in"},
		{Type: AssocDocuments, DisplayName: "Documents"},
		{Type: AssocSupports, DisplayName: "Supports"},
		{Type: AssocBlockedBy, DisplayName: "Blocked by"},
		{Type: AssocOwnedBy, DisplayName: "Owned by"},
		{Type: AssocNeedsAction, DisplayName: "Needs action"},
		{Type: AssocEvidencedBy, DisplayName: "Evidenced by"},
	}

	registry := Registry{
		ObjectTypes:      make(map[domain.ObjectType]ObjectTypeDef, len(objects)),
		AssociationTypes: make(map[domain.AssociationType]AssociationTypeDef, len(associations)),
	}
	for _, object := range objects {
		registry.ObjectTypes[object.Type] = object
	}
	for _, association := range associations {
		registry.AssociationTypes[association.Type] = association
	}
	return registry
}

func (r Registry) HasObjectType(objectType domain.ObjectType) bool {
	_, ok := r.ObjectTypes[objectType]
	return ok
}

func (r Registry) HasAssociationType(associationType domain.AssociationType) bool {
	_, ok := r.AssociationTypes[associationType]
	return ok
}
