package ontology

import "fmt"

// WorkAreaKind is the major bounded work domain owned by a person.
type WorkAreaKind string

// LensTargetKind is the concrete Ent object kind a WorkLens can link to.
type LensTargetKind string

// LensWindowKind is the partition strategy for a WorkLens result set.
type LensWindowKind string

// WorkLensKind is a specific bounded view inside a person's work area.
type WorkLensKind string

// WorkRelationKind is the semantic action represented by a lens result.
type WorkRelationKind string

const (
	WorkAreaDocuments      WorkAreaKind = "documents"      // WorkAreaDocuments groups a person's document-related lenses.
	WorkAreaCode           WorkAreaKind = "code"           // WorkAreaCode groups a person's code and pull-request lenses.
	WorkAreaTickets        WorkAreaKind = "tickets"        // WorkAreaTickets groups a person's ticket and issue-tracker lenses.
	WorkAreaCommunications WorkAreaKind = "communications" // WorkAreaCommunications groups a person's message and thread lenses.
)

const (
	LensTargetDocument    LensTargetKind = "document"     // LensTargetDocument means a lens points to Document rows.
	LensTargetPullRequest LensTargetKind = "pull_request" // LensTargetPullRequest means a lens points to PullRequest rows.
	LensTargetTicket      LensTargetKind = "ticket"       // LensTargetTicket means a lens points to Ticket rows.
	LensTargetMessage     LensTargetKind = "message"      // LensTargetMessage means a lens points to Message rows.
)

const (
	LensWindowRecent     LensWindowKind = "recent"      // LensWindowRecent contains the current ranked head of a lens.
	LensWindowTimeBucket LensWindowKind = "time_bucket" // LensWindowTimeBucket contains results bounded by source activity time.
	LensWindowSource     LensWindowKind = "source"      // LensWindowSource contains results contributed by one source system.
)

const (
	WorkLensDocumentsCreated         WorkLensKind = "documents_created"          // WorkLensDocumentsCreated contains documents created by the person.
	WorkLensDocumentsEdited          WorkLensKind = "documents_edited"           // WorkLensDocumentsEdited contains documents edited by the person.
	WorkLensDocumentsCommentedOn     WorkLensKind = "documents_commented_on"     // WorkLensDocumentsCommentedOn contains documents the person commented on.
	WorkLensPullRequestsAuthored     WorkLensKind = "pull_requests_authored"     // WorkLensPullRequestsAuthored contains pull requests authored by the person.
	WorkLensPullRequestsReviewed     WorkLensKind = "pull_requests_reviewed"     // WorkLensPullRequestsReviewed contains pull requests reviewed by the person.
	WorkLensPullRequestsCommented    WorkLensKind = "pull_requests_commented_on" // WorkLensPullRequestsCommented contains pull requests the person commented on.
	WorkLensTicketsOwned             WorkLensKind = "tickets_owned"              // WorkLensTicketsOwned contains tickets owned or assigned to the person.
	WorkLensTicketsReviewed          WorkLensKind = "tickets_reviewed"           // WorkLensTicketsReviewed contains tickets reviewed or triaged by the person.
	WorkLensTicketsMentionedIn       WorkLensKind = "tickets_mentioned_in"       // WorkLensTicketsMentionedIn contains tickets where the person was mentioned.
	WorkLensMessagesAuthored         WorkLensKind = "messages_authored"          // WorkLensMessagesAuthored contains messages authored by the person.
	WorkLensMessagesMentioningPerson WorkLensKind = "messages_mentioning_person" // WorkLensMessagesMentioningPerson contains messages that mention the person.
	WorkLensMessagesRepliedTo        WorkLensKind = "messages_replied_to"        // WorkLensMessagesRepliedTo contains message threads the person replied to.
)

const (
	WorkRelationCreated        WorkRelationKind = "created"         // WorkRelationCreated means the source actor created the target object.
	WorkRelationEdited         WorkRelationKind = "edited"          // WorkRelationEdited means the source actor edited the target object.
	WorkRelationCommentedOn    WorkRelationKind = "commented_on"    // WorkRelationCommentedOn means the source actor commented on the target object.
	WorkRelationAuthored       WorkRelationKind = "authored"        // WorkRelationAuthored means the source actor authored the target object.
	WorkRelationReviewed       WorkRelationKind = "reviewed"        // WorkRelationReviewed means the source actor reviewed the target object.
	WorkRelationOwned          WorkRelationKind = "owned"           // WorkRelationOwned means the source actor owns or is assigned to the target object.
	WorkRelationMentionedIn    WorkRelationKind = "mentioned_in"    // WorkRelationMentionedIn means the source actor is mentioned in the target context.
	WorkRelationMentionsPerson WorkRelationKind = "mentions_person" // WorkRelationMentionsPerson means the target message mentions the source person.
	WorkRelationRepliedTo      WorkRelationKind = "replied_to"      // WorkRelationRepliedTo means the source actor replied to the target message thread.
)

