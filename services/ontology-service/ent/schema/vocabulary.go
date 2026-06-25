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
	workstreamOperatingUnknown            = "unknown"             // workstreamOperatingUnknown means no operating status was materialized.
	workstreamOperatingClear              = "clear"               // workstreamOperatingClear means no immediate TPM action is required.
	workstreamOperatingWatch              = "watch"               // workstreamOperatingWatch means the workstream should remain visible but does not need interruption.
	workstreamOperatingValidationRequired = "validation_required" // workstreamOperatingValidationRequired means generated leads need validation before product escalation.
	workstreamOperatingAttentionRequired  = "attention_required"  // workstreamOperatingAttentionRequired means the workstream needs active TPM follow-up.
)

const (
	workstreamStandupSectionTopAction        = "top_action"        // workstreamStandupSectionTopAction means an agenda item selected from the top action queue.
	workstreamStandupSectionProductAction    = "product_action"    // workstreamStandupSectionProductAction means the item is ready for product follow-up.
	workstreamStandupSectionValidationLead   = "validation_lead"   // workstreamStandupSectionValidationLead means the item needs validation before escalation.
	workstreamStandupSectionSourceRepair     = "source_repair"     // workstreamStandupSectionSourceRepair means source coverage must be repaired first.
	workstreamStandupSectionCloseoutReview   = "closeout_review"   // workstreamStandupSectionCloseoutReview means a terminal transition needs confirmation.
	workstreamStandupSectionModelOrRuleQA    = "model_or_rule_qa"  // workstreamStandupSectionModelOrRuleQA means model/rule quality should be reviewed.
	workstreamStandupSectionSuppressedSignal = "suppressed_signal" // workstreamStandupSectionSuppressedSignal means the agenda row documents a suppressed signal.
	workstreamStandupSectionModelQuality     = "model_quality"     // workstreamStandupSectionModelQuality means forecast/model quality gates should be discussed.
	workstreamStandupSectionOwnerLoad        = "owner_load"        // workstreamStandupSectionOwnerLoad means owner load should be reviewed.
	workstreamStandupSectionResolvedChange   = "resolved_change"   // workstreamStandupSectionResolvedChange means an observed terminal change needs closeout hygiene.
)

const (
	workstreamStandupUrgencyUnknown  = "unknown"  // workstreamStandupUrgencyUnknown means urgency was not evaluated.
	workstreamStandupUrgencyCritical = "critical" // workstreamStandupUrgencyCritical means the item should be handled first.
	workstreamStandupUrgencyHigh     = "high"     // workstreamStandupUrgencyHigh means the item should be handled in the next operating pass.
	workstreamStandupUrgencyMedium   = "medium"   // workstreamStandupUrgencyMedium means the item belongs in the standup but is not interruptive.
	workstreamStandupUrgencyLow      = "low"      // workstreamStandupUrgencyLow means the item is informational or watchlist material.
)

const (
	workOwnerLoadUnknown           = "unknown"            // workOwnerLoadUnknown means owner load was not evaluated.
	workOwnerLoadClear             = "clear"              // workOwnerLoadClear means the owner has no generated follow-up load.
	workOwnerLoadWatch             = "watch"              // workOwnerLoadWatch means the owner has low operating load worth tracking.
	workOwnerLoadAttentionRequired = "attention_required" // workOwnerLoadAttentionRequired means the owner has urgent or product-ready follow-up.
	workOwnerLoadOverloaded        = "overloaded"         // workOwnerLoadOverloaded means the owner has multiple generated actions and should be load-balanced.
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
	proofStateGenerated         = "generated"          // proofStateGenerated means this proof row records generated output, not source-native proof.
)

const (
	claimKindObjectState      = "object_state"      // claimKindObjectState means Evidence supports fields on a product object row.
	claimKindRelationship     = "relationship"      // claimKindRelationship means Evidence supports a typed relationship row.
	claimKindIdentity         = "identity"          // claimKindIdentity means Evidence supports person/source identity resolution.
	claimKindCandidate        = "candidate"         // claimKindCandidate means Evidence supports a non-graph candidate or unresolved item.
	claimKindGeneratedSummary = "generated_summary" // claimKindGeneratedSummary means Evidence records a generated cited summary artifact.
)

const (
	evidenceAttachmentCurrent    = "current"    // evidenceAttachmentCurrent means this attachment is the active proof for the claim.
	evidenceAttachmentCandidate  = "candidate"  // evidenceAttachmentCandidate means this proof has not earned product-claim use.
	evidenceAttachmentSuperseded = "superseded" // evidenceAttachmentSuperseded means a newer attachment replaces this proof link.
	evidenceAttachmentRejected   = "rejected"   // evidenceAttachmentRejected means review rejected this proof link.
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
	workResponsibilityAccountable     = "accountable"      // workResponsibilityAccountable means the party is accountable for moving the subject forward.
	workResponsibilityAssignee        = "assignee"         // workResponsibilityAssignee means the source assigned the subject to the party.
	workResponsibilityAuthor          = "author"           // workResponsibilityAuthor means the party authored or opened the subject.
	workResponsibilityReviewer        = "reviewer"         // workResponsibilityReviewer means review is expected from the party.
	workResponsibilityApprover        = "approver"         // workResponsibilityApprover means approval is expected from the party.
	workResponsibilityReporter        = "reporter"         // workResponsibilityReporter means the party reported or requested the subject.
	workResponsibilityCoordinator     = "coordinator"      // workResponsibilityCoordinator means the party coordinates work around the subject.
	workResponsibilityValidationOwner = "validation_owner" // workResponsibilityValidationOwner means the party should validate evidence or a generated claim.
	workResponsibilityObserver        = "observer"         // workResponsibilityObserver means the party is relevant context but not accountable.
)

