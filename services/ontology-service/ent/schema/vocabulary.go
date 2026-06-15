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
	relationDocumentedBy   = "documented_by"                             // relationDocumentedBy means a document explains or supports a ticket.
	relationDiscussedIn    = "discussed_in"                              // relationDiscussedIn means a message discusses a ticket.
	relationLinksTo        = "links_to"                                  // relationLinksTo means a document references another document.
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
	deletionStatePresent = "present" // deletionStatePresent means the source object was last observed as present.
	deletionStateDeleted = "deleted" // deletionStateDeleted means the source object was deleted or tombstoned.
	deletionStateUnknown = "unknown" // deletionStateUnknown means Cubicle has not checked deletion state.
)

const (
	aclStateUnknown     = "unknown"     // aclStateUnknown means no source ACL state has been resolved yet.
	aclStateCurrent     = "current"     // aclStateCurrent means the row's ACL fields are current enough for reads.
	aclStateStale       = "stale"       // aclStateStale means the row should be permission-refreshed before broad display.
	aclStateBlocked     = "blocked"     // aclStateBlocked means the source denied permission metadata or content.
	aclStateUnavailable = "unavailable" // aclStateUnavailable means the source cannot provide ACL metadata.
)

const (
	sourceSyncStatusRunning     = "running"      // sourceSyncStatusRunning means a connector pass is open or not yet terminal.
	sourceSyncStatusComplete    = "complete"     // sourceSyncStatusComplete means the declared source scope was fully crawled.
	sourceSyncStatusPartial     = "partial"      // sourceSyncStatusPartial means Cubicle saw some data but not the whole declared scope.
	sourceSyncStatusFailed      = "failed"       // sourceSyncStatusFailed means the connector pass failed before producing reliable coverage.
	sourceSyncStatusRateLimited = "rate_limited" // sourceSyncStatusRateLimited means source API throttling prevented full coverage.
)

const (
	sourceSyncModeSnapshot      = "snapshot"       // sourceSyncModeSnapshot means the connector enumerated a declared source scope.
	sourceSyncModeIncremental   = "incremental"    // sourceSyncModeIncremental means the connector consumed changes since a checkpoint.
	sourceSyncModeFederatedLive = "federated_live" // sourceSyncModeFederatedLive means the source was queried live for this run/scope.
)

const (
	sourceCoverageUnknown      = "unknown"       // sourceCoverageUnknown means coverage semantics are not known.
	sourceCoverageExactScope   = "exact_scope"   // sourceCoverageExactScope means the declared scope can support absence claims if status is complete.
	sourceCoveragePartialScope = "partial_scope" // sourceCoveragePartialScope means only part of the declared scope was observed.
	sourceCoverageMetadataOnly = "metadata_only" // sourceCoverageMetadataOnly means source content was not fully captured.
	sourceCoverageIdentityOnly = "identity_only" // sourceCoverageIdentityOnly means only identities/keys were captured.
	sourceCoverageLiveOnly     = "live_only"     // sourceCoverageLiveOnly means data was query-time only and not durable coverage.
)

const (
	identityStatusActive  = "active"  // identityStatusActive means the source identity currently maps to the person.
	identityStatusAlias   = "alias"   // identityStatusAlias means the source identity is an alternate handle for the person.
	identityStatusRetired = "retired" // identityStatusRetired means the source identity is historical but still resolves.
	identityStatusMerged  = "merged"  // identityStatusMerged means the source identity was merged into another source identity.
	identityStatusDeleted = "deleted" // identityStatusDeleted means the source identity was deleted or tombstoned.
)

const (
	proofStateCurrent           = "current"            // proofStateCurrent means this proof locator is current and citeable.
	proofStateStale             = "stale"              // proofStateStale means the proof may no longer match current content.
	proofStateSuperseded        = "superseded"         // proofStateSuperseded means another Evidence row replaces this proof.
	proofStateDeleted           = "deleted"            // proofStateDeleted means the cited source span or object was deleted.
	proofStatePermissionBlocked = "permission_blocked" // proofStatePermissionBlocked means current permissions hide this proof.
	proofStateLocatorFailed     = "locator_failed"     // proofStateLocatorFailed means the source object remains but the exact locator no longer resolves.
)

const (
	claimKindObjectState  = "object_state" // claimKindObjectState means Evidence supports fields on a product object row.
	claimKindRelationship = "relationship" // claimKindRelationship means Evidence supports a typed relationship row.
	claimKindIdentity     = "identity"     // claimKindIdentity means Evidence supports person/source identity resolution.
	claimKindCandidate    = "candidate"    // claimKindCandidate means Evidence supports a non-graph candidate or unresolved item.
)

const (
	referenceURL        = "url"         // referenceURL means a URL was observed in source content.
	referenceIssueKey   = "issue_key"   // referenceIssueKey means a Jira/Linear-style issue key was observed.
	referencePRNumber   = "pr_number"   // referencePRNumber means a pull request number/reference was observed.
	referenceMention    = "mention"     // referenceMention means a user/person mention was observed.
	referenceCommit     = "commit"      // referenceCommit means a source-control commit reference was observed.
	referenceMessageRef = "message_ref" // referenceMessageRef means a message/thread reference was observed.
	referenceDocLink    = "doc_link"    // referenceDocLink means a document link/reference was observed.
)