// WorkLensDefinition is the canonical meaning of a WorkLens kind.
type WorkLensDefinition struct {
	WorkLensKind     WorkLensKind     // WorkLensKind is the specific bounded view being defined.
	WorkAreaKind     WorkAreaKind     // WorkAreaKind is the owning work area domain.
	LensTargetKind   LensTargetKind   // LensTargetKind is the only concrete object kind this lens may link to.
	WorkRelationKind WorkRelationKind // WorkRelationKind is the semantic edge from this lens to its targets.
	DisplayName      string           // DisplayName is the short user-facing label for graph explorers.
}

// BuiltinWorkLensDefinitions returns the closed Cubicle work lens vocabulary.
func BuiltinWorkLensDefinitions() []WorkLensDefinition {
	return []WorkLensDefinition{
		{WorkLensKind: WorkLensDocumentsCreated, WorkAreaKind: WorkAreaDocuments, LensTargetKind: LensTargetDocument, WorkRelationKind: WorkRelationCreated, DisplayName: "Documents Created"},
		{WorkLensKind: WorkLensDocumentsEdited, WorkAreaKind: WorkAreaDocuments, LensTargetKind: LensTargetDocument, WorkRelationKind: WorkRelationEdited, DisplayName: "Documents Edited"},
		{WorkLensKind: WorkLensDocumentsCommentedOn, WorkAreaKind: WorkAreaDocuments, LensTargetKind: LensTargetDocument, WorkRelationKind: WorkRelationCommentedOn, DisplayName: "Documents Commented On"},
		{WorkLensKind: WorkLensPullRequestsAuthored, WorkAreaKind: WorkAreaCode, LensTargetKind: LensTargetPullRequest, WorkRelationKind: WorkRelationAuthored, DisplayName: "Pull Requests Authored"},
		{WorkLensKind: WorkLensPullRequestsReviewed, WorkAreaKind: WorkAreaCode, LensTargetKind: LensTargetPullRequest, WorkRelationKind: WorkRelationReviewed, DisplayName: "Pull Requests Reviewed"},
		{WorkLensKind: WorkLensPullRequestsCommented, WorkAreaKind: WorkAreaCode, LensTargetKind: LensTargetPullRequest, WorkRelationKind: WorkRelationCommentedOn, DisplayName: "Pull Requests Commented On"},
		{WorkLensKind: WorkLensTicketsOwned, WorkAreaKind: WorkAreaTickets, LensTargetKind: LensTargetTicket, WorkRelationKind: WorkRelationOwned, DisplayName: "Tickets Owned"},
		{WorkLensKind: WorkLensTicketsReviewed, WorkAreaKind: WorkAreaTickets, LensTargetKind: LensTargetTicket, WorkRelationKind: WorkRelationReviewed, DisplayName: "Tickets Reviewed"},
		{WorkLensKind: WorkLensTicketsMentionedIn, WorkAreaKind: WorkAreaTickets, LensTargetKind: LensTargetTicket, WorkRelationKind: WorkRelationMentionedIn, DisplayName: "Tickets Mentioned In"},
		{WorkLensKind: WorkLensMessagesAuthored, WorkAreaKind: WorkAreaCommunications, LensTargetKind: LensTargetMessage, WorkRelationKind: WorkRelationAuthored, DisplayName: "Messages Authored"},
		{WorkLensKind: WorkLensMessagesMentioningPerson, WorkAreaKind: WorkAreaCommunications, LensTargetKind: LensTargetMessage, WorkRelationKind: WorkRelationMentionsPerson, DisplayName: "Messages Mentioning Person"},
		{WorkLensKind: WorkLensMessagesRepliedTo, WorkAreaKind: WorkAreaCommunications, LensTargetKind: LensTargetMessage, WorkRelationKind: WorkRelationRepliedTo, DisplayName: "Messages Replied To"},
	}
}

