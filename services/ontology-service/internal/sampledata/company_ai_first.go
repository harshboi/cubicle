package sampledata

import (
	"context"
	"fmt"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/documentlink"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/pullrequestauthorship"
	"cubicle/services/ontology-service/ent/pullrequestreview"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketassignment"
	"cubicle/services/ontology-service/ent/ticketdocument"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
)

const companyAIFirstFixtureInstance = "company-ai-first-minimum"

// SeedCompanyAIFirstMinimum writes a tiny persisted product graph for the
// source-neutral bounded graph eval pack.
func SeedCompanyAIFirstMinimum(ctx context.Context, client *genent.Client) error {
	if client == nil {
		return fmt.Errorf("seed company ai-first fixture: ent client is required")
	}
	observedAt := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	sourceScopes, err := seedCompanyAIFirstSourceScopes(ctx, client, observedAt)
	if err != nil {
		return err
	}

	alice, err := client.Person.Create().
		SetKey("person:alice").
		SetDisplayName("Alice Example").
		SetPrimaryEmail("alice@example.test").
		SetGithubLogin("alice").
		SetJiraAccountID("alice-jira").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed alice: %w", err)
	}
	bob, err := client.Person.Create().
		SetKey("person:bob").
		SetDisplayName("Bob Reviewer").
		SetPrimaryEmail("bob@example.test").
		SetGithubLogin("bob").
		SetJiraAccountID("bob-jira").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed bob: %w", err)
	}

	launchTicket, err := client.Ticket.Create().
		SetKey("ticket:COMP-101").
		SetTitle("Prepare customer launch checklist").
		SetBody("Track the launch checklist API, rollout plan, and customer-facing readiness notes.").
		SetStatus(ticket.StatusOpen).
		SetSummary("Customer launch checklist needs API, docs, and standup coordination.").
		SetSourceSystem("jira").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("ticket").
		SetExternalID("COMP-101").
		SetSourceURL("https://tickets.example.test/COMP-101").
		SetSourceScopeStateID(sourceScopes.JiraStateID).
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed launch ticket: %w", err)
	}
	launchPR, err := client.PullRequest.Create().
		SetKey("pull-request:company/app#42").
		SetRepository("company/app").
		SetNumber(42).
		SetTitle("Ship launch checklist API").
		SetState(pullrequest.StateOpen).
		SetSummary("Adds the checklist API used by the customer launch workflow.").
		SetSourceCreatedAt(observedAt.Add(-2 * time.Hour)).
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request").
		SetExternalID("company/app#42").
		SetSourceURL("https://github.example.test/company/app/pull/42").
		SetSourceScopeStateID(sourceScopes.GitHubStateID).
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt.Add(-2 * time.Hour)).
		SetLastActivityAt(observedAt).
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(88).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed launch PR: %w", err)
	}
	launchPlan, err := client.Document.Create().
		SetKey("document:company-plan").
		SetTitle("Company launch plan").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSummary("Launch plan tying COMP-101 to the checklist API and rollout reference.").
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown").
		SetExternalID("company-plan.md").
		SetSourceURL("https://docs.example.test/company-plan").
		SetSourceScopeStateID(sourceScopes.DocsStateID).
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt.Add(-24 * time.Hour)).
		SetLastActivityAt(observedAt).
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed launch plan: %w", err)
	}
	apiReference, err := client.Document.Create().
		SetKey("document:api-reference").
		SetTitle("API launch reference").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSummary("Reference for launch checklist API behavior.").
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown").
		SetExternalID("api-reference.md").
		SetSourceURL("https://docs.example.test/api-reference").
		SetSourceScopeStateID(sourceScopes.DocsStateID).
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt.Add(-12 * time.Hour)).
		SetLastActivityAt(observedAt).
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(75).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed api reference: %w", err)
	}
	standupMessage, err := client.Message.Create().
		SetKey("message:launch-standup").
		SetSummary("Launch standup note").
		SetBody("Alice called out COMP-101 and PR #42 as the next launch checklist focus.").
		SetChannelKey("launch-room").
		SetThreadKey("launch-2026-06-24").
		SetAuthorPersonKey("person:alice").
		SetSentAt(observedAt).
		SetSourceSystem("chat").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("chat_message").
		SetExternalID("launch-standup").
		SetSourceURL("https://chat.example.test/launch/standup").
		SetSourceScopeStateID(sourceScopes.ChatStateID).
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(70).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed standup message: %w", err)
	}

	if err := seedCompanyAIFirstRelationships(ctx, client, observedAt, alice, bob, launchTicket, launchPR, launchPlan, apiReference, standupMessage); err != nil {
		return err
	}
	if err := seedCompanyAIFirstDistractors(ctx, client, observedAt); err != nil {
		return err
	}
	if err := seedCompanyAIFirstCoverageIssue(ctx, client, sourceScopes.DocsScope, observedAt); err != nil {
		return err
	}
	return nil
}

