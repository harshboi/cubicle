package ontology

import "fmt"

// SurfaceKind is the major bounded work domain owned by a person.
type SurfaceKind string

// TargetKind is the concrete Ent object kind a WorkPane can link to.
type TargetKind string

// PaneKind is a specific bounded view inside a person's work surface.
type PaneKind string

// RelationKind is the semantic action represented by a pane target link.
type RelationKind string

const (
	SurfaceDocuments      SurfaceKind = "documents"      // SurfaceDocuments groups a person's document-related panes.
	SurfaceCode           SurfaceKind = "code"           // SurfaceCode groups a person's code and pull-request panes.
	SurfaceTickets        SurfaceKind = "tickets"        // SurfaceTickets groups a person's ticket and issue-tracker panes.
	SurfaceCommunications SurfaceKind = "communications" // SurfaceCommunications groups a person's message and thread panes.
)

const (
	TargetDocument    TargetKind = "document"     // TargetDocument means a pane links to Document rows.
	TargetPullRequest TargetKind = "pull_request" // TargetPullRequest means a pane links to PullRequest rows.
	TargetTicket      TargetKind = "ticket"       // TargetTicket means a pane links to Ticket rows.
	TargetMessage     TargetKind = "message"      // TargetMessage means a pane links to Message rows.
)

const (
	PaneDocumentsCreated       PaneKind = "documents_created"          // PaneDocumentsCreated contains documents created by the person.
	PaneDocumentsEdited        PaneKind = "documents_edited"           // PaneDocumentsEdited contains documents edited by the person.
	PaneDocumentsCommentedOn   PaneKind = "documents_commented_on"     // PaneDocumentsCommentedOn contains documents the person commented on.
	PanePullRequestsAuthored   PaneKind = "pull_requests_authored"     // PanePullRequestsAuthored contains pull requests authored by the person.
	PanePullRequestsReviewed   PaneKind = "pull_requests_reviewed"     // PanePullRequestsReviewed contains pull requests reviewed by the person.
	PanePullRequestsCommented  PaneKind = "pull_requests_commented_on" // PanePullRequestsCommented contains pull requests the person commented on.
	PaneTicketsOwned           PaneKind = "tickets_owned"              // PaneTicketsOwned contains tickets owned or assigned to the person.
	PaneTicketsReviewed        PaneKind = "tickets_reviewed"           // PaneTicketsReviewed contains tickets reviewed or triaged by the person.
	PaneTicketsMentionedIn     PaneKind = "tickets_mentioned_in"       // PaneTicketsMentionedIn contains tickets where the person was mentioned.
	PaneMessagesAuthored       PaneKind = "messages_authored"          // PaneMessagesAuthored contains messages authored by the person.
	PaneMessagesMentioningUser PaneKind = "messages_mentioning_person" // PaneMessagesMentioningUser contains messages that mention the person.
	PaneMessagesRepliedTo      PaneKind = "messages_replied_to"        // PaneMessagesRepliedTo contains message threads the person replied to.
)

const (
	RelationCreated        RelationKind = "created"         // RelationCreated means the source actor created the target object.
	RelationEdited         RelationKind = "edited"          // RelationEdited means the source actor edited the target object.
	RelationCommentedOn    RelationKind = "commented_on"    // RelationCommentedOn means the source actor commented on the target object.
	RelationAuthored       RelationKind = "authored"        // RelationAuthored means the source actor authored the target object.
	RelationReviewed       RelationKind = "reviewed"        // RelationReviewed means the source actor reviewed the target object.
	RelationOwned          RelationKind = "owned"           // RelationOwned means the source actor owns or is assigned to the target object.
	RelationMentionedIn    RelationKind = "mentioned_in"    // RelationMentionedIn means the source actor is mentioned in the target context.
	RelationMentionsPerson RelationKind = "mentions_person" // RelationMentionsPerson means the target message mentions the source person.
	RelationRepliedTo      RelationKind = "replied_to"      // RelationRepliedTo means the source actor replied to the target message thread.
)

// PaneDefinition is the canonical meaning of a WorkPane kind.
type PaneDefinition struct {
	PaneKind     PaneKind     // PaneKind is the specific bounded view being defined.
	SurfaceKind  SurfaceKind  // SurfaceKind is the owning work surface domain.
	TargetKind   TargetKind   // TargetKind is the only concrete object kind this pane may link to.
	RelationKind RelationKind // RelationKind is the semantic edge from this pane to its targets.
	DisplayName  string       // DisplayName is the short user-facing label for graph explorers.
}

