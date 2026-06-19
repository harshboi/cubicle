// Association:
//
//	Record -> LoadFixture -> SourceConnection -> SourceScope -> SourceSyncRun
//	LoadFixture -> Ticket -> TicketPullRequest -> PullRequest
//	LoadFixture -> Evidence; non-200 snapshots -> SourceSyncIssue only
//
// Fixture replay separates product truth from source coverage so failed fetches
// never become empty product rows.
package sourcegraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/sourceconnection"
	"cubicle/services/ontology-service/ent/sourcescope"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/worklens"
	"cubicle/services/ontology-service/ent/worklenswindow"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

const (
	defaultFixtureStreamKey = "flink-autoscaler-2026-05-11_2026-06-11"
	fixtureSourceSystem     = "fixture"
	fixtureExternalKind     = "flink_workstream"
)

var requiredPRBundleTypes = []string{
	"github_pull_request",
	"github_pull_request_files",
	"github_issue_comments",
	"github_pull_request_review_comments",
	"github_pull_request_reviews",
	"github_pull_request_commits",
}

// LoadOptions controls materialization of a replay fixture into typed graph rows.
type LoadOptions struct {
	StreamKey   string
	DisplayName string
	RunKey      string
	Now         func() time.Time
}

// LoadResult reports what the fixture materializer accepted into product rows
// and what it retained only as source coverage evidence.
type LoadResult struct {
	RecordsSeen                int `json:"records_seen"`
	Records200                 int `json:"records_200"`
	RecordsFailed              int `json:"records_failed"`
	DiscoveredPullRequests     int `json:"discovered_pull_requests"`
	CompletePullRequestBundles int `json:"complete_pull_request_bundles"`
	Tickets                    int `json:"tickets"`
	PullRequests               int `json:"pull_requests"`
	TicketPullRequests         int `json:"ticket_pull_requests"`
	Evidence                   int `json:"evidence"`
	SyncIssues                 int `json:"sync_issues"`
	WorkLensWindows            int `json:"work_lens_windows"`
}

// LoadFixture materializes replay records into the typed ontology graph. Non-200
// source bodies are never normalized as empty product state; they only become
// SourceSyncIssue coverage rows and aggregate sync counters.
func LoadFixture(ctx context.Context, client *genent.Client, records []sourcecapture.Record, opts LoadOptions) (LoadResult, error) {
	if client == nil {
		return LoadResult{}, errors.New("ent client is required")
	}
	now := fixtureNow(opts.Now)
	streamKey := fixtureStreamKey(opts.StreamKey)
	displayName := opts.DisplayName
	if displayName == "" {
		displayName = "Flink autoscaler fixture"
	}
	runKey := opts.RunKey
	if runKey == "" {
		runKey = "source-sync-run:" + streamKey + ":" + now.Format("20060102T150405.000000000Z")
	}

	result := LoadResult{RecordsSeen: len(records)}
	result.DiscoveredPullRequests = len(discoveredPullRequests(records))
	for _, record := range records {
		if record.Response.StatusCode == 200 {
			result.Records200++
			continue
		}
		result.RecordsFailed++
	}

	conn, err := ensureFixtureConnection(ctx, client, streamKey, displayName, now)
	if err != nil {
		return LoadResult{}, err
	}
	scope, err := ensureFixtureScope(ctx, client, conn.ID, streamKey, displayName)
	if err != nil {
		return LoadResult{}, err
	}
	run, err := client.SourceSyncRun.Create().
		SetSourceScopeID(scope.ID).
		SetRunKey(runKey).
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(coverageModeFor(result.RecordsFailed)).
		SetStatus(sourcesyncrun.StatusRunning).
		SetStartedAt(now).
		Save(ctx)
	if err != nil {
		return LoadResult{}, fmt.Errorf("create fixture sync run: %w", err)
	}

	if err := materializeSyncIssues(ctx, client, scope.ID, run.ID, records, &result); err != nil {
		return LoadResult{}, err
	}

	ticketsByKey := make(map[string]*genent.Ticket)
	for _, record := range records {
		if record.Response.StatusCode != 200 || record.SourceObjectType != "jira_issue" {
			continue
		}
		t, evidenceCount, err := materializeJiraIssue(ctx, client, record, now)
		if err != nil {
			return LoadResult{}, err
		}
		ticketsByKey[t.Key] = t
		result.Tickets++
		result.Evidence += evidenceCount
	}

	prsByKey := make(map[string]*genent.PullRequest)
	completeBundles := completePRBundles(records)
	for _, objectID := range sortedBundleKeys(completeBundles) {
		record := completeBundles[objectID]["github_pull_request"]
		pr, err := materializeCompletePullRequest(ctx, client, record, now)
		if err != nil {
			return LoadResult{}, err
		}
		prsByKey[pr.Key] = pr
		if _, err := upsertObjectEvidence(ctx, client, "pull_request", pr.ID, "github_pull_request", record, pr.Title, now); err != nil {
			return LoadResult{}, err
		}
		result.CompletePullRequestBundles++
		result.Evidence++
	}

	for _, record := range records {
		if record.Response.StatusCode != 200 || record.SourceObjectType != "jira_remote_links" {
			continue
		}
		t, ok := ticketsByKey[ticketKey(record.SourceObjectID)]
		if !ok {
			var err error
			t, err = client.Ticket.Query().Where(ticket.KeyEQ(ticketKey(record.SourceObjectID))).Only(ctx)
			if err != nil {
				if genent.IsNotFound(err) {
					continue
				}
				return LoadResult{}, fmt.Errorf("load remote-link ticket %s: %w", record.SourceObjectID, err)
			}
			ticketsByKey[t.Key] = t
		}
		edges, evidenceCount, err := materializeJiraRemoteLinks(ctx, client, t, record, now, prsByKey)
		if err != nil {
			return LoadResult{}, err
		}
		result.TicketPullRequests += edges
		result.Evidence += evidenceCount
	}

	result.PullRequests = len(prsByKey)
	window, err := ensureFixtureLensWindow(ctx, client, streamKey, displayName, result.PullRequests, result.RecordsFailed == 0, now)
	if err != nil {
		return LoadResult{}, err
	}
	if window != nil {
		result.WorkLensWindows = 1
	}

	status := sourcesyncrun.StatusComplete
	if result.RecordsFailed > 0 {
		status = sourcesyncrun.StatusPartial
	}
	if err := run.Update().
		SetCoverageMode(coverageModeFor(result.RecordsFailed)).
		SetStatus(status).
		SetCompletedAt(now).
		SetCheckpointToken("records:" + strconv.Itoa(result.RecordsSeen)).
		SetObjectsSeenCount(result.DiscoveredPullRequests + result.Tickets).
		SetObjectsCreatedCount(result.Tickets + result.PullRequests).
		SetRelationshipsCreatedCount(result.TicketPullRequests).
		SetEvidenceCreatedCount(result.Evidence).
		SetIssuesCreatedCount(result.SyncIssues).
		Exec(ctx); err != nil {
		return LoadResult{}, fmt.Errorf("update fixture sync run counters: %w", err)
	}

	return result, nil
}