type companyAIFirstSourceScopes struct {
	DocsScope     *genent.SourceScope
	DocsStateID   int
	JiraStateID   int
	GitHubStateID int
	ChatStateID   int
}

func seedCompanyAIFirstSourceScopes(ctx context.Context, client *genent.Client, observedAt time.Time) (companyAIFirstSourceScopes, error) {
	docsScope, docsStateID, err := seedCompanyAIFirstSourceScope(ctx, client, "docs", companyAIFirstFixtureInstance, "folder", "launch", "bounded_graph_absence=links_to", observedAt)
	if err != nil {
		return companyAIFirstSourceScopes{}, err
	}
	_, jiraStateID, err := seedCompanyAIFirstSourceScope(ctx, client, "jira", companyAIFirstFixtureInstance, "project", "COMP", "bounded_graph_absence=implemented_by,documented_by,discussed_in", observedAt)
	if err != nil {
		return companyAIFirstSourceScopes{}, err
	}
	_, githubStateID, err := seedCompanyAIFirstSourceScope(ctx, client, "github", "company/app", "repository", "company/app", "bounded_graph_absence=author,approver,reviewer", observedAt)
	if err != nil {
		return companyAIFirstSourceScopes{}, err
	}
	_, chatStateID, err := seedCompanyAIFirstSourceScope(ctx, client, "chat", companyAIFirstFixtureInstance, "channel", "launch-room", "bounded_graph_absence=discussed_in", observedAt)
	if err != nil {
		return companyAIFirstSourceScopes{}, err
	}
	return companyAIFirstSourceScopes{
		DocsScope:     docsScope,
		DocsStateID:   docsStateID,
		JiraStateID:   jiraStateID,
		GitHubStateID: githubStateID,
		ChatStateID:   chatStateID,
	}, nil
}

func seedCompanyAIFirstSourceScope(ctx context.Context, client *genent.Client, sourceSystem string, sourceInstance string, scopeKind string, scopeKey string, crawlPolicy string, observedAt time.Time) (*genent.SourceScope, int, error) {
	connection, err := client.SourceConnection.Create().
		SetKey("source-connection:" + sourceSystem + ":" + sourceInstance).
		SetSourceSystem(sourceSystem).
		SetSourceInstance(sourceInstance).
		SetDisplayName(sourceSystem + " " + sourceInstance).
		SetLastSyncedAt(observedAt).
		Save(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("seed source connection %s/%s: %w", sourceSystem, sourceInstance, err)
	}
	scope, err := client.SourceScope.Create().
		SetKey("source-scope:" + sourceSystem + ":" + scopeKey).
		SetConnection(connection).
		SetScopeKind(scopeKind).
		SetScopeKey(scopeKey).
		SetDisplayName(sourceSystem + " " + scopeKey).
		SetCrawlPolicy(crawlPolicy).
		Save(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("seed source scope %s/%s: %w", sourceSystem, scopeKey, err)
	}
	run, err := client.SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("source-sync-run:" + sourceSystem + ":" + scopeKey + ":2026-06-24").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModeExactScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(observedAt.Add(-30 * time.Minute)).
		SetCompletedAt(observedAt.Add(-25 * time.Minute)).
		SetCoverageStartAt(observedAt.Add(-7 * 24 * time.Hour)).
		SetCoverageEndAt(observedAt).
		SetObjectsSeenCount(1).
		Save(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("seed source sync run %s/%s: %w", sourceSystem, scopeKey, err)
	}
	state, err := client.SourceScopeState.Create().
		SetScope(scope).
		SetFreshnessState(sourcescopestate.FreshnessStateFresh).
		SetCoverageMode(sourcescopestate.CoverageModeExactScope).
		SetLastSuccessfulSyncRun(run).
		SetLastSuccessfulAt(observedAt.Add(-25 * time.Minute)).
		SetLastAttemptedAt(observedAt.Add(-25 * time.Minute)).
		Save(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("seed source scope state %s/%s: %w", sourceSystem, scopeKey, err)
	}
	return scope, state.ID, nil
}