const (
	workResponsibilitySubjectPullRequest  = "pull_request"               // workResponsibilitySubjectPullRequest points at a PullRequest subject.
	workResponsibilitySubjectTicket       = "ticket"                     // workResponsibilitySubjectTicket points at a Ticket subject.
	workResponsibilitySubjectWorkstream   = "workstream"                 // workResponsibilitySubjectWorkstream points at a Workstream subject.
	workResponsibilitySubjectAction       = "work_action"                // workResponsibilitySubjectAction points at a WorkAction operating subject.
	workResponsibilitySubjectBlocker      = "work_blocker"               // workResponsibilitySubjectBlocker points at a WorkBlocker operating subject.
	workResponsibilitySubjectEvidenceNeed = "work_program_evidence_need" // workResponsibilitySubjectEvidenceNeed points at a WorkProgramEvidenceNeed subject.
)

const (
	workResponsibilityPartyPerson     = "person"     // workResponsibilityPartyPerson means party_key resolves to a Person.
	workResponsibilityPartyTeam       = "team"       // workResponsibilityPartyTeam means party_key represents a team or group.
	workResponsibilityPartyUnresolved = "unresolved" // workResponsibilityPartyUnresolved means party_key did not resolve to a canonical identity.
	workResponsibilityPartyUnassigned = "unassigned" // workResponsibilityPartyUnassigned means no accountable party is known yet.
)

const (
	workResponsibilityBasisSourceNative        = "source_native"             // workResponsibilityBasisSourceNative means the source system directly supplied the responsibility.
	workResponsibilityBasisDerivedRelationship = "derived_from_relationship" // workResponsibilityBasisDerivedRelationship means Cubicle derived the row from another typed relationship.
	workResponsibilityBasisGeneratedCandidate  = "generated_candidate"       // workResponsibilityBasisGeneratedCandidate means analytics inferred a candidate responsibility.
	workResponsibilityBasisHumanOverride       = "human_override"            // workResponsibilityBasisHumanOverride means a person explicitly set or corrected the responsibility.
	workResponsibilityBasisImportedLabel       = "imported_label"            // workResponsibilityBasisImportedLabel means the responsibility came from an imported evaluation or label set.
)