const (
	referenceResolutionUnresolved        = "unresolved"         // referenceResolutionUnresolved means no target has been resolved yet.
	referenceResolutionResolved          = "resolved"           // referenceResolutionResolved means the reference resolved into a typed graph edge.
	referenceResolutionAmbiguous         = "ambiguous"          // referenceResolutionAmbiguous means multiple targets match.
	referenceResolutionPermissionBlocked = "permission_blocked" // referenceResolutionPermissionBlocked means target resolution exists but is hidden.
	referenceResolutionMissing           = "missing"            // referenceResolutionMissing means the referenced target was not found.
)

const (
	assignmentAssignee = "assignee" // assignmentAssignee means a person is assigned to a ticket.
	assignmentReporter = "reporter" // assignmentReporter means a person reported or requested a ticket.
	assignmentOwner    = "owner"    // assignmentOwner means a person owns a ticket or source work item.
)

const (
	authorshipAuthor  = "author"  // authorshipAuthor means the person is the source-declared author.
	authorshipCreator = "creator" // authorshipCreator means the person created the source object.
	authorshipEditor  = "editor"  // authorshipEditor means the person edited the source object.
	authorshipSender  = "sender"  // authorshipSender means the person sent a communication item.
)

const (
	reviewReviewer          = "reviewer"           // reviewReviewer means the person reviewed a pull request.
	reviewApprover          = "approver"           // reviewApprover means the person approved a pull request.
	reviewCommenter         = "commenter"          // reviewCommenter means the person commented during review.
	reviewRequestedReviewer = "requested_reviewer" // reviewRequestedReviewer means review was requested from the person.
)

const (
	mentionMentioned  = "mentioned"  // mentionMentioned means the person was mentioned in the source content.
	mentionReferenced = "referenced" // mentionReferenced means the content references the person or their identity.
	mentionRepliedTo  = "replied_to" // mentionRepliedTo means a message/thread is a reply to the person.
)

const (
	sourceIssueSeverityInfo    = "info"    // sourceIssueSeverityInfo means informational connector state.
	sourceIssueSeverityWarning = "warning" // sourceIssueSeverityWarning means degraded but not failed source coverage.
	sourceIssueSeverityError   = "error"   // sourceIssueSeverityError means the connector could not complete expected work.
)

func freshnessValues() []string {
	return []string{freshnessFresh, freshnessPartial, freshnessStale, freshnessUnknown}
}

func visibilityValues() []string {
	return []string{visibilityUnknown, visibilityPublic, visibilityPrivate, visibilityTeam, visibilityRestricted}
}

func deletionStateValues() []string {
	return []string{deletionStatePresent, deletionStateDeleted, deletionStateUnknown}
}

func aclStateValues() []string {
	return []string{aclStateUnknown, aclStateCurrent, aclStateStale, aclStateBlocked, aclStateUnavailable}
}

func sourceSyncStatusValues() []string {
	return []string{sourceSyncStatusRunning, sourceSyncStatusComplete, sourceSyncStatusPartial, sourceSyncStatusFailed, sourceSyncStatusRateLimited}
}

func sourceSyncModeValues() []string {
	return []string{sourceSyncModeSnapshot, sourceSyncModeIncremental, sourceSyncModeFederatedLive}
}

func sourceCoverageValues() []string {
	return []string{sourceCoverageUnknown, sourceCoverageExactScope, sourceCoveragePartialScope, sourceCoverageMetadataOnly, sourceCoverageIdentityOnly, sourceCoverageLiveOnly}
}

func proofStateValues() []string {
	return []string{proofStateCurrent, proofStateStale, proofStateSuperseded, proofStateDeleted, proofStatePermissionBlocked, proofStateLocatorFailed}
}

func claimKindValues() []string {
	return []string{claimKindObjectState, claimKindRelationship, claimKindIdentity, claimKindCandidate}
}

func referenceKindValues() []string {
	return []string{referenceURL, referenceIssueKey, referencePRNumber, referenceMention, referenceCommit, referenceMessageRef, referenceDocLink}
}

func referenceResolutionValues() []string {
	return []string{referenceResolutionUnresolved, referenceResolutionResolved, referenceResolutionAmbiguous, referenceResolutionPermissionBlocked, referenceResolutionMissing}
}

func identityStatusValues() []string {
	return []string{identityStatusActive, identityStatusAlias, identityStatusRetired, identityStatusMerged, identityStatusDeleted}
}

func assignmentKindValues() []string {
	return []string{assignmentAssignee, assignmentReporter, assignmentOwner}
}

func documentAuthorshipKindValues() []string {
	return []string{authorshipAuthor, authorshipCreator, authorshipEditor}
}

func messageAuthorshipKindValues() []string {
	return []string{authorshipAuthor, authorshipSender}
}

func pullRequestAuthorshipKindValues() []string {
	return []string{authorshipAuthor, authorshipCreator}
}

func reviewKindValues() []string {
	return []string{reviewReviewer, reviewApprover, reviewCommenter, reviewRequestedReviewer}
}

func mentionKindValues() []string {
	return []string{mentionMentioned, mentionReferenced, mentionRepliedTo}
}

func sourceIssueSeverityValues() []string {
	return []string{sourceIssueSeverityInfo, sourceIssueSeverityWarning, sourceIssueSeverityError}
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

func ticketDocumentRelationValues() []string {
	return []string{relationDocumentedBy}
}

func documentLinkRelationValues() []string {
	return []string{relationLinksTo}
}