// WorkLensDefinitionFor returns the canonical definition for lensKind.
func WorkLensDefinitionFor(lensKind WorkLensKind) (WorkLensDefinition, bool) {
	for _, definition := range BuiltinWorkLensDefinitions() {
		if definition.WorkLensKind == lensKind {
			return definition, true
		}
	}
	return WorkLensDefinition{}, false
}

// ValidateWorkLensTargetKind verifies that a work lens kind and lens target kind agree.
func ValidateWorkLensTargetKind(lensKind WorkLensKind, lensTargetKind LensTargetKind) error {
	definition, ok := WorkLensDefinitionFor(lensKind)
	if !ok {
		return fmt.Errorf("unknown work lens kind %q", lensKind)
	}
	if definition.LensTargetKind != lensTargetKind {
		return fmt.Errorf("work lens kind %q targets %q, not %q", lensKind, definition.LensTargetKind, lensTargetKind)
	}
	return nil
}

// ValidateWorkLensPlacement verifies that a WorkLens is attached to the work area and
// lens target kind implied by its work lens kind.
func ValidateWorkLensPlacement(lensKind WorkLensKind, workAreaKind WorkAreaKind, lensTargetKind LensTargetKind) error {
	definition, ok := WorkLensDefinitionFor(lensKind)
	if !ok {
		return fmt.Errorf("unknown work lens kind %q", lensKind)
	}
	if definition.WorkAreaKind != workAreaKind {
		return fmt.Errorf("work lens kind %q belongs to work area %q, not %q", lensKind, definition.WorkAreaKind, workAreaKind)
	}
	if definition.LensTargetKind != lensTargetKind {
		return fmt.Errorf("work lens kind %q targets %q, not %q", lensKind, definition.LensTargetKind, lensTargetKind)
	}
	return nil
}

// ValidateLensResult verifies that a lens result table, lens lens target kind,
// and result relation all match the canonical work lens definition.
func ValidateLensResult(lensKind WorkLensKind, lensTargetKind LensTargetKind, resultLensTargetKind LensTargetKind, relationKind WorkRelationKind) error {
	definition, ok := WorkLensDefinitionFor(lensKind)
	if !ok {
		return fmt.Errorf("unknown work lens kind %q", lensKind)
	}
	if definition.LensTargetKind != lensTargetKind {
		return fmt.Errorf("work lens kind %q targets %q, but lens row targets %q", lensKind, definition.LensTargetKind, lensTargetKind)
	}
	if lensTargetKind != resultLensTargetKind {
		return fmt.Errorf("work lens kind %q targets %q, not result table target %q", lensKind, lensTargetKind, resultLensTargetKind)
	}
	if definition.WorkRelationKind != relationKind {
		return fmt.Errorf("work lens kind %q uses relation %q, not %q", lensKind, definition.WorkRelationKind, relationKind)
	}
	return nil
}

// WorkAreaKindStrings returns all closed work-area-kind enum values.
func WorkAreaKindStrings() []string {
	return []string{
		string(WorkAreaDocuments),
		string(WorkAreaCode),
		string(WorkAreaTickets),
		string(WorkAreaCommunications),
	}
}

// LensTargetKindStrings returns all closed lens-target-kind enum values.
func LensTargetKindStrings() []string {
	return []string{
		string(LensTargetDocument),
		string(LensTargetPullRequest),
		string(LensTargetTicket),
		string(LensTargetMessage),
	}
}

// LensWindowKindStrings returns all closed lens-window-kind enum values.
func LensWindowKindStrings() []string {
	return []string{
		string(LensWindowRecent),
		string(LensWindowTimeBucket),
		string(LensWindowSource),
	}
}

// WorkLensKindStrings returns all closed work-lens-kind enum values.
func WorkLensKindStrings() []string {
	definitions := BuiltinWorkLensDefinitions()
	values := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		values = append(values, string(definition.WorkLensKind))
	}
	return values
}

// WorkRelationKindStringsForTarget returns work-relation-kind enum values allowed by a
// target-specific lens result table.
func WorkRelationKindStringsForTarget(lensTargetKind LensTargetKind) []string {
	seen := make(map[WorkRelationKind]bool)
	values := make([]string, 0)
	for _, definition := range BuiltinWorkLensDefinitions() {
		if definition.LensTargetKind != lensTargetKind || seen[definition.WorkRelationKind] {
			continue
		}
		seen[definition.WorkRelationKind] = true
		values = append(values, string(definition.WorkRelationKind))
	}
	return values
}