const (
	workResponsibilityStateActive     = "active"     // workResponsibilityStateActive means the responsibility is current for product/accountability reads.
	workResponsibilityStateCandidate  = "candidate"  // workResponsibilityStateCandidate means the responsibility needs validation before product use.
	workResponsibilityStateSuperseded = "superseded" // workResponsibilityStateSuperseded means a newer responsibility replaced this row.
	workResponsibilityStateRejected   = "rejected"   // workResponsibilityStateRejected means a reviewer rejected the responsibility.
	workResponsibilityStateResolved   = "resolved"   // workResponsibilityStateResolved means the responsibility was completed or no longer active.
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

const (
	workInsightForecastRisk         = "forecast_risk"         // workInsightForecastRisk flags work likely to need TPM follow-up.
	workInsightBlockerCandidate     = "blocker_candidate"     // workInsightBlockerCandidate flags source text that may describe blocked work.
	workInsightDependencyCluster    = "dependency_cluster"    // workInsightDependencyCluster flags related work that should be coordinated together.
	workInsightStatusSummary        = "status_summary"        // workInsightStatusSummary summarizes current workstream state for review.
	workInsightDeveloperCorrelation = "developer_correlation" // workInsightDeveloperCorrelation flags same-window identity/workload overlap for capacity discussion only.
	workInsightModelQuality         = "model_quality"         // workInsightModelQuality records limits or health of a generated insight set.
	workInsightAIGraphBrief         = "ai_graph_brief"        // workInsightAIGraphBrief records a generated, cited graph traversal operating brief.
)

const (
	workInsightSeverityInfo     = "info"     // workInsightSeverityInfo is informational and usually does not need action.
	workInsightSeverityLow      = "low"      // workInsightSeverityLow is a weak or low-urgency signal.
	workInsightSeverityMedium   = "medium"   // workInsightSeverityMedium should be reviewed during normal planning.
	workInsightSeverityHigh     = "high"     // workInsightSeverityHigh should be reviewed soon by an owner or TPM.
	workInsightSeverityCritical = "critical" // workInsightSeverityCritical is a high-urgency signal that may block delivery.
)

const (
	workInsightProducerCurrent    = "current"    // workInsightProducerCurrent means the latest producer run reproduced this insight.
	workInsightProducerStale      = "stale"      // workInsightProducerStale means the latest producer run did not reproduce this insight.
	workInsightProducerSuperseded = "superseded" // workInsightProducerSuperseded means a newer generated insight replaced this row.
)

const (
	workInsightReviewKindTriageRequest   = "triage_request"   // workInsightReviewKindTriageRequest asks a TPM or owner to inspect a generated insight.
	workInsightReviewKindHumanAssessment = "human_assessment" // workInsightReviewKindHumanAssessment records a human judgment of an insight.
	workInsightReviewKindEvaluationLabel = "evaluation_label" // workInsightReviewKindEvaluationLabel records a gold or imported evaluation label.
)

const (
	workInsightReviewStateRequested     = "requested"       // workInsightReviewStateRequested means review is queued and not yet decided.
	workInsightReviewStateNeedsMoreData = "needs_more_data" // workInsightReviewStateNeedsMoreData means the reviewer could not decide from available evidence.
	workInsightReviewStateAccepted      = "accepted"        // workInsightReviewStateAccepted means the reviewer accepted the insight as useful.
	workInsightReviewStateDismissed     = "dismissed"       // workInsightReviewStateDismissed means the reviewer rejected the insight.
	workInsightReviewStateResolved      = "resolved"        // workInsightReviewStateResolved means follow-up was completed.
)

const (
	workInsightTruthUnknown       = "unknown"        // workInsightTruthUnknown means no truth label exists yet.
	workInsightTruthTruePositive  = "true_positive"  // workInsightTruthTruePositive means the generated insight matched the reviewed condition.
	workInsightTruthFalsePositive = "false_positive" // workInsightTruthFalsePositive means the generated insight did not match the reviewed condition.
	workInsightTruthPartial       = "partial"        // workInsightTruthPartial means the signal was partly correct but incomplete or overstated.
)

const (
	workInsightActionabilityUnknown       = "unknown"        // workInsightActionabilityUnknown means actionability has not been judged.
	workInsightActionabilityActionable    = "actionable"     // workInsightActionabilityActionable means a TPM or owner could act on the insight.
	workInsightActionabilityNotActionable = "not_actionable" // workInsightActionabilityNotActionable means the insight was not useful as presented.
	workInsightActionabilityNeedsOwner    = "needs_owner"    // workInsightActionabilityNeedsOwner means the next action is to identify an accountable owner.
)

const (
	workInsightReviewerSystem   = "system"   // workInsightReviewerSystem means the review row was seeded by Cubicle.
	workInsightReviewerHuman    = "human"    // workInsightReviewerHuman means a person supplied the review.
	workInsightReviewerImported = "imported" // workInsightReviewerImported means labels were imported from an external evaluation set.
)

const (
	workInsightLabelQualityUnknown   = "unknown"   // workInsightLabelQualityUnknown means no quality tier has been assigned to this review label.
	workInsightLabelQualityCandidate = "candidate" // workInsightLabelQualityCandidate means the label is useful for triage but not measurement gates.
	workInsightLabelQualityGold      = "gold"      // workInsightLabelQualityGold means the label is trusted for measurement gates.
	workInsightLabelQualitySmoke     = "smoke"     // workInsightLabelQualitySmoke means the label was created by a smoke-test or low-rigor pass.
)

const (
	workInsightSubjectUnknown     = "unknown"      // workInsightSubjectUnknown means the subject has not been resolved to a product kind.
	workInsightSubjectPullRequest = "pull_request" // workInsightSubjectPullRequest means the insight is about a PullRequest.
	workInsightSubjectTicket      = "ticket"       // workInsightSubjectTicket means the insight is about a Ticket.
)

const (
	workActionTypeDecisionOrOwnerFollowup = "decision_or_owner_followup" // workActionTypeDecisionOrOwnerFollowup asks for a merge, close, park, or owner decision.
	workActionTypeValidateSignal          = "validate_signal"            // workActionTypeValidateSignal asks for source or label validation before escalation.
	workActionTypeCICheckFollowup         = "ci_check_followup"          // workActionTypeCICheckFollowup asks for CI check triage.
	workActionTypeReviewWaitFollowup      = "review_wait_followup"       // workActionTypeReviewWaitFollowup asks for reviewer/approval follow-up.
	workActionTypeVerifyResolution        = "verify_resolution"          // workActionTypeVerifyResolution asks to confirm a terminal source transition.
	workActionTypeModelQualityReview      = "model_quality_review"       // workActionTypeModelQualityReview asks for model or rule quality review.
	workActionTypeRefreshSource           = "refresh_source"             // workActionTypeRefreshSource asks to repair source coverage before product claims.
	workActionTypeDismissedSignal         = "dismissed_signal"           // workActionTypeDismissedSignal records a suppressed or dismissed generated signal.
	workActionTypeClearBlocker            = "clear_blocker"              // workActionTypeClearBlocker asks to clear a confirmed blocker.
	workActionTypeCoordinateWorkstream    = "coordinate_workstream"      // workActionTypeCoordinateWorkstream asks to coordinate linked work.
	workActionTypeReviewInsight           = "review_insight"             // workActionTypeReviewInsight is a generic generated-insight review action.
)

const (
	workActionStateOpen       = "open"       // workActionStateOpen means the action is still active.
	workActionStateDecided    = "decided"    // workActionStateDecided means a TPM or gate made a decision, but closeout may remain.
	workActionStateClosed     = "closed"     // workActionStateClosed means no further follow-up is expected.
	workActionStateSuperseded = "superseded" // workActionStateSuperseded means a newer action replaced this action.
)

const (
	workActionDecisionProductAction  = "product_action"    // workActionDecisionProductAction means evidence and measurement gates support product escalation.
	workActionDecisionValidationLead = "validation_lead"   // workActionDecisionValidationLead means the row is a lead pending labels or source validation.
	workActionDecisionSourceRepair   = "source_repair"     // workActionDecisionSourceRepair means source coverage must be repaired first.
	workActionDecisionCloseoutReview = "closeout_review"   // workActionDecisionCloseoutReview means terminal-state closeout needs confirmation.
	workActionDecisionSourceResolved = "source_resolved"   // workActionDecisionSourceResolved means authenticated source state closed the follow-up.
	workActionDecisionModelOrRuleQA  = "model_or_rule_qa"  // workActionDecisionModelOrRuleQA means the row belongs to model or rule validation.
	workActionDecisionSuppressed     = "suppressed_signal" // workActionDecisionSuppressed means the signal was dismissed or suppressed from product follow-up.
	workActionDecisionPendingReview  = "pending_review"    // workActionDecisionPendingReview means the action has not been classified by gates yet.
)

const (
	workActionDueNow         = "now"         // workActionDueNow means the action should be reviewed immediately.
	workActionDueThisWeek    = "this_week"   // workActionDueThisWeek means the action belongs in the current planning window.
	workActionDueWatch       = "watch"       // workActionDueWatch means the action should remain visible but not interrupt active triage.
	workActionDueUnscheduled = "unscheduled" // workActionDueUnscheduled means no due bucket has been assigned.
)

const (
	workProgramStatusUnknown             = "unknown"               // workProgramStatusUnknown means the register status was not classified.
	workProgramStatusNeedsDecision       = "needs_decision"        // workProgramStatusNeedsDecision means a product/owner decision is required.
	workProgramStatusValidateSignal      = "validate_signal"       // workProgramStatusValidateSignal means source or label validation is needed before escalation.
	workProgramStatusCIFailing           = "ci_failing"            // workProgramStatusCIFailing means CI needs triage before product action.
	workProgramStatusWaitingReview       = "waiting_review"        // workProgramStatusWaitingReview means reviewer or approval routing is the next TPM step.
	workProgramStatusSourceRepair        = "source_repair"         // workProgramStatusSourceRepair means source coverage must be repaired first.
	workProgramStatusClosedPendingReview = "closed_pending_review" // workProgramStatusClosedPendingReview means a terminal transition needs closeout review.
	workProgramStatusModelQuality        = "model_quality"         // workProgramStatusModelQuality means model/rule quality gates need attention.
	workProgramStatusDismissed           = "dismissed"             // workProgramStatusDismissed means the signal was suppressed or dismissed.
	workProgramStatusClosureCandidate    = "closure_candidate"     // workProgramStatusClosureCandidate means the item is ready for blocker/closure follow-up.
	workProgramStatusNeedsReview         = "needs_review"          // workProgramStatusNeedsReview means no more specific register status was assigned.
)

const (
	workProgramBucketUnknown        = "unknown"         // workProgramBucketUnknown means no TPM bucket was assigned.
	workProgramBucketRisk           = "risk"            // workProgramBucketRisk means decision or risk follow-up.
	workProgramBucketRiskValidation = "risk_validation" // workProgramBucketRiskValidation means risk evidence needs validation.
	workProgramBucketBlocker        = "blocker"         // workProgramBucketBlocker means blocker validation or clearing.
	workProgramBucketCI             = "ci"              // workProgramBucketCI means CI/check follow-up.
	workProgramBucketCIValidation   = "ci_validation"   // workProgramBucketCIValidation means CI evidence needs validation.
	workProgramBucketReviewerWait   = "reviewer_wait"   // workProgramBucketReviewerWait means reviewer or approval routing.
	workProgramBucketClosure        = "closure"         // workProgramBucketClosure means closeout or blocker-closure follow-up.
	workProgramBucketDismissal      = "dismissal"       // workProgramBucketDismissal means dismissed/suppressed signal tracking.
	workProgramBucketModelQuality   = "model_quality"   // workProgramBucketModelQuality means model/rule quality review.
	workProgramBucketSourceRepair   = "source_repair"   // workProgramBucketSourceRepair means source coverage repair.
	workProgramBucketReview         = "review"          // workProgramBucketReview means generic review work.
)

const (
	workProgramMilestoneReleaseTarget     = "release_target"     // workProgramMilestoneReleaseTarget records a source release or fix-version target signal.
	workProgramMilestoneExplicitDueDate   = "explicit_due_date"  // workProgramMilestoneExplicitDueDate records a source-native due date.
	workProgramMilestoneResolutionOutcome = "resolution_outcome" // workProgramMilestoneResolutionOutcome records a resolved or closed source outcome date.
)

const (
	workProgramMilestoneStateUnknown              = "unknown"                // workProgramMilestoneStateUnknown means no milestone state was classified.
	workProgramMilestoneStatePlanned              = "planned"                // workProgramMilestoneStatePlanned means the target date is still ahead.
	workProgramMilestoneStatePastTargetUnresolved = "past_target_unresolved" // workProgramMilestoneStatePastTargetUnresolved means the target date passed and no source resolution was observed.
	workProgramMilestoneStateResolvedBeforeTarget = "resolved_before_target" // workProgramMilestoneStateResolvedBeforeTarget means source resolution happened on or before the target.
	workProgramMilestoneStateResolvedAfterTarget  = "resolved_after_target"  // workProgramMilestoneStateResolvedAfterTarget means source resolution happened after the target.
	workProgramMilestoneStateNoTargetDate         = "no_target_date"         // workProgramMilestoneStateNoTargetDate means the source gives a milestone name but no date.
	workProgramMilestoneStateOutcomeOnly          = "outcome_only"           // workProgramMilestoneStateOutcomeOnly means only a terminal outcome date is known.
)

const (
	workProgramMilestoneCommitmentUnknown            = "unknown"             // workProgramMilestoneCommitmentUnknown means commitment strength was not classified.
	workProgramMilestoneCommitmentReleaseSignal      = "release_signal"      // workProgramMilestoneCommitmentReleaseSignal means a release assignment exists but is not an explicit due-date promise.
	workProgramMilestoneCommitmentExplicitCommitment = "explicit_commitment" // workProgramMilestoneCommitmentExplicitCommitment means the source supplied an explicit due date.
	workProgramMilestoneCommitmentOutcomeEvidence    = "outcome_evidence"    // workProgramMilestoneCommitmentOutcomeEvidence means the row records observed completion timing only.
)

const (
	workProgramAdversarialCheckStatePass    = "pass"    // workProgramAdversarialCheckStatePass means the claim is supported for current product use.
	workProgramAdversarialCheckStateWarning = "warning" // workProgramAdversarialCheckStateWarning means the claim is usable only with human-visible caveats.
	workProgramAdversarialCheckStateFail    = "fail"    // workProgramAdversarialCheckStateFail means the claim must not be automated or presented as true.
)

const (
	workActionObservationSourceState      = "source_state"      // workActionObservationSourceState records current source lifecycle or follow-up state.
	workActionObservationCISignal         = "ci_signal"         // workActionObservationCISignal records current CI status/check evidence.
	workActionObservationModelOrRuleQA    = "model_or_rule_qa"  // workActionObservationModelOrRuleQA records model or rule-readiness evidence.
	workActionObservationSuppressedSignal = "suppressed_signal" // workActionObservationSuppressedSignal records dismissal/suppression evidence.
	workActionObservationSourceRepair     = "source_repair"     // workActionObservationSourceRepair records a source-coverage repair need.
	workActionObservationCloseoutReview   = "closeout_review"   // workActionObservationCloseoutReview records a terminal transition closeout signal.
)

const (
	workBlockerKindSourceSignal = "source_signal" // workBlockerKindSourceSignal means source text or status evidence indicates blocked work.
	workBlockerKindDependency   = "dependency"    // workBlockerKindDependency means another work item or relationship is blocking progress.
	workBlockerKindDecision     = "decision"      // workBlockerKindDecision means progress needs an explicit decision.
	workBlockerKindCI           = "ci"            // workBlockerKindCI means CI or required checks are blocking or need validation.
	workBlockerKindReview       = "review"        // workBlockerKindReview means review ownership or approval is blocking progress.
)

const (
	workBlockerStateUnknown    = "unknown"    // workBlockerStateUnknown means Cubicle has not classified current blocker state.
	workBlockerStateActive     = "active"     // workBlockerStateActive means this blocker is currently believed to block progress.
	workBlockerStateValidating = "validating" // workBlockerStateValidating means this blocker candidate needs more validation before escalation.
	workBlockerStateResolved   = "resolved"   // workBlockerStateResolved means the blocker appears cleared but may need closeout.
	workBlockerStateDismissed  = "dismissed"  // workBlockerStateDismissed means the candidate was reviewed and rejected or suppressed.
)

const (
	workDependencyEdgeTicketPR          = "ticket_pr"          // workDependencyEdgeTicketPR connects a ticket to an implementing pull request.
	workDependencyEdgeWorkstreamMember  = "workstream_member"  // workDependencyEdgeWorkstreamMember connects a workstream to an included work item.
	workDependencyEdgeWorkstreamCluster = "workstream_cluster" // workDependencyEdgeWorkstreamCluster connects a derived component to its member work items.
	workDependencyEdgeBlockedBy         = "blocked_by"         // workDependencyEdgeBlockedBy connects work to an active blocker.
	workDependencyEdgeNeedsAction       = "needs_action"       // workDependencyEdgeNeedsAction connects a blocker to its operating action.
	workDependencyEdgeRelatedWork       = "related_work"       // workDependencyEdgeRelatedWork connects work items that should be coordinated.
)

const (
	workDependencyRelationshipOperatingProjection = "operating_projection" // workDependencyRelationshipOperatingProjection means this edge is a generated TPM topology projection.
	workDependencyRelationshipCanonicalMirror     = "canonical_mirror"     // workDependencyRelationshipCanonicalMirror means this edge mirrors a first-class typed relationship row.
)

const (
	workDependencyCanonicalRelationshipTicketPullRequest = "ticket_pull_request" // workDependencyCanonicalRelationshipTicketPullRequest means the edge mirrors a TicketPullRequest row.
)

const (
	workDependencyNodeWorkstream  = "workstream"   // workDependencyNodeWorkstream identifies a Workstream endpoint.
	workDependencyNodeTicket      = "ticket"       // workDependencyNodeTicket identifies a Ticket endpoint.
	workDependencyNodePullRequest = "pull_request" // workDependencyNodePullRequest identifies a PullRequest endpoint.
	workDependencyNodeBlocker     = "blocker"      // workDependencyNodeBlocker identifies a WorkBlocker endpoint.
	workDependencyNodeAction      = "action"       // workDependencyNodeAction identifies a WorkAction endpoint.
	workDependencyNodeComponent   = "component"    // workDependencyNodeComponent identifies an analytics component boundary.
)

const (
	workForecastEvaluationSummary              = "summary"                  // workForecastEvaluationSummary stores the current forecast gate decision and aggregate metrics.
	workForecastEvaluationKFold                = "kfold"                    // workForecastEvaluationKFold stores a cross-validation fold result.
	workForecastEvaluationChronologicalHoldout = "chronological_holdout"    // workForecastEvaluationChronologicalHoldout stores a time-ordered holdout result.
	workForecastEvaluationSourceEventKFold     = "source_event_as_of_kfold" // workForecastEvaluationSourceEventKFold stores source-event replay K-fold rows.
	workForecastEvaluationSourceEventHoldout   = "source_event_as_of_chronological_holdout"
	workForecastEvaluationLifecycleAsOf        = "lifecycle_as_of_baseline" // workForecastEvaluationLifecycleAsOf stores lifecycle-derived as-of baseline rows that do not clear ETA readiness.
	workForecastEvaluationSurvivalTimeToMerge  = "survival_time_to_merge"   // workForecastEvaluationSurvivalTimeToMerge stores censored time-to-merge survival baseline rows.
	workForecastEvaluationInsufficientSample   = "insufficient_sample"      // workForecastEvaluationInsufficientSample records that no forecast backtest can be trusted yet.
)

const (
	workForecastReadinessUnknown            = "unknown"             // workForecastReadinessUnknown means no forecast evaluation has been materialized.
	workForecastReadinessInsufficientSample = "insufficient_sample" // workForecastReadinessInsufficientSample means there is not enough history to evaluate ETA quality.
	workForecastReadinessGated              = "gated"               // workForecastReadinessGated means forecasts are useful for risk triage but not ETA commitments.
	workForecastReadinessReady              = "ready"               // workForecastReadinessReady means typed backtests support ETA-style product use.
)

const (
	workForecastKindCycleTime = "cycle_time" // workForecastKindCycleTime estimates total and remaining cycle time for source work.
)

const (
	workForecastRiskUnknown  = "unknown"  // workForecastRiskUnknown means no risk band was assigned.
	workForecastRiskLow      = "low"      // workForecastRiskLow means the forecast does not currently need TPM attention.
	workForecastRiskMedium   = "medium"   // workForecastRiskMedium means the forecast should remain visible.
	workForecastRiskHigh     = "high"     // workForecastRiskHigh means the forecast is an active TPM triage candidate.
	workForecastRiskCritical = "critical" // workForecastRiskCritical means the forecast should interrupt normal review order.
)

const (
	workItemTransitionStateChange         = "state_change"          // workItemTransitionStateChange records a non-terminal lifecycle change.
	workItemTransitionTerminalStateChange = "terminal_state_change" // workItemTransitionTerminalStateChange records a transition into merged, closed, resolved, or done.
	workItemTransitionCoverageStateChange = "coverage_state_change" // workItemTransitionCoverageStateChange records a source coverage/detail change without a state change.
	workItemTransitionStateRefresh        = "state_refresh"         // workItemTransitionStateRefresh records a refresh event retained for audit.
)

const (
	workItemTransitionConfidenceBasisUnknown                   = "unknown"                     // workItemTransitionConfidenceBasisUnknown means the basis was not recorded.
	workItemTransitionConfidenceBasisAdjacentSnapshotDetection = "adjacent_snapshot_detection" // workItemTransitionConfidenceBasisAdjacentSnapshotDetection means confidence scores the detector over adjacent state snapshots, not product truth.
	workItemTransitionConfidenceBasisSourceVerified            = "source_verified"             // workItemTransitionConfidenceBasisSourceVerified means the transition is backed by source-native lifecycle evidence.
	workItemTransitionConfidenceBasisHumanVerified             = "human_verified"              // workItemTransitionConfidenceBasisHumanVerified means a reviewer confirmed the transition outcome.
)

const (
	workItemTransitionVerificationCandidate        = "candidate"         // workItemTransitionVerificationCandidate means the transition is an unreviewed candidate.
	workItemTransitionVerificationCloseoutRequired = "closeout_required" // workItemTransitionVerificationCloseoutRequired means terminal work needs TPM closeout confirmation.
	workItemTransitionVerificationSourceVerified   = "source_verified"   // workItemTransitionVerificationSourceVerified means source-native lifecycle evidence confirms the transition.
	workItemTransitionVerificationHumanVerified    = "human_verified"    // workItemTransitionVerificationHumanVerified means a reviewer confirmed the transition.
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
	return []string{proofStateCurrent, proofStateStale, proofStateSuperseded, proofStateDeleted, proofStatePermissionBlocked, proofStateLocatorFailed, proofStateGenerated}
}

func claimKindValues() []string {
	return []string{claimKindObjectState, claimKindRelationship, claimKindIdentity, claimKindCandidate, claimKindGeneratedSummary}
}

func evidenceAttachmentStateValues() []string {
	return []string{evidenceAttachmentCurrent, evidenceAttachmentCandidate, evidenceAttachmentSuperseded, evidenceAttachmentRejected}
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

func workResponsibilityKindValues() []string {
	return []string{
		workResponsibilityAccountable,
		workResponsibilityAssignee,
		workResponsibilityAuthor,
		workResponsibilityReviewer,
		workResponsibilityApprover,
		workResponsibilityReporter,
		workResponsibilityCoordinator,
		workResponsibilityValidationOwner,
		workResponsibilityObserver,
	}
}

func workResponsibilitySubjectKindValues() []string {
	return []string{
		workResponsibilitySubjectPullRequest,
		workResponsibilitySubjectTicket,
		workResponsibilitySubjectWorkstream,
		workResponsibilitySubjectAction,
		workResponsibilitySubjectBlocker,
		workResponsibilitySubjectEvidenceNeed,
	}
}

func workResponsibilityPartyKindValues() []string {
	return []string{
		workResponsibilityPartyPerson,
		workResponsibilityPartyTeam,
		workResponsibilityPartyUnresolved,
		workResponsibilityPartyUnassigned,
	}
}

func workResponsibilityBasisKindValues() []string {
	return []string{
		workResponsibilityBasisSourceNative,
		workResponsibilityBasisDerivedRelationship,
		workResponsibilityBasisGeneratedCandidate,
		workResponsibilityBasisHumanOverride,
		workResponsibilityBasisImportedLabel,
	}
}

func workResponsibilityStateValues() []string {
	return []string{
		workResponsibilityStateActive,
		workResponsibilityStateCandidate,
		workResponsibilityStateSuperseded,
		workResponsibilityStateRejected,
		workResponsibilityStateResolved,
	}
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

func workInsightKindValues() []string {
	return []string{
		workInsightForecastRisk,
		workInsightBlockerCandidate,
		workInsightDependencyCluster,
		workInsightStatusSummary,
		workInsightDeveloperCorrelation,
		workInsightModelQuality,
		workInsightAIGraphBrief,
	}
}

func workInsightSeverityValues() []string {
	return []string{workInsightSeverityInfo, workInsightSeverityLow, workInsightSeverityMedium, workInsightSeverityHigh, workInsightSeverityCritical}
}

func workInsightProducerStateValues() []string {
	return []string{workInsightProducerCurrent, workInsightProducerStale, workInsightProducerSuperseded}
}

func workInsightReviewKindValues() []string {
	return []string{workInsightReviewKindTriageRequest, workInsightReviewKindHumanAssessment, workInsightReviewKindEvaluationLabel}
}

func workInsightReviewStateValues() []string {
	return []string{workInsightReviewStateRequested, workInsightReviewStateNeedsMoreData, workInsightReviewStateAccepted, workInsightReviewStateDismissed, workInsightReviewStateResolved}
}

func workInsightTruthValues() []string {
	return []string{workInsightTruthUnknown, workInsightTruthTruePositive, workInsightTruthFalsePositive, workInsightTruthPartial}
}

func workInsightActionabilityValues() []string {
	return []string{workInsightActionabilityUnknown, workInsightActionabilityActionable, workInsightActionabilityNotActionable, workInsightActionabilityNeedsOwner}
}

func workInsightReviewerKindValues() []string {
	return []string{workInsightReviewerSystem, workInsightReviewerHuman, workInsightReviewerImported}
}

func workInsightLabelQualityValues() []string {
	return []string{workInsightLabelQualityUnknown, workInsightLabelQualityCandidate, workInsightLabelQualityGold, workInsightLabelQualitySmoke}
}

func workInsightSubjectKindValues() []string {
	return []string{workInsightSubjectUnknown, workInsightSubjectPullRequest, workInsightSubjectTicket}
}

func workActionTypeValues() []string {
	return []string{
		workActionTypeDecisionOrOwnerFollowup,
		workActionTypeValidateSignal,
		workActionTypeCICheckFollowup,
		workActionTypeReviewWaitFollowup,
		workActionTypeVerifyResolution,
		workActionTypeModelQualityReview,
		workActionTypeRefreshSource,
		workActionTypeDismissedSignal,
		workActionTypeClearBlocker,
		workActionTypeCoordinateWorkstream,
		workActionTypeReviewInsight,
	}
}

func workActionStateValues() []string {
	return []string{workActionStateOpen, workActionStateDecided, workActionStateClosed, workActionStateSuperseded}
}

func workActionDecisionStateValues() []string {
	return []string{
		workActionDecisionProductAction,
		workActionDecisionValidationLead,
		workActionDecisionSourceRepair,
		workActionDecisionCloseoutReview,
		workActionDecisionSourceResolved,
		workActionDecisionModelOrRuleQA,
		workActionDecisionSuppressed,
		workActionDecisionPendingReview,
	}
}

func workActionDueBucketValues() []string {
	return []string{workActionDueNow, workActionDueThisWeek, workActionDueWatch, workActionDueUnscheduled}
}

func workProgramStatusValues() []string {
	return []string{
		workProgramStatusUnknown,
		workProgramStatusNeedsDecision,
		workProgramStatusValidateSignal,
		workProgramStatusCIFailing,
		workProgramStatusWaitingReview,
		workProgramStatusSourceRepair,
		workProgramStatusClosedPendingReview,
		workProgramStatusModelQuality,
		workProgramStatusDismissed,
		workProgramStatusClosureCandidate,
		workProgramStatusNeedsReview,
	}
}

func workProgramBucketValues() []string {
	return []string{
		workProgramBucketUnknown,
		workProgramBucketRisk,
		workProgramBucketRiskValidation,
		workProgramBucketBlocker,
		workProgramBucketCI,
		workProgramBucketCIValidation,
		workProgramBucketReviewerWait,
		workProgramBucketClosure,
		workProgramBucketDismissal,
		workProgramBucketModelQuality,
		workProgramBucketSourceRepair,
		workProgramBucketReview,
	}
}

func workProgramMilestoneKindValues() []string {
	return []string{
		workProgramMilestoneReleaseTarget,
		workProgramMilestoneExplicitDueDate,
		workProgramMilestoneResolutionOutcome,
	}
}

func workProgramMilestoneStateValues() []string {
	return []string{
		workProgramMilestoneStateUnknown,
		workProgramMilestoneStatePlanned,
		workProgramMilestoneStatePastTargetUnresolved,
		workProgramMilestoneStateResolvedBeforeTarget,
		workProgramMilestoneStateResolvedAfterTarget,
		workProgramMilestoneStateNoTargetDate,
		workProgramMilestoneStateOutcomeOnly,
	}
}

func workProgramMilestoneCommitmentStrengthValues() []string {
	return []string{
		workProgramMilestoneCommitmentUnknown,
		workProgramMilestoneCommitmentReleaseSignal,
		workProgramMilestoneCommitmentExplicitCommitment,
		workProgramMilestoneCommitmentOutcomeEvidence,
	}
}

func workProgramAdversarialCheckStateValues() []string {
	return []string{workProgramAdversarialCheckStatePass, workProgramAdversarialCheckStateWarning, workProgramAdversarialCheckStateFail}
}

func workActionObservationKindValues() []string {
	return []string{
		workActionObservationSourceState,
		workActionObservationCISignal,
		workActionObservationModelOrRuleQA,
		workActionObservationSuppressedSignal,
		workActionObservationSourceRepair,
		workActionObservationCloseoutReview,
	}
}

func workBlockerKindValues() []string {
	return []string{
		workBlockerKindSourceSignal,
		workBlockerKindDependency,
		workBlockerKindDecision,
		workBlockerKindCI,
		workBlockerKindReview,
	}
}

func workBlockerStateValues() []string {
	return []string{
		workBlockerStateUnknown,
		workBlockerStateActive,
		workBlockerStateValidating,
		workBlockerStateResolved,
		workBlockerStateDismissed,
	}
}

func workDependencyEdgeKindValues() []string {
	return []string{
		workDependencyEdgeTicketPR,
		workDependencyEdgeWorkstreamMember,
		workDependencyEdgeWorkstreamCluster,
		workDependencyEdgeBlockedBy,
		workDependencyEdgeNeedsAction,
		workDependencyEdgeRelatedWork,
	}
}

func workDependencyRelationshipAuthorityValues() []string {
	return []string{
		workDependencyRelationshipOperatingProjection,
		workDependencyRelationshipCanonicalMirror,
	}
}

func workDependencyCanonicalRelationshipKindValues() []string {
	return []string{
		workDependencyCanonicalRelationshipTicketPullRequest,
	}
}

func workDependencyNodeKindValues() []string {
	return []string{
		workDependencyNodeWorkstream,
		workDependencyNodeTicket,
		workDependencyNodePullRequest,
		workDependencyNodeBlocker,
		workDependencyNodeAction,
		workDependencyNodeComponent,
	}
}

const (
	workDependencyEndpointFrom = "from"
	workDependencyEndpointTo   = "to"
)

func workDependencyEndpointRoleValues() []string {
	return []string{
		workDependencyEndpointFrom,
		workDependencyEndpointTo,
	}
}

const (
	workDependencyEndpointResolved = "resolved" // workDependencyEndpointResolved means the endpoint has a typed target pointer.
	workDependencyEndpointKeyOnly  = "key_only" // workDependencyEndpointKeyOnly means the key is meaningful but no typed target table exists yet.
	workDependencyEndpointMissing  = "missing"  // workDependencyEndpointMissing means a typed target should exist but was not resolved.
)

func workDependencyEndpointResolutionValues() []string {
	return []string{
		workDependencyEndpointResolved,
		workDependencyEndpointKeyOnly,
		workDependencyEndpointMissing,
	}
}

const (
	workBlockerImpactDirectSubject   = "direct_subject"
	workBlockerImpactWorkstream      = "workstream"
	workBlockerImpactDependencyChain = "dependency_chain"
)

func workBlockerImpactKindValues() []string {
	return []string{
		workBlockerImpactDirectSubject,
		workBlockerImpactWorkstream,
		workBlockerImpactDependencyChain,
	}
}

func workstreamOperatingStatusValues() []string {
	return []string{
		workstreamOperatingUnknown,
		workstreamOperatingClear,
		workstreamOperatingWatch,
		workstreamOperatingValidationRequired,
		workstreamOperatingAttentionRequired,
	}
}

func workstreamStandupSectionKindValues() []string {
	return []string{
		workstreamStandupSectionTopAction,
		workstreamStandupSectionProductAction,
		workstreamStandupSectionValidationLead,
		workstreamStandupSectionSourceRepair,
		workstreamStandupSectionCloseoutReview,
		workstreamStandupSectionModelOrRuleQA,
		workstreamStandupSectionSuppressedSignal,
		workstreamStandupSectionModelQuality,
		workstreamStandupSectionOwnerLoad,
		workstreamStandupSectionResolvedChange,
	}
}

func workstreamStandupUrgencyValues() []string {
	return []string{
		workstreamStandupUrgencyUnknown,
		workstreamStandupUrgencyCritical,
		workstreamStandupUrgencyHigh,
		workstreamStandupUrgencyMedium,
		workstreamStandupUrgencyLow,
	}
}

func workOwnerLoadStatusValues() []string {
	return []string{
		workOwnerLoadUnknown,
		workOwnerLoadClear,
		workOwnerLoadWatch,
		workOwnerLoadAttentionRequired,
		workOwnerLoadOverloaded,
	}
}

func workForecastEvaluationKindValues() []string {
	return []string{
		workForecastEvaluationSummary,
		workForecastEvaluationKFold,
		workForecastEvaluationChronologicalHoldout,
		workForecastEvaluationSourceEventKFold,
		workForecastEvaluationSourceEventHoldout,
		workForecastEvaluationLifecycleAsOf,
		workForecastEvaluationSurvivalTimeToMerge,
		workForecastEvaluationInsufficientSample,
	}
}

func workForecastReadinessStateValues() []string {
	return []string{
		workForecastReadinessUnknown,
		workForecastReadinessInsufficientSample,
		workForecastReadinessGated,
		workForecastReadinessReady,
	}
}

func workForecastKindValues() []string {
	return []string{workForecastKindCycleTime}
}

func workForecastRiskBandValues() []string {
	return []string{
		workForecastRiskUnknown,
		workForecastRiskLow,
		workForecastRiskMedium,
		workForecastRiskHigh,
		workForecastRiskCritical,
	}
}

func workItemTransitionKindValues() []string {
	return []string{
		workItemTransitionStateChange,
		workItemTransitionTerminalStateChange,
		workItemTransitionCoverageStateChange,
		workItemTransitionStateRefresh,
	}
}

func workItemTransitionConfidenceBasisValues() []string {
	return []string{
		workItemTransitionConfidenceBasisUnknown,
		workItemTransitionConfidenceBasisAdjacentSnapshotDetection,
		workItemTransitionConfidenceBasisSourceVerified,
		workItemTransitionConfidenceBasisHumanVerified,
	}
}

func workItemTransitionVerificationStateValues() []string {
	return []string{
		workItemTransitionVerificationCandidate,
		workItemTransitionVerificationCloseoutRequired,
		workItemTransitionVerificationSourceVerified,
		workItemTransitionVerificationHumanVerified,
	}
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