// BuiltinPaneDefinitions returns the closed Cubicle pane vocabulary.
func BuiltinPaneDefinitions() []PaneDefinition {
	return []PaneDefinition{
		{PaneKind: PaneDocumentsCreated, SurfaceKind: SurfaceDocuments, TargetKind: TargetDocument, RelationKind: RelationCreated, DisplayName: "Documents Created"},
		{PaneKind: PaneDocumentsEdited, SurfaceKind: SurfaceDocuments, TargetKind: TargetDocument, RelationKind: RelationEdited, DisplayName: "Documents Edited"},
		{PaneKind: PaneDocumentsCommentedOn, SurfaceKind: SurfaceDocuments, TargetKind: TargetDocument, RelationKind: RelationCommentedOn, DisplayName: "Documents Commented On"},
		{PaneKind: PanePullRequestsAuthored, SurfaceKind: SurfaceCode, TargetKind: TargetPullRequest, RelationKind: RelationAuthored, DisplayName: "Pull Requests Authored"},
		{PaneKind: PanePullRequestsReviewed, SurfaceKind: SurfaceCode, TargetKind: TargetPullRequest, RelationKind: RelationReviewed, DisplayName: "Pull Requests Reviewed"},
		{PaneKind: PanePullRequestsCommented, SurfaceKind: SurfaceCode, TargetKind: TargetPullRequest, RelationKind: RelationCommentedOn, DisplayName: "Pull Requests Commented On"},
		{PaneKind: PaneTicketsOwned, SurfaceKind: SurfaceTickets, TargetKind: TargetTicket, RelationKind: RelationOwned, DisplayName: "Tickets Owned"},
		{PaneKind: PaneTicketsReviewed, SurfaceKind: SurfaceTickets, TargetKind: TargetTicket, RelationKind: RelationReviewed, DisplayName: "Tickets Reviewed"},
		{PaneKind: PaneTicketsMentionedIn, SurfaceKind: SurfaceTickets, TargetKind: TargetTicket, RelationKind: RelationMentionedIn, DisplayName: "Tickets Mentioned In"},
		{PaneKind: PaneMessagesAuthored, SurfaceKind: SurfaceCommunications, TargetKind: TargetMessage, RelationKind: RelationAuthored, DisplayName: "Messages Authored"},
		{PaneKind: PaneMessagesMentioningUser, SurfaceKind: SurfaceCommunications, TargetKind: TargetMessage, RelationKind: RelationMentionsPerson, DisplayName: "Messages Mentioning Person"},
		{PaneKind: PaneMessagesRepliedTo, SurfaceKind: SurfaceCommunications, TargetKind: TargetMessage, RelationKind: RelationRepliedTo, DisplayName: "Messages Replied To"},
	}
}

// PaneDefinitionFor returns the canonical definition for paneKind.
func PaneDefinitionFor(paneKind PaneKind) (PaneDefinition, bool) {
	for _, definition := range BuiltinPaneDefinitions() {
		if definition.PaneKind == paneKind {
			return definition, true
		}
	}
	return PaneDefinition{}, false
}

// ValidatePaneTargetKind verifies that a pane kind and target kind agree.
func ValidatePaneTargetKind(paneKind PaneKind, targetKind TargetKind) error {
	definition, ok := PaneDefinitionFor(paneKind)
	if !ok {
		return fmt.Errorf("unknown pane kind %q", paneKind)
	}
	if definition.TargetKind != targetKind {
		return fmt.Errorf("pane kind %q targets %q, not %q", paneKind, definition.TargetKind, targetKind)
	}
	return nil
}

// ValidatePanePlacement verifies that a WorkPane is attached to the surface and
// target kind implied by its pane kind.
func ValidatePanePlacement(paneKind PaneKind, surfaceKind SurfaceKind, targetKind TargetKind) error {
	definition, ok := PaneDefinitionFor(paneKind)
	if !ok {
		return fmt.Errorf("unknown pane kind %q", paneKind)
	}
	if definition.SurfaceKind != surfaceKind {
		return fmt.Errorf("pane kind %q belongs to surface %q, not %q", paneKind, definition.SurfaceKind, surfaceKind)
	}
	if definition.TargetKind != targetKind {
		return fmt.Errorf("pane kind %q targets %q, not %q", paneKind, definition.TargetKind, targetKind)
	}
	return nil
}

// ValidatePaneLink verifies that a pane target link table, pane target kind,
// and link relation all match the canonical pane definition.
func ValidatePaneLink(paneKind PaneKind, paneTargetKind TargetKind, linkTargetKind TargetKind, relationKind RelationKind) error {
	definition, ok := PaneDefinitionFor(paneKind)
	if !ok {
		return fmt.Errorf("unknown pane kind %q", paneKind)
	}
	if definition.TargetKind != paneTargetKind {
		return fmt.Errorf("pane kind %q targets %q, but pane row targets %q", paneKind, definition.TargetKind, paneTargetKind)
	}
	if paneTargetKind != linkTargetKind {
		return fmt.Errorf("pane kind %q targets %q, not link table target %q", paneKind, paneTargetKind, linkTargetKind)
	}
	if definition.RelationKind != relationKind {
		return fmt.Errorf("pane kind %q uses relation %q, not %q", paneKind, definition.RelationKind, relationKind)
	}
	return nil
}

// SurfaceKindStrings returns all closed surface-kind enum values.
func SurfaceKindStrings() []string {
	return []string{
		string(SurfaceDocuments),
		string(SurfaceCode),
		string(SurfaceTickets),
		string(SurfaceCommunications),
	}
}

// TargetKindStrings returns all closed target-kind enum values.
func TargetKindStrings() []string {
	return []string{
		string(TargetDocument),
		string(TargetPullRequest),
		string(TargetTicket),
		string(TargetMessage),
	}
}

// PaneKindStrings returns all closed pane-kind enum values.
func PaneKindStrings() []string {
	definitions := BuiltinPaneDefinitions()
	values := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		values = append(values, string(definition.PaneKind))
	}
	return values
}

// RelationKindStringsForTarget returns relation-kind enum values allowed by a
// target-specific pane link table.
func RelationKindStringsForTarget(targetKind TargetKind) []string {
	seen := make(map[RelationKind]bool)
	values := make([]string, 0)
	for _, definition := range BuiltinPaneDefinitions() {
		if definition.TargetKind != targetKind || seen[definition.RelationKind] {
			continue
		}
		seen[definition.RelationKind] = true
		values = append(values, string(definition.RelationKind))
	}
	return values
}