func seedCompanyAIFirstRelationships(ctx context.Context, client *genent.Client, observedAt time.Time, alice *genent.Person, bob *genent.Person, launchTicket *genent.Ticket, launchPR *genent.PullRequest, launchPlan *genent.Document, apiReference *genent.Document, standupMessage *genent.Message) error {
	implementedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-101:pr-42", "implemented_by", "ticket_pull_request", "jira", companyAIFirstFixtureInstance, "remote_link", "COMP-101|company/app#42", "https://tickets.example.test/COMP-101#remote-link-pr-42", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketPullRequest.Create().
		SetTicket(launchTicket).
		SetPullRequest(launchPR).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetLatestEvidence(implementedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("jira").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("remote_link").
		SetExternalID("COMP-101|company/app#42").
		SetSourceURL("https://tickets.example.test/COMP-101#remote-link-pr-42").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(100).
		Save(ctx); err != nil {
		return fmt.Errorf("seed ticket PR relationship: %w", err)
	}

	documentedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-101:launch-plan", "documented_by", "ticket_document", "docs", companyAIFirstFixtureInstance, "markdown_link", "COMP-101|company-plan.md", "https://docs.example.test/company-plan#comp-101", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketDocument.Create().
		SetTicket(launchTicket).
		SetDocument(launchPlan).
		SetTicketDocumentKind(ticketdocument.TicketDocumentKindDocumentedBy).
		SetLatestEvidence(documentedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown_link").
		SetExternalID("COMP-101|company-plan.md").
		SetSourceURL("https://docs.example.test/company-plan#comp-101").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketdocument.FreshnessStateFresh).
		SetVisibility(ticketdocument.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(98).
		Save(ctx); err != nil {
		return fmt.Errorf("seed ticket document relationship: %w", err)
	}

	discussedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-101:standup", "discussed_in", "ticket_message", "chat", companyAIFirstFixtureInstance, "chat_message", "launch-standup", "https://chat.example.test/launch/standup", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketMessage.Create().
		SetTicket(launchTicket).
		SetMessage(standupMessage).
		SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
		SetLatestEvidence(discussedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("chat").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("chat_message").
		SetExternalID("launch-standup").
		SetSourceURL("https://chat.example.test/launch/standup").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketmessage.FreshnessStateFresh).
		SetVisibility(ticketmessage.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(80).
		Save(ctx); err != nil {
		return fmt.Errorf("seed ticket message relationship: %w", err)
	}

	assigneeEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-101:alice-assignee", "assignee", "ticket_assignment", "jira", companyAIFirstFixtureInstance, "ticket_field", "COMP-101:assignee", "https://tickets.example.test/COMP-101#assignee", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketAssignment.Create().
		SetTicket(launchTicket).
		SetPerson(alice).
		SetAssignmentKind(ticketassignment.AssignmentKindAssignee).
		SetLatestEvidence(assigneeEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("jira").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("ticket_field").
		SetExternalID("COMP-101:assignee").
		SetSourceURL("https://tickets.example.test/COMP-101#assignee").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketassignment.FreshnessStateFresh).
		SetVisibility(ticketassignment.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(92).
		Save(ctx); err != nil {
		return fmt.Errorf("seed ticket assignment relationship: %w", err)
	}

	authorEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:pr-42:alice-author", "author", "pull_request_authorship", "github", "company/app", "github_pull_request", "company/app#42:author", "https://github.example.test/company/app/pull/42", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.PullRequestAuthorship.Create().
		SetPullRequest(launchPR).
		SetPerson(alice).
		SetAuthorshipKind(pullrequestauthorship.AuthorshipKindAuthor).
		SetLatestEvidence(authorEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request").
		SetExternalID("company/app#42:author").
		SetSourceURL("https://github.example.test/company/app/pull/42").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(pullrequestauthorship.FreshnessStateFresh).
		SetVisibility(pullrequestauthorship.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(94).
		Save(ctx); err != nil {
		return fmt.Errorf("seed PR authorship relationship: %w", err)
	}

	approverEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:pr-42:bob-approver", "approver", "pull_request_review", "github", "company/app", "github_pull_request_review", "company/app#42:bob-approval", "https://github.example.test/company/app/pull/42#review-bob", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.PullRequestReview.Create().
		SetPullRequest(launchPR).
		SetPerson(bob).
		SetReviewKind(pullrequestreview.ReviewKindApprover).
		SetLatestEvidence(approverEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request_review").
		SetExternalID("company/app#42:bob-approval").
		SetSourceURL("https://github.example.test/company/app/pull/42#review-bob").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(pullrequestreview.FreshnessStateFresh).
		SetVisibility(pullrequestreview.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(86).
		Save(ctx); err != nil {
		return fmt.Errorf("seed PR review relationship: %w", err)
	}

	docLinkEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:launch-plan:api-reference", "links_to", "document_link", "docs", companyAIFirstFixtureInstance, "markdown_link", "company-plan.md|api-reference.md", "https://docs.example.test/company-plan#api-reference", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.DocumentLink.Create().
		SetFromDocument(launchPlan).
		SetToDocument(apiReference).
		SetDocumentLinkKind(documentlink.DocumentLinkKindLinksTo).
		SetLatestEvidence(docLinkEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown_link").
		SetExternalID("company-plan.md|api-reference.md").
		SetSourceURL("https://docs.example.test/company-plan#api-reference").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(documentlink.FreshnessStateFresh).
		SetVisibility(documentlink.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(82).
		Save(ctx); err != nil {
		return fmt.Errorf("seed document link relationship: %w", err)
	}
	return nil
}

func seedCompanyAIFirstDistractors(ctx context.Context, client *genent.Client, observedAt time.Time) error {
	distractorAt := observedAt.Add(30 * time.Minute)
	mallory, err := client.Person.Create().
		SetKey("person:mallory").
		SetDisplayName("Mallory Distractor").
		SetPrimaryEmail("mallory@example.test").
		SetGithubLogin("mallory").
		SetJiraAccountID("mallory-jira").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed distractor person: %w", err)
	}
	distractorTicket, err := client.Ticket.Create().
		SetKey("ticket:COMP-999").
		SetTitle("Unrelated finance export").
		SetBody("Separate finance export work that shares the same source tenant but is not connected to launch.").
		SetStatus(ticket.StatusOpen).
		SetSummary("Unrelated finance export ticket.").
		SetSourceSystem("ticketing").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("ticket").
		SetExternalID("COMP-999").
		SetSourceURL("https://tickets.example.test/COMP-999").
		SetSourceUpdatedAt(distractorAt).
		SetLastConfirmedAt(distractorAt).
		SetFirstSeenAt(distractorAt).
		SetLastActivityAt(distractorAt).
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed distractor ticket: %w", err)
	}
	distractorPR, err := client.PullRequest.Create().
		SetKey("pull-request:company/app#99").
		SetRepository("company/app").
		SetNumber(99).
		SetTitle("Export finance report").
		SetState(pullrequest.StateOpen).
		SetSummary("Unrelated finance reporting change in the same repository.").
		SetSourceCreatedAt(distractorAt).
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request").
		SetExternalID("company/app#99").
		SetSourceURL("https://github.example.test/company/app/pull/99").
		SetSourceUpdatedAt(distractorAt).
		SetLastConfirmedAt(distractorAt).
		SetFirstSeenAt(distractorAt).
		SetLastActivityAt(distractorAt).
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed distractor PR: %w", err)
	}
	distractorDocument, err := client.Document.Create().
		SetKey("document:unrelated-roadmap").
		SetTitle("Unrelated finance roadmap").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSummary("Finance roadmap that should not appear in launch traversals.").
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown").
		SetExternalID("unrelated-roadmap.md").
		SetSourceURL("https://docs.example.test/unrelated-roadmap").
		SetSourceUpdatedAt(distractorAt).
		SetLastConfirmedAt(distractorAt).
		SetFirstSeenAt(distractorAt).
		SetLastActivityAt(distractorAt).
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed distractor document: %w", err)
	}
	distractorMessage, err := client.Message.Create().
		SetKey("message:finance-thread").
		SetSummary("Finance export thread").
		SetBody("Mallory is coordinating COMP-999 and PR #99 in a separate thread.").
		SetChannelKey("finance-room").
		SetThreadKey("finance-2026-06-24").
		SetAuthorPersonKey("person:mallory").
		SetSentAt(distractorAt).
		SetSourceSystem("chat").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("chat_message").
		SetExternalID("finance-thread").
		SetSourceURL("https://chat.example.test/finance/thread").
		SetSourceUpdatedAt(distractorAt).
		SetLastConfirmedAt(distractorAt).
		SetFirstSeenAt(distractorAt).
		SetLastActivityAt(distractorAt).
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed distractor message: %w", err)
	}

	if err := seedCompanyAIFirstDistractorRelationships(ctx, client, distractorAt, mallory, distractorTicket, distractorPR, distractorDocument, distractorMessage); err != nil {
		return err
	}
	return nil
}

func seedCompanyAIFirstDistractorRelationships(ctx context.Context, client *genent.Client, observedAt time.Time, mallory *genent.Person, distractorTicket *genent.Ticket, distractorPR *genent.PullRequest, distractorDocument *genent.Document, distractorMessage *genent.Message) error {
	implementedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-999:pr-99", "implemented_by", "ticket_pull_request", "ticketing", companyAIFirstFixtureInstance, "remote_link", "COMP-999|company/app#99", "https://tickets.example.test/COMP-999#remote-link-pr-99", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketPullRequest.Create().
		SetTicket(distractorTicket).
		SetPullRequest(distractorPR).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetLatestEvidence(implementedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("ticketing").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("remote_link").
		SetExternalID("COMP-999|company/app#99").
		SetSourceURL("https://tickets.example.test/COMP-999#remote-link-pr-99").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx); err != nil {
		return fmt.Errorf("seed distractor ticket PR relationship: %w", err)
	}

	documentedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-999:roadmap", "documented_by", "ticket_document", "docs", companyAIFirstFixtureInstance, "markdown_link", "COMP-999|unrelated-roadmap.md", "https://docs.example.test/unrelated-roadmap#comp-999", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketDocument.Create().
		SetTicket(distractorTicket).
		SetDocument(distractorDocument).
		SetTicketDocumentKind(ticketdocument.TicketDocumentKindDocumentedBy).
		SetLatestEvidence(documentedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown_link").
		SetExternalID("COMP-999|unrelated-roadmap.md").
		SetSourceURL("https://docs.example.test/unrelated-roadmap#comp-999").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketdocument.FreshnessStateFresh).
		SetVisibility(ticketdocument.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx); err != nil {
		return fmt.Errorf("seed distractor ticket document relationship: %w", err)
	}

	discussedEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-999:finance-thread", "discussed_in", "ticket_message", "chat", companyAIFirstFixtureInstance, "chat_message", "finance-thread", "https://chat.example.test/finance/thread", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketMessage.Create().
		SetTicket(distractorTicket).
		SetMessage(distractorMessage).
		SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
		SetLatestEvidence(discussedEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("chat").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("chat_message").
		SetExternalID("finance-thread").
		SetSourceURL("https://chat.example.test/finance/thread").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketmessage.FreshnessStateFresh).
		SetVisibility(ticketmessage.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx); err != nil {
		return fmt.Errorf("seed distractor ticket message relationship: %w", err)
	}

	assigneeEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:comp-999:mallory-assignee", "assignee", "ticket_assignment", "ticketing", companyAIFirstFixtureInstance, "ticket_field", "COMP-999:assignee", "https://tickets.example.test/COMP-999#assignee", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.TicketAssignment.Create().
		SetTicket(distractorTicket).
		SetPerson(mallory).
		SetAssignmentKind(ticketassignment.AssignmentKindAssignee).
		SetLatestEvidence(assigneeEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("ticketing").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("ticket_field").
		SetExternalID("COMP-999:assignee").
		SetSourceURL("https://tickets.example.test/COMP-999#assignee").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(ticketassignment.FreshnessStateFresh).
		SetVisibility(ticketassignment.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx); err != nil {
		return fmt.Errorf("seed distractor ticket assignment relationship: %w", err)
	}

	authorEvidence, err := companyRelationshipEvidence(ctx, client, "evidence:company:pr-99:mallory-author", "author", "pull_request_authorship", "github", "company/app", "github_pull_request", "company/app#99:author", "https://github.example.test/company/app/pull/99", observedAt)
	if err != nil {
		return err
	}
	if _, err := client.PullRequestAuthorship.Create().
		SetPullRequest(distractorPR).
		SetPerson(mallory).
		SetAuthorshipKind(pullrequestauthorship.AuthorshipKindAuthor).
		SetLatestEvidence(authorEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("github").
		SetSourceInstance("company/app").
		SetExternalKind("github_pull_request").
		SetExternalID("company/app#99:author").
		SetSourceURL("https://github.example.test/company/app/pull/99").
		SetSourceUpdatedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetFirstSeenAt(observedAt).
		SetLastActivityAt(observedAt).
		SetFreshnessState(pullrequestauthorship.FreshnessStateFresh).
		SetVisibility(pullrequestauthorship.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(999).
		Save(ctx); err != nil {
		return fmt.Errorf("seed distractor PR authorship relationship: %w", err)
	}
	return nil
}

func companyRelationshipEvidence(ctx context.Context, client *genent.Client, key string, relationshipKind string, targetKind string, sourceSystem string, sourceInstance string, externalKind string, externalID string, sourceURL string, observedAt time.Time) (*genent.Evidence, error) {
	row, err := client.Evidence.Create().
		SetKey(key).
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind(targetKind).
		SetRelationshipKind(relationshipKind).
		SetLocatorKind(externalKind).
		SetLocator(key).
		SetSourceSpanKey(key).
		SetSourceSystem(sourceSystem).
		SetSourceInstance(sourceInstance).
		SetExternalKind(externalKind).
		SetExternalID(externalID).
		SetSourceURL(sourceURL).
		SetSourceUpdatedAt(observedAt).
		SetObservedAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed relationship evidence %s: %w", key, err)
	}
	return row, nil
}

func seedCompanyAIFirstCoverageIssue(ctx context.Context, client *genent.Client, scope *genent.SourceScope, observedAt time.Time) error {
	_, err := client.SourceSyncIssue.Create().
		SetScope(scope).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_http_403").
		SetMessage("HTTP 403 while fetching restricted launch-plan comments").
		SetSourceSystem("docs").
		SetSourceInstance(companyAIFirstFixtureInstance).
		SetExternalKind("markdown").
		SetExternalID("company-plan.md").
		SetSourceURL("https://docs.example.test/company-plan").
		Save(ctx)
	if err != nil {
		return fmt.Errorf("seed source sync issue: %w", err)
	}
	return nil
}
