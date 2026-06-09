package schema

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
	surfaceDocuments      = "documents"      // surfaceDocuments groups a person's document-related work panes.
	surfaceCode           = "code"           // surfaceCode groups a person's pull-request and code-review panes.
	surfaceTickets        = "tickets"        // surfaceTickets groups a person's ticket and issue-tracker panes.
	surfaceCommunications = "communications" // surfaceCommunications groups a person's message and thread panes.
)

const (
	targetDocument    = "document"     // targetDocument means a pane link points to Document rows.
	targetPullRequest = "pull_request" // targetPullRequest means a pane link points to PullRequest rows.
	targetTicket      = "ticket"       // targetTicket means a pane link points to Ticket rows.
	targetMessage     = "message"      // targetMessage means a pane link points to Message rows.
)

const (
	paneDocumentsCreated       = "documents_created"          // paneDocumentsCreated contains documents created by the person.
	paneDocumentsEdited        = "documents_edited"           // paneDocumentsEdited contains documents edited by the person.
	paneDocumentsCommentedOn   = "documents_commented_on"     // paneDocumentsCommentedOn contains documents the person commented on.
	panePullRequestsAuthored   = "pull_requests_authored"     // panePullRequestsAuthored contains pull requests authored by the person.
	panePullRequestsReviewed   = "pull_requests_reviewed"     // panePullRequestsReviewed contains pull requests reviewed by the person.
	panePullRequestsCommented  = "pull_requests_commented_on" // panePullRequestsCommented contains pull requests the person commented on.
	paneTicketsOwned           = "tickets_owned"              // paneTicketsOwned contains tickets owned or assigned to the person.
	paneTicketsReviewed        = "tickets_reviewed"           // paneTicketsReviewed contains tickets reviewed or triaged by the person.
	paneTicketsMentionedIn     = "tickets_mentioned_in"       // paneTicketsMentionedIn contains tickets where the person was mentioned.
	paneMessagesAuthored       = "messages_authored"          // paneMessagesAuthored contains messages authored by the person.
	paneMessagesMentioningUser = "messages_mentioning_person" // paneMessagesMentioningUser contains messages that mention the person.
	paneMessagesRepliedTo      = "messages_replied_to"        // paneMessagesRepliedTo contains message threads the person replied to.
)

const (
	relationCreated        = "created"         // relationCreated means the source actor created the target object.
	relationEdited         = "edited"          // relationEdited means the source actor edited the target object.
	relationCommentedOn    = "commented_on"    // relationCommentedOn means the source actor commented on the target object.
	relationAuthored       = "authored"        // relationAuthored means the source actor authored the target object.
	relationReviewed       = "reviewed"        // relationReviewed means the source actor reviewed the target object.
	relationOwned          = "owned"           // relationOwned means the source actor owns or is assigned to the target object.
	relationMentionedIn    = "mentioned_in"    // relationMentionedIn means the source actor is mentioned in the target context.
	relationMentionsPerson = "mentions_person" // relationMentionsPerson means the target message mentions the source person.
	relationRepliedTo      = "replied_to"      // relationRepliedTo means the source actor replied to the target message thread.
	relationContains       = "contains"        // relationContains means a parent work object contains a child work object.
	relationImplementedBy  = "implemented_by"  // relationImplementedBy means a pull request implements a ticket.
	relationDocumentedBy   = "documented_by"   // relationDocumentedBy means a document fragment explains or supports a ticket.
	relationDiscussedIn    = "discussed_in"    // relationDiscussedIn means a message discusses a ticket.
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
	return []string{surfaceDocuments, surfaceCode, surfaceTickets, surfaceCommunications}
}

func targetKindValues() []string {
	return []string{targetDocument, targetPullRequest, targetTicket, targetMessage}
}

func paneKindValues() []string {
	return []string{
		paneDocumentsCreated,
		paneDocumentsEdited,
		paneDocumentsCommentedOn,
		panePullRequestsAuthored,
		panePullRequestsReviewed,
		panePullRequestsCommented,
		paneTicketsOwned,
		paneTicketsReviewed,
		paneTicketsMentionedIn,
		paneMessagesAuthored,
		paneMessagesMentioningUser,
		paneMessagesRepliedTo,
	}
}

func documentRelationValues() []string {
	return []string{relationCreated, relationEdited, relationCommentedOn}
}

func pullRequestRelationValues() []string {
	return []string{relationAuthored, relationReviewed, relationCommentedOn}
}

func ticketRelationValues() []string {
	return []string{relationOwned, relationReviewed, relationMentionedIn}
}

func messageRelationValues() []string {
	return []string{relationAuthored, relationMentionsPerson, relationRepliedTo}
}
