package schema

import "cubicle/services/ontology-service/internal/ontology"

const (
	freshnessFresh   = "fresh"   // freshnessFresh means Cubicle believes the fact is current for its source.
	freshnessPartial = "partial" // freshnessPartial means at least one source gap or incomplete crawl affects the fact.
	freshnessStale   = "stale"   // freshnessStale means the fact is retained but should not be treated as current.
	freshnessUnknown = "unknown" // freshnessUnknown means Cubicle has not evaluated source freshness yet.
)

const (
	visibilityUnknown    = "unknown"    // visibilityUnknown means no explicit visibility decision has been recorded yet.
	visibilityPublic     = "public"     // visibilityPublic means the source fact is safe to show broadly.
	visibilityPrivate    = "private"    // visibilityPrivate means the source fact should be scoped to the owning user.
	visibilityTeam       = "team"       // visibilityTeam means the source fact should be scoped to a team or group.
	visibilityRestricted = "restricted" // visibilityRestricted means source permissions must be checked before display.
)

const (
	surfaceDocuments      = string(ontology.SurfaceDocuments)      // surfaceDocuments groups a person's document-related work panes.
	surfaceCode           = string(ontology.SurfaceCode)           // surfaceCode groups a person's pull-request and code-review panes.
	surfaceTickets        = string(ontology.SurfaceTickets)        // surfaceTickets groups a person's ticket and issue-tracker panes.
	surfaceCommunications = string(ontology.SurfaceCommunications) // surfaceCommunications groups a person's message and thread panes.
)

const (
	targetDocument    = string(ontology.TargetDocument)    // targetDocument means a pane link points to Document rows.
	targetPullRequest = string(ontology.TargetPullRequest) // targetPullRequest means a pane link points to PullRequest rows.
	targetTicket      = string(ontology.TargetTicket)      // targetTicket means a pane link points to Ticket rows.
	targetMessage     = string(ontology.TargetMessage)     // targetMessage means a pane link points to Message rows.
)

const (
	paneDocumentsCreated       = string(ontology.PaneDocumentsCreated)       // paneDocumentsCreated contains documents created by the person.
	paneDocumentsEdited        = string(ontology.PaneDocumentsEdited)        // paneDocumentsEdited contains documents edited by the person.
	paneDocumentsCommentedOn   = string(ontology.PaneDocumentsCommentedOn)   // paneDocumentsCommentedOn contains documents the person commented on.
	panePullRequestsAuthored   = string(ontology.PanePullRequestsAuthored)   // panePullRequestsAuthored contains pull requests authored by the person.
	panePullRequestsReviewed   = string(ontology.PanePullRequestsReviewed)   // panePullRequestsReviewed contains pull requests reviewed by the person.
	panePullRequestsCommented  = string(ontology.PanePullRequestsCommented)  // panePullRequestsCommented contains pull requests the person commented on.
	paneTicketsOwned           = string(ontology.PaneTicketsOwned)           // paneTicketsOwned contains tickets owned or assigned to the person.
	paneTicketsReviewed        = string(ontology.PaneTicketsReviewed)        // paneTicketsReviewed contains tickets reviewed or triaged by the person.
	paneTicketsMentionedIn     = string(ontology.PaneTicketsMentionedIn)     // paneTicketsMentionedIn contains tickets where the person was mentioned.
	paneMessagesAuthored       = string(ontology.PaneMessagesAuthored)       // paneMessagesAuthored contains messages authored by the person.
	paneMessagesMentioningUser = string(ontology.PaneMessagesMentioningUser) // paneMessagesMentioningUser contains messages that mention the person.
	paneMessagesRepliedTo      = string(ontology.PaneMessagesRepliedTo)      // paneMessagesRepliedTo contains message threads the person replied to.
)