// fixtureNow gives replay loads one clock so source rows and evidence agree.
func fixtureNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}

// fixtureStreamKey names the bounded Flink workstream when callers do not override it.
func fixtureStreamKey(streamKey string) string {
	if streamKey == "" {
		return defaultFixtureStreamKey
	}
	return streamKey
}

// coverageModeFor makes failed snapshots visible as partial coverage, not missing facts.
func coverageModeFor(failed int) sourcesyncrun.CoverageMode {
	if failed > 0 {
		return sourcesyncrun.CoverageModePartialScope
	}
	return sourcesyncrun.CoverageModeExactScope
}

// ensureFixtureConnection records the fixture as a source before any product rows appear.
func ensureFixtureConnection(ctx context.Context, client *genent.Client, streamKey string, displayName string, now time.Time) (*genent.SourceConnection, error) {
	key := "source-connection:" + streamKey
	conn, err := client.SourceConnection.Query().Where(sourceconnection.KeyEQ(key)).Only(ctx)
	if err == nil {
		if err := conn.Update().SetLastSyncedAt(now).Exec(ctx); err != nil {
			return nil, fmt.Errorf("update fixture source connection: %w", err)
		}
		return conn, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture source connection: %w", err)
	}
	return client.SourceConnection.Create().
		SetKey(key).
		SetSourceSystem(fixtureSourceSystem).
		SetSourceInstance(streamKey).
		SetDisplayName(displayName).
		SetConnectorKind("fixture_replay").
		SetLastSyncedAt(now).
		Save(ctx)
}

// ensureFixtureScope bounds the crawl to one workstream so coverage is scoped.
func ensureFixtureScope(ctx context.Context, client *genent.Client, sourceConnectionID int, streamKey string, displayName string) (*genent.SourceScope, error) {
	key := "source-scope:" + streamKey
	scope, err := client.SourceScope.Query().Where(sourcescope.KeyEQ(key)).Only(ctx)
	if err == nil {
		return scope, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture source scope: %w", err)
	}
	return client.SourceScope.Create().
		SetKey(key).
		SetSourceConnectionID(sourceConnectionID).
		SetScopeKind("workstream").
		SetScopeKey(streamKey).
		SetDisplayName(displayName).
		SetCrawlPolicy("fixture_replay").
		Save(ctx)
}

// materializeSyncIssues turns non-200 snapshots into coverage rows only.
func materializeSyncIssues(ctx context.Context, client *genent.Client, scopeID int, runID int, records []sourcecapture.Record, result *LoadResult) error {
	for _, record := range records {
		status := record.Response.StatusCode
		if status == 200 {
			continue
		}
		if _, err := client.SourceSyncIssue.Create().
			SetScopeID(scopeID).
			SetSyncRunID(runID).
			SetSeverity(sourcesyncissue.SeverityWarning).
			SetIssueCode(syncIssueCode(status)).
			SetMessage(fmt.Sprintf("source snapshot returned status %d; body retained for replay coverage only", status)).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(record.SourceObjectID).
			SetNillableSourceURL(nonEmptyPtr(sourceURLForRecord(record))).
			Save(ctx); err != nil {
			return fmt.Errorf("create sync issue for %s: %w", record.SnapshotKey, err)
		}
		result.SyncIssues++
	}
	return nil
}

