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
	workAreaDocuments      = string(ontology.WorkAreaDocuments)      // workAreaDocuments groups a person's document-related work lenses.
	workAreaCode           = string(ontology.WorkAreaCode)           // workAreaCode groups a person's pull-request and code-review lenses.
	workAreaTickets        = string(ontology.WorkAreaTickets)        // workAreaTickets groups a person's ticket and issue-tracker lenses.
	workAreaCommunications = string(ontology.WorkAreaCommunications) // workAreaCommunications groups a person's message and thread lenses.
)

const (
	lensTargetDocument    = string(ontology.LensTargetDocument)    // lensTargetDocument means a lens result points to Document rows.
	lensTargetPullRequest = string(ontology.LensTargetPullRequest) // lensTargetPullRequest means a lens result points to PullRequest rows.
	lensTargetTicket      = string(ontology.LensTargetTicket)      // lensTargetTicket means a lens result points to Ticket rows.
	lensTargetMessage     = string(ontology.LensTargetMessage)     // lensTargetMessage means a lens result points to Message rows.
)

const (
	lensWindowRecent     = string(ontology.LensWindowRecent)     // lensWindowRecent stores the ranked head of a lens.
	lensWindowTimeBucket = string(ontology.LensWindowTimeBucket) // lensWindowTimeBucket stores a time-bounded lens partition.
	lensWindowSource     = string(ontology.LensWindowSource)     // lensWindowSource stores a source-bounded lens partition.
)

const (
	workLensDocumentsCreated         = string(ontology.WorkLensDocumentsCreated)         // workLensDocumentsCreated contains documents created by the person.
	workLensDocumentsEdited          = string(ontology.WorkLensDocumentsEdited)          // workLensDocumentsEdited contains documents edited by the person.
	workLensDocumentsCommentedOn     = string(ontology.WorkLensDocumentsCommentedOn)     // workLensDocumentsCommentedOn contains documents the person commented on.
	workLensPullRequestsAuthored     = string(ontology.WorkLensPullRequestsAuthored)     // workLensPullRequestsAuthored contains pull requests authored by the person.
	workLensPullRequestsReviewed     = string(ontology.WorkLensPullRequestsReviewed)     // workLensPullRequestsReviewed contains pull requests reviewed by the person.
	workLensPullRequestsCommented    = string(ontology.WorkLensPullRequestsCommented)    // workLensPullRequestsCommented contains pull requests the person commented on.
	workLensTicketsOwned             = string(ontology.WorkLensTicketsOwned)             // workLensTicketsOwned contains tickets owned or assigned to the person.
	workLensTicketsReviewed          = string(ontology.WorkLensTicketsReviewed)          // workLensTicketsReviewed contains tickets reviewed or triaged by the person.
	workLensTicketsMentionedIn       = string(ontology.WorkLensTicketsMentionedIn)       // workLensTicketsMentionedIn contains tickets where the person was mentioned.
	workLensMessagesAuthored         = string(ontology.WorkLensMessagesAuthored)         // workLensMessagesAuthored contains messages authored by the person.
	workLensMessagesMentioningPerson = string(ontology.WorkLensMessagesMentioningPerson) // workLensMessagesMentioningPerson contains messages that mention the person.
	workLensMessagesRepliedTo        = string(ontology.WorkLensMessagesRepliedTo)        // workLensMessagesRepliedTo contains message threads the person replied to.
)