const (
	relationCreated        = string(ontology.RelationCreated)        // relationCreated means the source actor created the target object.
	relationEdited         = string(ontology.RelationEdited)         // relationEdited means the source actor edited the target object.
	relationCommentedOn    = string(ontology.RelationCommentedOn)    // relationCommentedOn means the source actor commented on the target object.
	relationAuthored       = string(ontology.RelationAuthored)       // relationAuthored means the source actor authored the target object.
	relationReviewed       = string(ontology.RelationReviewed)       // relationReviewed means the source actor reviewed the target object.
	relationOwned          = string(ontology.RelationOwned)          // relationOwned means the source actor owns or is assigned to the target object.
	relationMentionedIn    = string(ontology.RelationMentionedIn)    // relationMentionedIn means the source actor is mentioned in the target context.
	relationMentionsPerson = string(ontology.RelationMentionsPerson) // relationMentionsPerson means the target message mentions the source person.
	relationRepliedTo      = string(ontology.RelationRepliedTo)      // relationRepliedTo means the source actor replied to the target message thread.
	relationContains       = "contains"                              // relationContains means a parent work object contains a child work object.
	relationImplementedBy  = "implemented_by"                        // relationImplementedBy means a pull request implements a ticket.
	relationDocumentedBy   = "documented_by"                         // relationDocumentedBy means a document fragment explains or supports a ticket.
	relationDiscussedIn    = "discussed_in"                          // relationDiscussedIn means a message discusses a ticket.
)

const (
	ticketStatusUnknown = "unknown" // ticketStatusUnknown means the source status was absent or unmapped.
	ticketStatusOpen    = "open"    // ticketStatusOpen means the ticket is active.
	ticketStatusClosed  = "closed"  // ticketStatusClosed means the ticket is complete or otherwise closed.
)

const (
	pullRequestStateUnknown = "unknown" // pullRequestStateUnknown means the source PR state was absent or unmapped.
	pullRequestStateOpen    = "open"    // pullRequestStateOpen means the pull request is open.
	pullRequestStateMerged  = "merged"  // pullRequestStateMerged means the pull request was merged.
	pullRequestStateClosed  = "closed"  // pullRequestStateClosed means the pull request closed without merge or with unknown merge state.
)

const (
	workstreamStatusUnknown = "unknown" // workstreamStatusUnknown means the workstream state was not evaluated.
	workstreamStatusActive  = "active"  // workstreamStatusActive means the workstream is actively moving.
	workstreamStatusPaused  = "paused"  // workstreamStatusPaused means the workstream is intentionally paused.
	workstreamStatusDone    = "done"    // workstreamStatusDone means the workstream is complete.
)

const (
	documentKindUnknown  = "unknown"    // documentKindUnknown means the source document type was not mapped.
	documentKindGoogle   = "google_doc" // documentKindGoogle means the document came from Google Docs.
	documentKindMarkdown = "markdown"   // documentKindMarkdown means the document is Markdown or repo-backed prose.
	documentKindSpec     = "spec"       // documentKindSpec means the document is an explicit design or requirements spec.
)

func freshnessValues() []string {
	return []string{freshnessFresh, freshnessPartial, freshnessStale, freshnessUnknown}
}

func visibilityValues() []string {
	return []string{visibilityUnknown, visibilityPublic, visibilityPrivate, visibilityTeam, visibilityRestricted}
}

func surfaceKindValues() []string {
	return ontology.SurfaceKindStrings()
}

func targetKindValues() []string {
	return ontology.TargetKindStrings()
}

func paneKindValues() []string {
	return ontology.PaneKindStrings()
}

func documentRelationValues() []string {
	return ontology.RelationKindStringsForTarget(ontology.TargetDocument)
}

func pullRequestRelationValues() []string {
	return ontology.RelationKindStringsForTarget(ontology.TargetPullRequest)
}

func ticketRelationValues() []string {
	return ontology.RelationKindStringsForTarget(ontology.TargetTicket)
}

func messageRelationValues() []string {
	return ontology.RelationKindStringsForTarget(ontology.TargetMessage)
}