// syncIssueCode keeps common source failure modes queryable by code.
func syncIssueCode(status int) string {
	switch status {
	case 403:
		return "source_forbidden"
	case 429:
		return "source_rate_limited"
	default:
		return "source_non_200"
	}
}

// jiraIssuePayload is the minimal Jira issue shape needed for Ticket materialization.
type jiraIssuePayload struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string          `json:"summary"`
		Description json.RawMessage `json:"description"`
		Status      struct {
			Name string `json:"name"`
		} `json:"status"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		Updated string `json:"updated"`
	} `json:"fields"`
}

// materializeJiraIssue maps one successful Jira issue snapshot into Ticket + Evidence.
func materializeJiraIssue(ctx context.Context, client *genent.Client, record sourcecapture.Record, now time.Time) (*genent.Ticket, int, error) {
	var payload jiraIssuePayload
	if err := json.Unmarshal(record.Body, &payload); err != nil {
		return nil, 0, fmt.Errorf("decode Jira issue %s: %w", record.SnapshotKey, err)
	}
	key := payload.Key
	if key == "" {
		key = record.SourceObjectID
	}
	title := payload.Fields.Summary
	if title == "" {
		title = key
	}
	body := jiraDescriptionText(payload.Fields.Description)
	sourceUpdatedAt := parseJiraTime(payload.Fields.Updated)

	t, err := client.Ticket.Query().Where(ticket.KeyEQ(ticketKey(key))).Only(ctx)
	if err == nil {
		updater := t.Update().
			SetTitle(title).
			SetStatus(normalizeTicketStatus(payload.Fields.Status.Name)).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(key).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(ticket.FreshnessStateFresh).
			SetVisibility(ticket.VisibilityPublic).
			SetACLState(ticket.ACLStateCurrent).
			SetFreshnessCheckedAt(now).
			SetLastConfirmedAt(now)
		if body != "" {
			updater.SetBody(body)
		}
		if payload.Fields.Priority.Name != "" {
			updater.SetPriority(payload.Fields.Priority.Name)
		}
		if sourceURL := sourceURLForRecord(record); sourceURL != "" {
			updater.SetSourceURL(sourceURL)
		}
		if !sourceUpdatedAt.IsZero() {
			updater.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
		}
		t, err = updater.Save(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("update ticket %s: %w", key, err)
		}
		e, err := upsertObjectEvidence(ctx, client, "ticket", t.ID, "jira_issue", record, title, now)
		if err != nil {
			return nil, 0, err
		}
		_ = e
		return t, 1, nil
	}
	if !genent.IsNotFound(err) {
		return nil, 0, fmt.Errorf("query ticket %s: %w", key, err)
	}

	builder := client.Ticket.Create().
		SetKey(ticketKey(key)).
		SetTitle(title).
		SetStatus(normalizeTicketStatus(payload.Fields.Status.Name)).
		SetSourceSystem(record.SourceKey).
		SetSourceInstance(record.SourceInstance).
		SetExternalKind(record.SourceObjectType).
		SetExternalID(key).
		SetContentHash(record.BodySHA256).
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetACLState(ticket.ACLStateCurrent).
		SetFreshnessCheckedAt(now).
		SetLastConfirmedAt(now)
	if body != "" {
		builder.SetBody(body)
	}
	if payload.Fields.Priority.Name != "" {
		builder.SetPriority(payload.Fields.Priority.Name)
	}
	if sourceURL := sourceURLForRecord(record); sourceURL != "" {
		builder.SetSourceURL(sourceURL)
	}
	if !sourceUpdatedAt.IsZero() {
		builder.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
	}
	t, err = builder.Save(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("create ticket %s: %w", key, err)
	}
	if _, err := upsertObjectEvidence(ctx, client, "ticket", t.ID, "jira_issue", record, title, now); err != nil {
		return nil, 0, err
	}
	return t, 1, nil
}

// jiraDescriptionText preserves Jira descriptions without depending on one payload era.
func jiraDescriptionText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}

// normalizeTicketStatus maps source workflow names into Cubicle ticket states.
func normalizeTicketStatus(status string) ticket.Status {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "closed", "done", "resolved":
		return ticket.StatusClosed
	case "":
		return ticket.StatusUnknown
	default:
		return ticket.StatusOpen
	}
}

// githubPullRequestPayload is the GitHub PR detail shape required for full PR rows.
type githubPullRequestPayload struct {
	HTMLURL   string `json:"html_url"`
	Title     string `json:"title"`
	State     string `json:"state"`
	MergedAt  string `json:"merged_at"`
	Number    int    `json:"number"`
	UpdatedAt string `json:"updated_at"`
	Base      struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

// materializeCompletePullRequest creates a fresh PullRequest only from a complete PR detail snapshot.
func materializeCompletePullRequest(ctx context.Context, client *genent.Client, record sourcecapture.Record, now time.Time) (*genent.PullRequest, error) {
	var payload githubPullRequestPayload
	if err := json.Unmarshal(record.Body, &payload); err != nil {
		return nil, fmt.Errorf("decode GitHub pull request %s: %w", record.SnapshotKey, err)
	}
	ref := PullRequestRef{Repo: payload.Base.Repo.FullName, Number: payload.Number}
	if ref.Repo == "" || ref.Number == 0 {
		parsed, ok := parsePRObjectID(record.SourceObjectID)
		if !ok {
			return nil, fmt.Errorf("pull request snapshot %s missing repo/number", record.SnapshotKey)
		}
		ref = parsed
	}
	title := payload.Title
	if title == "" {
		title = prExternalID(ref)
	}
	sourceURL := payload.HTMLURL
	if sourceURL == "" {
		sourceURL = prURL(ref)
	}
	sourceUpdatedAt := parseGitHubTime(payload.UpdatedAt)
	mergedAt := parseGitHubTime(payload.MergedAt)

	existing, err := client.PullRequest.Query().Where(pullrequest.KeyEQ(prKey(ref))).Only(ctx)
	if err == nil {
		updater := existing.Update().
			SetRepository(ref.Repo).
			SetNumber(ref.Number).
			SetTitle(title).
			SetState(normalizePRState(payload.State, mergedAt)).
			SetSummary(title).
			SetSearchText(title).
			SetSourceSystem(SourceGitHub).
			SetSourceInstance(githubSourceInstance(ref.Repo)).
			SetExternalKind("github_pull_request").
			SetExternalID(prExternalID(ref)).
			SetSourceURL(sourceURL).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(pullrequest.FreshnessStateFresh).
			SetVisibility(pullrequest.VisibilityPublic).
			SetACLState(pullrequest.ACLStateCurrent).
			SetConfidence(1).
			SetFreshnessCheckedAt(now).
			SetLastConfirmedAt(now)
		if !sourceUpdatedAt.IsZero() {
			updater.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
		}
		if !mergedAt.IsZero() {
			updater.SetMergedAt(mergedAt)
		}
		return updater.Save(ctx)
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query pull request %s: %w", prExternalID(ref), err)
	}

	builder := client.PullRequest.Create().
		SetKey(prKey(ref)).
		SetRepository(ref.Repo).
		SetNumber(ref.Number).
		SetTitle(title).
		SetState(normalizePRState(payload.State, mergedAt)).
		SetSummary(title).
		SetSearchText(title).
		SetSourceSystem(SourceGitHub).
		SetSourceInstance(githubSourceInstance(ref.Repo)).
		SetExternalKind("github_pull_request").
		SetExternalID(prExternalID(ref)).
		SetSourceURL(sourceURL).
		SetContentHash(record.BodySHA256).
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetACLState(pullrequest.ACLStateCurrent).
		SetConfidence(1).
		SetFreshnessCheckedAt(now).
		SetLastConfirmedAt(now)
	if !sourceUpdatedAt.IsZero() {
		builder.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
	}
	if !mergedAt.IsZero() {
		builder.SetMergedAt(mergedAt)
	}
	return builder.Save(ctx)
}

// normalizePRState prefers merged_at because GitHub reports merged PRs as closed.
func normalizePRState(state string, mergedAt time.Time) pullrequest.State {
	if !mergedAt.IsZero() {
		return pullrequest.StateMerged
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open":
		return pullrequest.StateOpen
	case "closed":
		return pullrequest.StateClosed
	case "":
		return pullrequest.StateUnknown
	default:
		return pullrequest.StateUnknown
	}
}

// materializeJiraRemoteLinks turns Jira PR links into TicketPullRequest edges with evidence.
func materializeJiraRemoteLinks(ctx context.Context, client *genent.Client, t *genent.Ticket, record sourcecapture.Record, now time.Time, prsByKey map[string]*genent.PullRequest) (int, int, error) {
	prURLs, err := PRURLsFromJiraRemoteLinks(record)
	if err != nil {
		return 0, 0, err
	}
	var edges int
	var evidenceCount int
	for _, rawURL := range prURLs {
		ref, ok := ParseGitHubPRURL(rawURL)
		if !ok {
			continue
		}
		pr := prsByKey[prKey(ref)]
		if pr == nil {
			pr, err = ensureMinimalPullRequest(ctx, client, ref, now)
			if err != nil {
				return 0, 0, err
			}
			prsByKey[pr.Key] = pr
		}
		if pr.FreshnessState == pullrequest.FreshnessStatePartial {
			if _, err := upsertPullRequestRemoteLinkEvidence(ctx, client, pr.ID, record, rawURL, now); err != nil {
				return 0, 0, err
			}
			evidenceCount++
		}
		rel, created, err := ensureTicketPullRequest(ctx, client, t, pr, record, now)
		if err != nil {
			return 0, 0, err
		}
		e, err := upsertRelationshipEvidence(ctx, client, rel.ID, record, rawURL, now)
		if err != nil {
			return 0, 0, err
		}
		if err := rel.Update().
			SetLatestEvidenceID(e.ID).
			SetEvidenceCount(1).
			SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
			SetLastConfirmedAt(now).
			Exec(ctx); err != nil {
			return 0, 0, fmt.Errorf("update ticket-pull-request evidence: %w", err)
		}
		if created {
			edges++
		}
		evidenceCount++
	}
	return edges, evidenceCount, nil
}

// ensureMinimalPullRequest records a remote-link-only PR as partial, not fully trusted.
func ensureMinimalPullRequest(ctx context.Context, client *genent.Client, ref PullRequestRef, now time.Time) (*genent.PullRequest, error) {
	existing, err := client.PullRequest.Query().Where(pullrequest.KeyEQ(prKey(ref))).Only(ctx)
	if err == nil {
		return existing, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query minimal pull request %s: %w", prExternalID(ref), err)
	}
	return client.PullRequest.Create().
		SetKey(prKey(ref)).
		SetRepository(ref.Repo).
		SetNumber(ref.Number).
		SetTitle(prExternalID(ref)).
		SetState(pullrequest.StateUnknown).
		SetSourceSystem(SourceGitHub).
		SetSourceInstance(githubSourceInstance(ref.Repo)).
		SetExternalKind("github_pull_request").
		SetExternalID(prExternalID(ref)).
		SetSourceURL(prURL(ref)).
		SetFreshnessState(pullrequest.FreshnessStatePartial).
		SetVisibility(pullrequest.VisibilityPublic).
		SetACLState(pullrequest.ACLStateUnknown).
		SetConfidence(0.6).
		SetFreshnessCheckedAt(now).
		SetLastConfirmedAt(now).
		Save(ctx)
}

// ensureTicketPullRequest upserts the typed "ticket implemented by PR" relationship.
func ensureTicketPullRequest(ctx context.Context, client *genent.Client, t *genent.Ticket, pr *genent.PullRequest, record sourcecapture.Record, now time.Time) (*genent.TicketPullRequest, bool, error) {
	rel, err := client.TicketPullRequest.Query().Where(
		ticketpullrequest.TicketIDEQ(t.ID),
		ticketpullrequest.PullRequestIDEQ(pr.ID),
		ticketpullrequest.TicketPullRequestKindEQ(ticketpullrequest.TicketPullRequestKindImplementedBy),
	).Only(ctx)
	if err == nil {
		return rel, false, nil
	}
	if !genent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query ticket-pull-request %s -> %s: %w", t.Key, pr.Key, err)
	}
	builder := client.TicketPullRequest.Create().
		SetTicketID(t.ID).
		SetPullRequestID(pr.ID).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetEvidenceCount(1).
		SetSourceSystem(record.SourceKey).
		SetSourceInstance(record.SourceInstance).
		SetExternalKind(record.SourceObjectType).
		SetExternalID(record.SourceObjectID + "->" + pr.ExternalID).
		SetContentHash(record.BodySHA256).
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetACLState(ticketpullrequest.ACLStateCurrent).
		SetConfidence(1).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetLastConfirmedAt(now)
	if sourceURL := sourceURLForRecord(record); sourceURL != "" {
		builder.SetSourceURL(sourceURL)
	}
	rel, err = builder.Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create ticket-pull-request %s -> %s: %w", t.Key, pr.Key, err)
	}
	return rel, true, nil
}

// upsertObjectEvidence answers why one product row is believed current.
func upsertObjectEvidence(ctx context.Context, client *genent.Client, targetKind string, targetID int, locatorKind string, record sourcecapture.Record, excerpt string, now time.Time) (*genent.Evidence, error) {
	key := "evidence:" + targetKind + ":" + record.SourceObjectID + ":" + record.BodySHA256
	return upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:       evidence.ClaimKindObjectState,
		ClaimTargetKind: targetKind,
		ClaimTargetID:   targetID,
		LocatorKind:     locatorKind,
		Locator:         sourceURLForRecord(record),
		SourceSpanKey:   record.SnapshotKey,
		Excerpt:         excerpt,
		Record:          record,
		ObservedAt:      now,
	})
}

// upsertRelationshipEvidence answers why one typed relationship is believed current.
func upsertRelationshipEvidence(ctx context.Context, client *genent.Client, relationshipID int, record sourcecapture.Record, excerpt string, now time.Time) (*genent.Evidence, error) {
	key := "evidence:ticket_pull_request:" + record.SourceObjectID + ":" + sourcecapture.HashBody([]byte(excerpt))
	return upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:        evidence.ClaimKindRelationship,
		ClaimTargetKind:  "ticket_pull_request",
		ClaimTargetID:    relationshipID,
		RelationshipKind: ticketpullrequest.TicketPullRequestKindImplementedBy.String(),
		RelationshipID:   relationshipID,
		LocatorKind:      "jira_remote_link",
		Locator:          sourceURLForRecord(record),
		SourceSpanKey:    record.SnapshotKey + ":" + excerpt,
		Excerpt:          excerpt,
		Record:           record,
		ObservedAt:       now,
	})
}

// upsertPullRequestRemoteLinkEvidence marks remote-link-only PR rows as candidate evidence.
func upsertPullRequestRemoteLinkEvidence(ctx context.Context, client *genent.Client, pullRequestID int, record sourcecapture.Record, prURL string, now time.Time) (*genent.Evidence, error) {
	key := "evidence:pull_request:jira_remote_link:" + record.SourceObjectID + ":" + sourcecapture.HashBody([]byte(prURL))
	return upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:       evidence.ClaimKindCandidate,
		ClaimTargetKind: "pull_request",
		ClaimTargetID:   pullRequestID,
		LocatorKind:     "jira_remote_link",
		Locator:         sourceURLForRecord(record),
		SourceSpanKey:   record.SnapshotKey + ":" + prURL,
		Excerpt:         prURL,
		Record:          record,
		ObservedAt:      now,
	})
}

// evidenceSpec carries the source locator and claim target into the shared Evidence upsert.
type evidenceSpec struct {
	ClaimKind        evidence.ClaimKind
	ClaimTargetKind  string
	ClaimTargetID    int
	RelationshipKind string
	RelationshipID   int
	LocatorKind      string
	Locator          string
	SourceSpanKey    string
	Excerpt          string
	Record           sourcecapture.Record
	ObservedAt       time.Time
}

// upsertEvidence is the idempotent writer behind object, relationship, and candidate proof.
func upsertEvidence(ctx context.Context, client *genent.Client, key string, spec evidenceSpec) (*genent.Evidence, error) {
	existing, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Only(ctx)
	if err == nil {
		updater := existing.Update().
			SetClaimKind(spec.ClaimKind).
			SetProofState(evidence.ProofStateCurrent).
			SetSourceSystem(spec.Record.SourceKey).
			SetSourceInstance(spec.Record.SourceInstance).
			SetExternalKind(spec.Record.SourceObjectType).
			SetExternalID(spec.Record.SourceObjectID).
			SetContentHash(spec.Record.BodySHA256).
			SetFreshnessState(evidence.FreshnessStateFresh).
			SetVisibility(evidence.VisibilityPublic).
			SetACLState(evidence.ACLStateCurrent).
			SetConfidence(1)
		applyEvidenceSpecUpdate(updater, spec)
		return updater.Save(ctx)
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query evidence %s: %w", key, err)
	}

	builder := client.Evidence.Create().
		SetKey(key).
		SetClaimKind(spec.ClaimKind).
		SetProofState(evidence.ProofStateCurrent).
		SetSourceSystem(spec.Record.SourceKey).
		SetSourceInstance(spec.Record.SourceInstance).
		SetExternalKind(spec.Record.SourceObjectType).
		SetExternalID(spec.Record.SourceObjectID).
		SetContentHash(spec.Record.BodySHA256).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetACLState(evidence.ACLStateCurrent).
		SetConfidence(1)
	applyEvidenceSpecCreate(builder, spec)
	return builder.Save(ctx)
}

// applyEvidenceSpecCreate attaches locator-grade proof fields to new Evidence rows.
func applyEvidenceSpecCreate(builder *genent.EvidenceCreate, spec evidenceSpec) {
	if spec.ClaimTargetKind != "" {
		builder.SetClaimTargetKind(spec.ClaimTargetKind)
	}
	if spec.ClaimTargetID != 0 {
		builder.SetClaimTargetID(spec.ClaimTargetID)
	}
	if spec.RelationshipKind != "" {
		builder.SetRelationshipKind(spec.RelationshipKind)
	}
	if spec.RelationshipID != 0 {
		builder.SetRelationshipID(spec.RelationshipID)
	}
	if spec.LocatorKind != "" {
		builder.SetLocatorKind(spec.LocatorKind)
	}
	if spec.Locator != "" {
		builder.SetLocator(spec.Locator)
		builder.SetSourceURL(spec.Locator)
	}
	if spec.SourceSpanKey != "" {
		builder.SetSourceSpanKey(spec.SourceSpanKey)
	}
	if spec.Excerpt != "" {
		builder.SetExcerpt(truncateEvidenceExcerpt(spec.Excerpt))
		builder.SetTextHash(sourcecapture.HashBody([]byte(spec.Excerpt)))
	}
	if !spec.ObservedAt.IsZero() {
		builder.SetObservedAt(spec.ObservedAt)
		builder.SetLastConfirmedAt(spec.ObservedAt)
		builder.SetFreshnessCheckedAt(spec.ObservedAt)
	}
}

// applyEvidenceSpecUpdate refreshes proof fields without changing Evidence identity.
func applyEvidenceSpecUpdate(builder *genent.EvidenceUpdateOne, spec evidenceSpec) {
	if spec.ClaimTargetKind != "" {
		builder.SetClaimTargetKind(spec.ClaimTargetKind)
	}
	if spec.ClaimTargetID != 0 {
		builder.SetClaimTargetID(spec.ClaimTargetID)
	}
	if spec.RelationshipKind != "" {
		builder.SetRelationshipKind(spec.RelationshipKind)
	}
	if spec.RelationshipID != 0 {
		builder.SetRelationshipID(spec.RelationshipID)
	}
	if spec.LocatorKind != "" {
		builder.SetLocatorKind(spec.LocatorKind)
	}
	if spec.Locator != "" {
		builder.SetLocator(spec.Locator)
		builder.SetSourceURL(spec.Locator)
	}
	if spec.SourceSpanKey != "" {
		builder.SetSourceSpanKey(spec.SourceSpanKey)
	}
	if spec.Excerpt != "" {
		builder.SetExcerpt(truncateEvidenceExcerpt(spec.Excerpt))
		builder.SetTextHash(sourcecapture.HashBody([]byte(spec.Excerpt)))
	}
	if !spec.ObservedAt.IsZero() {
		builder.SetObservedAt(spec.ObservedAt)
		builder.SetLastConfirmedAt(spec.ObservedAt)
		builder.SetFreshnessCheckedAt(spec.ObservedAt)
	}
}

// truncateEvidenceExcerpt keeps evidence snippets bounded while preserving the locator.
func truncateEvidenceExcerpt(excerpt string) string {
	if len(excerpt) <= 512 {
		return excerpt
	}
	return excerpt[:512]
}

// ensureFixtureLensWindow gives the replay a bounded pull-request view window.
func ensureFixtureLensWindow(ctx context.Context, client *genent.Client, streamKey string, displayName string, resultCount int, complete bool, now time.Time) (*genent.WorkLensWindow, error) {
	personRow, err := ensureFixturePerson(ctx, client, streamKey)
	if err != nil {
		return nil, err
	}
	area, err := ensureFixtureWorkArea(ctx, client, personRow.ID, streamKey)
	if err != nil {
		return nil, err
	}
	lens, err := ensureFixtureWorkLens(ctx, client, area.ID, streamKey, displayName, resultCount, complete, now)
	if err != nil {
		return nil, err
	}
	key := "work-lens-window:" + streamKey + ":source"
	existing, err := client.WorkLensWindow.Query().Where(worklenswindow.KeyEQ(key)).Only(ctx)
	if err == nil {
		return existing.Update().
			SetResultCount(resultCount).
			SetIsComplete(complete).
			SetLastIndexedAt(now).
			SetCheckpoint("fixture:" + streamKey + ":records").
			SetFreshnessState(worklenswindow.FreshnessStatePartial).
			SetVisibility(worklenswindow.VisibilityPublic).
			SetConfidence(1).
			SetLastActivityAt(now).
			Save(ctx)
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture work lens window: %w", err)
	}
	return client.WorkLensWindow.Create().
		SetKey(key).
		SetWorkLensID(lens.ID).
		SetLensWindowKind(worklenswindow.LensWindowKindSource).
		SetCheckpoint("fixture:" + streamKey + ":records").
		SetResultCount(resultCount).
		SetIsComplete(complete).
		SetLastIndexedAt(now).
		SetSourceSystem(fixtureSourceSystem).
		SetSourceInstance(streamKey).
		SetExternalKind(fixtureExternalKind).
		SetExternalID(streamKey).
		SetFreshnessState(worklenswindow.FreshnessStatePartial).
		SetVisibility(worklenswindow.VisibilityPublic).
		SetConfidence(1).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		Save(ctx)
}

// ensureFixturePerson creates the synthetic owner needed for WorkArea anchoring.
func ensureFixturePerson(ctx context.Context, client *genent.Client, streamKey string) (*genent.Person, error) {
	key := "person:fixture:" + streamKey
	row, err := client.Person.Query().Where(person.KeyEQ(key)).Only(ctx)
	if err == nil {
		return row, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture person: %w", err)
	}
	return client.Person.Create().
		SetKey(key).
		SetDisplayName("Flink fixture owner").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		Save(ctx)
}

// ensureFixtureWorkArea anchors the fixture lens under a code-focused work area.
func ensureFixtureWorkArea(ctx context.Context, client *genent.Client, personID int, streamKey string) (*genent.WorkArea, error) {
	key := "work-area:fixture:" + streamKey + ":code"
	row, err := client.WorkArea.Query().Where(workarea.KeyEQ(key)).Only(ctx)
	if err == nil {
		return row, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture work area: %w", err)
	}
	return client.WorkArea.Create().
		SetKey(key).
		SetPersonID(personID).
		SetWorkAreaKind(workarea.WorkAreaKindCode).
		SetDisplayName("Code").
		SetFreshnessState(workarea.FreshnessStateFresh).
		SetVisibility(workarea.VisibilityPublic).
		Save(ctx)
}

// ensureFixtureWorkLens describes the replay as a pull-request lens over the work area.
func ensureFixtureWorkLens(ctx context.Context, client *genent.Client, workAreaID int, streamKey string, displayName string, resultCount int, complete bool, now time.Time) (*genent.WorkLens, error) {
	key := "work-lens:fixture:" + streamKey + ":pull-requests"
	row, err := client.WorkLens.Query().Where(worklens.KeyEQ(key)).Only(ctx)
	if err == nil {
		return row.Update().
			SetResultCount(resultCount).
			SetSourceCount(4).
			SetIsComplete(complete).
			SetLastIndexedAt(now).
			SetFreshnessState(worklens.FreshnessStatePartial).
			SetVisibility(worklens.VisibilityPublic).
			Save(ctx)
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture work lens: %w", err)
	}
	return client.WorkLens.Create().
		SetKey(key).
		SetWorkAreaID(workAreaID).
		SetWorkLensKind(worklens.WorkLensKindPullRequestsAuthored).
		SetLensTargetKind(worklens.LensTargetKindPullRequest).
		SetDisplayName(displayName + " pull requests").
		SetResultCount(resultCount).
		SetSourceCount(4).
		SetIsComplete(complete).
		SetLastIndexedAt(now).
		SetFreshnessState(worklens.FreshnessStatePartial).
		SetVisibility(worklens.VisibilityPublic).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		Save(ctx)
}

// completePRBundles admits PRs only when every required GitHub endpoint succeeded.
func completePRBundles(records []sourcecapture.Record) map[string]map[string]sourcecapture.Record {
	grouped := make(map[string]map[string]sourcecapture.Record)
	for _, record := range records {
		if !isPRBundleType(record.SourceObjectType) {
			continue
		}
		if grouped[record.SourceObjectID] == nil {
			grouped[record.SourceObjectID] = make(map[string]sourcecapture.Record)
		}
		if record.Response.StatusCode == 200 {
			grouped[record.SourceObjectID][record.SourceObjectType] = record
		}
	}
	complete := make(map[string]map[string]sourcecapture.Record)
	for objectID, byType := range grouped {
		if hasCompletePRBundle(byType) {
			complete[objectID] = byType
		}
	}
	return complete
}

// hasCompletePRBundle checks the full source coverage contract for one PR.
func hasCompletePRBundle(byType map[string]sourcecapture.Record) bool {
	for _, objectType := range requiredPRBundleTypes {
		if _, ok := byType[objectType]; !ok {
			return false
		}
	}
	return true
}

// isPRBundleType keeps unrelated snapshots out of PR completeness checks.
func isPRBundleType(objectType string) bool {
	for _, required := range requiredPRBundleTypes {
		if objectType == required {
			return true
		}
	}
	return false
}

// sortedBundleKeys makes fixture loading stable across map iteration order.
func sortedBundleKeys(bundles map[string]map[string]sourcecapture.Record) []string {
	keys := make([]string, 0, len(bundles))
	for key := range bundles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// discoveredPullRequests counts hints without turning every hint into a product row.
func discoveredPullRequests(records []sourcecapture.Record) map[PullRequestRef]struct{} {
	discovered := make(map[PullRequestRef]struct{})
	for _, record := range records {
		if record.Response.StatusCode != 200 {
			continue
		}
		switch record.SourceObjectType {
		case "github_pr_search", "github_pr_key_search":
			refs, err := PullRequestsFromGitHubSearch(record)
			if err != nil {
				continue
			}
			for _, ref := range refs {
				discovered[ref] = struct{}{}
			}
		case "jira_remote_links":
			urls, err := PRURLsFromJiraRemoteLinks(record)
			if err != nil {
				continue
			}
			for _, rawURL := range urls {
				if ref, ok := ParseGitHubPRURL(rawURL); ok {
					discovered[ref] = struct{}{}
				}
			}
		case "github_pull_request":
			if ref, ok := parsePRObjectID(record.SourceObjectID); ok {
				discovered[ref] = struct{}{}
			}
		}
	}
	return discovered
}

// parsePRObjectID converts source object IDs like repo/name#123 into PR refs.
func parsePRObjectID(objectID string) (PullRequestRef, bool) {
	repo, numberText, ok := strings.Cut(objectID, "#")
	if !ok {
		return PullRequestRef{}, false
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 || repo == "" {
		return PullRequestRef{}, false
	}
	return PullRequestRef{Repo: repo, Number: number}, true
}

// ticketKey gives Jira tickets a deterministic Cubicle row key.
func ticketKey(key string) string {
	return "ticket:jira:" + strings.ToUpper(strings.TrimSpace(key))
}

// prKey gives GitHub PRs a deterministic Cubicle row key.
func prKey(ref PullRequestRef) string {
	return "pull-request:github:" + prExternalID(ref)
}

// prExternalID keeps the source-native PR identity readable in rows and evidence.
func prExternalID(ref PullRequestRef) string {
	return ref.Repo + "#" + strconv.Itoa(ref.Number)
}

// prURL builds the human locator used when API payloads do not include one.
func prURL(ref PullRequestRef) string {
	return "https://github.com/" + ref.Repo + "/pull/" + strconv.Itoa(ref.Number)
}

// sourceURLForRecord chooses the human source URL before falling back to the API request.
func sourceURLForRecord(record sourcecapture.Record) string {
	if record.SourceURL != "" {
		return record.SourceURL
	}
	return record.Request.URL
}

// nonEmptyPtr avoids writing blank optional source URLs.
func nonEmptyPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// parseJiraTime normalizes Jira timestamps into UTC product freshness fields.
func parseJiraTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

// parseGitHubTime normalizes GitHub timestamps into UTC product freshness fields.
func parseGitHubTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