const (
	relationCreated        = string(ontology.WorkRelationCreated)        // relationCreated means the source actor created the target object.
	relationEdited         = string(ontology.WorkRelationEdited)         // relationEdited means the source actor edited the target object.
	relationCommentedOn    = string(ontology.WorkRelationCommentedOn)    // relationCommentedOn means the source actor commented on the target object.
	relationAuthored       = string(ontology.WorkRelationAuthored)       // relationAuthored means the source actor authored the target object.
	relationReviewed       = string(ontology.WorkRelationReviewed)       // relationReviewed means the source actor reviewed the target object.
	relationOwned          = string(ontology.WorkRelationOwned)          // relationOwned means the source actor owns or is assigned to the target object.
	relationMentionedIn    = string(ontology.WorkRelationMentionedIn)    // relationMentionedIn means the source actor is mentioned in the target context.
	relationMentionsPerson = string(ontology.WorkRelationMentionsPerson) // relationMentionsPerson means the target message mentions the source person.
	relationRepliedTo      = string(ontology.WorkRelationRepliedTo)      // relationRepliedTo means the source actor replied to the target message thread.
	relationContains       = "contains"                                  // relationContains means a parent work object contains a child work object.
	relationImplementedBy  = "implemented_by"                            // relationImplementedBy means a pull request implements a ticket.
	relationDocumentedBy   = "documented_by"                             // relationDocumentedBy means a document fragment explains or supports a ticket.
	relationDiscussedIn    = "discussed_in"                              // relationDiscussedIn means a message discusses a ticket.
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

const (
	sourceRunStatusRunning     = "running"      // sourceRunStatusRunning means a connector pass is open or not yet terminal.
	sourceRunStatusComplete    = "complete"     // sourceRunStatusComplete means the declared source scope was fully crawled.
	sourceRunStatusPartial     = "partial"      // sourceRunStatusPartial means Cubicle saw some data but not the whole declared scope.
	sourceRunStatusFailed      = "failed"       // sourceRunStatusFailed means the connector pass failed before producing reliable coverage.
	sourceRunStatusRateLimited = "rate_limited" // sourceRunStatusRateLimited means source API throttling prevented full coverage.
)

const (
	identityStatusActive  = "active"  // identityStatusActive means the source identity currently maps to the target object.
	identityStatusAlias   = "alias"   // identityStatusAlias means the source identity is an alternate name for the target object.
	identityStatusRetired = "retired" // identityStatusRetired means the source identity is historical but still resolves to the target.
	identityStatusMerged  = "merged"  // identityStatusMerged means the source identity was merged into another source identity.
	identityStatusDeleted = "deleted" // identityStatusDeleted means the source identity was deleted or tombstoned.
)

const (
	permissionPolicyUnknown = "unknown" // permissionPolicyUnknown means no source permission policy has been resolved yet.
)

func freshnessValues() []string {
	return []string{freshnessFresh, freshnessPartial, freshnessStale, freshnessUnknown}
}

func visibilityValues() []string {
	return []string{visibilityUnknown, visibilityPublic, visibilityPrivate, visibilityTeam, visibilityRestricted}
}

func sourceRunStatusValues() []string {
	return []string{sourceRunStatusRunning, sourceRunStatusComplete, sourceRunStatusPartial, sourceRunStatusFailed, sourceRunStatusRateLimited}
}

func identityStatusValues() []string {
	return []string{identityStatusActive, identityStatusAlias, identityStatusRetired, identityStatusMerged, identityStatusDeleted}
}

func workAreaKindValues() []string {
	return ontology.WorkAreaKindStrings()
}

func lensTargetKindValues() []string {
	return ontology.LensTargetKindStrings()
}

func lensWindowKindValues() []string {
	return ontology.LensWindowKindStrings()
}

func workLensKindValues() []string {
	return ontology.WorkLensKindStrings()
}

func documentRelationValues() []string {
	return ontology.WorkRelationKindStringsForTarget(ontology.LensTargetDocument)
}

func pullRequestRelationValues() []string {
	return ontology.WorkRelationKindStringsForTarget(ontology.LensTargetPullRequest)
}

func ticketRelationValues() []string {
	return ontology.WorkRelationKindStringsForTarget(ontology.LensTargetTicket)
}

func messageRelationValues() []string {
	return ontology.WorkRelationKindStringsForTarget(ontology.LensTargetMessage)
}
