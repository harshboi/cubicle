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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/messageauthorship"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/personidentity"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/pullrequestauthorship"
	"cubicle/services/ontology-service/ent/pullrequestlensresult"
	"cubicle/services/ontology-service/ent/pullrequestreview"
	"cubicle/services/ontology-service/ent/sourceconnection"
	"cubicle/services/ontology-service/ent/sourcescope"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketassignment"
	"cubicle/services/ontology-service/ent/ticketlensresult"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/unresolvedreference"
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

var secretTokenPattern = regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|xoxb-[A-Za-z0-9-]+)\b`)

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
	MissingPRBundleSnapshots   int `json:"missing_pr_bundle_snapshots"`
	DiscoveredPullRequests     int `json:"discovered_pull_requests"`
	CompletePullRequestBundles int `json:"complete_pull_request_bundles"`
	Tickets                    int `json:"tickets"`
	PullRequests               int `json:"pull_requests"`
	TicketPullRequests         int `json:"ticket_pull_requests"`
	People                     int `json:"people"`
	PersonIdentities           int `json:"person_identities"`
	PullRequestAuthorships     int `json:"pull_request_authorships"`
	PullRequestReviews         int `json:"pull_request_reviews"`
	TicketAssignments          int `json:"ticket_assignments"`
	Messages                   int `json:"messages"`
	TicketMessages             int `json:"ticket_messages"`
	UnresolvedReferences       int `json:"unresolved_references"`
	PullRequestLensResults     int `json:"pull_request_lens_results"`
	TicketLensResults          int `json:"ticket_lens_results"`
	Evidence                   int `json:"evidence"`
	SyncIssues                 int `json:"sync_issues"`
	WorkLensWindows            int `json:"work_lens_windows"`
}

type loadDetailCounts struct {
	People                 int
	PersonIdentities       int
	PullRequestAuthorships int
	PullRequestReviews     int
	TicketAssignments      int
	Messages               int
	TicketMessages         int
	UnresolvedReferences   int
	Evidence               int
}

func (r *LoadResult) addDetails(counts loadDetailCounts) {
	r.People += counts.People
	r.PersonIdentities += counts.PersonIdentities
	r.PullRequestAuthorships += counts.PullRequestAuthorships
	r.PullRequestReviews += counts.PullRequestReviews
	r.TicketAssignments += counts.TicketAssignments
	r.Messages += counts.Messages
	r.TicketMessages += counts.TicketMessages
	r.UnresolvedReferences += counts.UnresolvedReferences
	r.Evidence += counts.Evidence
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
	result.MissingPRBundleSnapshots = countMissingPRBundleSnapshots(records)
	for _, record := range records {
		if record.Response.StatusCode == 200 {
			result.Records200++
			continue
		}
		result.RecordsFailed++
	}
	rowsBefore, err := countFixtureRows(ctx, client)
	if err != nil {
		return LoadResult{}, err
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
		SetCoverageMode(coverageModeFor(result.RecordsFailed + result.MissingPRBundleSnapshots)).
		SetStatus(sourcesyncrun.StatusRunning).
		SetStartedAt(now).
		Save(ctx)
	if err != nil {
		return LoadResult{}, fmt.Errorf("create fixture sync run: %w", err)
	}
	sourceStates, err := ensureFixtureSourceScopeStates(ctx, client, streamKey, displayName, records, now)
	if err != nil {
		return LoadResult{}, err
	}

	if err := materializeSyncIssues(ctx, client, scope.ID, run.ID, records, &result); err != nil {
		return LoadResult{}, err
	}
	if err := materializeMissingPRBundleIssues(ctx, client, scope.ID, run.ID, records, &result); err != nil {
		return LoadResult{}, err
	}

	ticketsByKey := make(map[string]*genent.Ticket)
	for _, record := range records {
		if record.Response.StatusCode != 200 || !isJiraIssueRecordType(record.SourceObjectType) {
			continue
		}
		t, counts, err := materializeJiraIssue(ctx, client, record, now, sourceScopeStateIDForRecord(sourceStates, record))
		if err != nil {
			return LoadResult{}, err
		}
		ticketsByKey[t.Key] = t
		result.Tickets++
		result.addDetails(counts)
	}

	prsByKey := make(map[string]*genent.PullRequest)
	prsByObjectID := make(map[string]*genent.PullRequest)
	prGroups := prBundleRecords(records)
	completeBundles := completePRBundles(records)
	for _, objectID := range sortedRecordKeys(successfulPullRequestRecords(records)) {
		record := prGroups[objectID]["github_pull_request"]
		pr, counts, err := materializePullRequest(ctx, client, record, now, sourceScopeStateIDForRecord(sourceStates, record))
		if err != nil {
			return LoadResult{}, err
		}
		prsByKey[pr.Key] = pr
		prsByObjectID[objectID] = pr
		if _, ok := completeBundles[objectID]; ok {
			result.CompletePullRequestBundles++
		}
		result.addDetails(counts)
	}

	for _, objectID := range sortedRecordKeys(prsByObjectID) {
		counts, err := materializePullRequestIssueLinks(ctx, client, prsByObjectID[objectID], prGroups[objectID], ticketsByKey, now, sourceStates)
		if err != nil {
			return LoadResult{}, err
		}
		result.TicketPullRequests += counts.TicketPullRequests
		result.addDetails(counts.Details)
		detailCounts, err := materializePullRequestParticipation(ctx, client, prsByObjectID[objectID], prGroups[objectID], now)
		if err != nil {
			return LoadResult{}, err
		}
		result.addDetails(detailCounts)
	}

	for _, record := range records {
		if record.Response.StatusCode != 200 || !isJiraRemoteLinksRecordType(record.SourceObjectType) {
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
		edges, evidenceCount, err := materializeJiraRemoteLinks(ctx, client, t, record, now, prsByKey, sourceStates)
		if err != nil {
			return LoadResult{}, err
		}
		result.TicketPullRequests += edges
		result.Evidence += evidenceCount
	}
	for _, record := range records {
		if record.Response.StatusCode != 200 || (record.SourceObjectType != "github_pr_search" && record.SourceObjectType != "github_pr_key_search") {
			continue
		}
		counts, err := materializeGitHubSearchUnresolvedReferences(ctx, client, record, prsByKey, now)
		if err != nil {
			return LoadResult{}, err
		}
		result.addDetails(counts)
	}

	result.PullRequests = len(prsByKey)
	window, err := ensureFixtureLensWindow(ctx, client, streamKey, displayName, result.PullRequests, result.SyncIssues == 0, now)
	if err != nil {
		return LoadResult{}, err
	}
	if window != nil {
		created, err := materializePullRequestLensResults(ctx, client, window, prsByKey, now)
		if err != nil {
			return LoadResult{}, err
		}
		result.PullRequestLensResults = created
		result.WorkLensWindows++
	}
	ticketWindow, err := ensureFixtureTicketLensWindow(ctx, client, streamKey, displayName, result.Tickets, result.SyncIssues == 0, now)
	if err != nil {
		return LoadResult{}, err
	}
	if ticketWindow != nil {
		created, err := materializeTicketLensResults(ctx, client, ticketWindow, ticketsByKey, now)
		if err != nil {
			return LoadResult{}, err
		}
		result.TicketLensResults = created
		result.WorkLensWindows++
	}
	rowsAfter, err := countFixtureRows(ctx, client)
	if err != nil {
		return LoadResult{}, err
	}
	rowDelta := rowsAfter.subtract(rowsBefore)

	status := sourcesyncrun.StatusComplete
	if result.SyncIssues > 0 {
		status = sourcesyncrun.StatusPartial
	}
	if err := run.Update().
		SetCoverageMode(coverageModeFor(result.SyncIssues)).
		SetStatus(status).
		SetCompletedAt(now).
		SetCoverageStartAt(fixtureCoverageWindowStart(records, now)).
		SetCoverageEndAt(fixtureCoverageWindowEnd(records, now)).
		SetCheckpointToken("records:" + strconv.Itoa(result.RecordsSeen)).
		SetObjectsSeenCount(result.DiscoveredPullRequests + result.Tickets + result.People + result.Messages + result.UnresolvedReferences).
		SetObjectsCreatedCount(rowDelta.objects()).
		SetRelationshipsCreatedCount(rowDelta.relationships()).
		SetEvidenceCreatedCount(rowDelta.Evidence).
		SetIssuesCreatedCount(rowDelta.SourceSyncIssues).
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

func fixtureCoverageWindowStart(records []sourcecapture.Record, fallback time.Time) time.Time {
	start, _ := fixtureCoverageWindow(records, fallback)
	return start
}

func fixtureCoverageWindowEnd(records []sourcecapture.Record, fallback time.Time) time.Time {
	_, end := fixtureCoverageWindow(records, fallback)
	return end
}

func fixtureCoverageWindow(records []sourcecapture.Record, fallback time.Time) (time.Time, time.Time) {
	start := time.Time{}
	end := time.Time{}
	for _, record := range records {
		fetchedAt := record.Response.FetchedAt
		if fetchedAt.IsZero() {
			continue
		}
		fetchedAt = fetchedAt.UTC()
		if start.IsZero() || fetchedAt.Before(start) {
			start = fetchedAt
		}
		if end.IsZero() || fetchedAt.After(end) {
			end = fetchedAt
		}
	}
	if start.IsZero() {
		start = fallback
	}
	if end.IsZero() {
		end = fallback
	}
	return start, end
}

type fixtureRowCounts struct {
	Tickets               int
	PullRequests          int
	People                int
	PersonIdentities      int
	Messages              int
	UnresolvedReferences  int
	TicketPullRequests    int
	PullRequestAuthorship int
	PullRequestReviews    int
	TicketAssignments     int
	TicketMessages        int
	MessageAuthorships    int
	PullRequestLensResult int
	TicketLensResult      int
	Evidence              int
	SourceSyncIssues      int
}

func (c fixtureRowCounts) subtract(before fixtureRowCounts) fixtureRowCounts {
	return fixtureRowCounts{
		Tickets:               c.Tickets - before.Tickets,
		PullRequests:          c.PullRequests - before.PullRequests,
		People:                c.People - before.People,
		PersonIdentities:      c.PersonIdentities - before.PersonIdentities,
		Messages:              c.Messages - before.Messages,
		UnresolvedReferences:  c.UnresolvedReferences - before.UnresolvedReferences,
		TicketPullRequests:    c.TicketPullRequests - before.TicketPullRequests,
		PullRequestAuthorship: c.PullRequestAuthorship - before.PullRequestAuthorship,
		PullRequestReviews:    c.PullRequestReviews - before.PullRequestReviews,
		TicketAssignments:     c.TicketAssignments - before.TicketAssignments,
		TicketMessages:        c.TicketMessages - before.TicketMessages,
		MessageAuthorships:    c.MessageAuthorships - before.MessageAuthorships,
		PullRequestLensResult: c.PullRequestLensResult - before.PullRequestLensResult,
		TicketLensResult:      c.TicketLensResult - before.TicketLensResult,
		Evidence:              c.Evidence - before.Evidence,
		SourceSyncIssues:      c.SourceSyncIssues - before.SourceSyncIssues,
	}
}

func (c fixtureRowCounts) objects() int {
	return c.Tickets + c.PullRequests + c.People + c.PersonIdentities + c.Messages + c.UnresolvedReferences
}

func (c fixtureRowCounts) relationships() int {
	return c.TicketPullRequests +
		c.PullRequestAuthorship +
		c.PullRequestReviews +
		c.TicketAssignments +
		c.TicketMessages +
		c.MessageAuthorships +
		c.PullRequestLensResult +
		c.TicketLensResult
}

func countFixtureRows(ctx context.Context, client *genent.Client) (fixtureRowCounts, error) {
	var counts fixtureRowCounts
	var err error
	if counts.Tickets, err = client.Ticket.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count tickets: %w", err)
	}
	if counts.PullRequests, err = client.PullRequest.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count pull requests: %w", err)
	}
	if counts.People, err = client.Person.Query().
		Where(person.Not(person.KeyHasPrefix("person:fixture:"))).
		Count(ctx); err != nil {
		return counts, fmt.Errorf("count people: %w", err)
	}
	if counts.PersonIdentities, err = client.PersonIdentity.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count person identities: %w", err)
	}
	if counts.Messages, err = client.Message.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count messages: %w", err)
	}
	if counts.UnresolvedReferences, err = client.UnresolvedReference.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count unresolved references: %w", err)
	}
	if counts.TicketPullRequests, err = client.TicketPullRequest.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count ticket pull requests: %w", err)
	}
	if counts.PullRequestAuthorship, err = client.PullRequestAuthorship.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count pull request authorships: %w", err)
	}
	if counts.PullRequestReviews, err = client.PullRequestReview.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count pull request reviews: %w", err)
	}
	if counts.TicketAssignments, err = client.TicketAssignment.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count ticket assignments: %w", err)
	}
	if counts.TicketMessages, err = client.TicketMessage.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count ticket messages: %w", err)
	}
	if counts.MessageAuthorships, err = client.MessageAuthorship.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count message authorships: %w", err)
	}
	if counts.PullRequestLensResult, err = client.PullRequestLensResult.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count pull request lens results: %w", err)
	}
	if counts.TicketLensResult, err = client.TicketLensResult.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count ticket lens results: %w", err)
	}
	if counts.Evidence, err = client.Evidence.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count evidence: %w", err)
	}
	if counts.SourceSyncIssues, err = client.SourceSyncIssue.Query().Count(ctx); err != nil {
		return counts, fmt.Errorf("count source sync issues: %w", err)
	}
	return counts, nil
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

type fixtureSourceScopeKey struct {
	sourceSystem   string
	sourceInstance string
}

func ensureFixtureSourceScopeStates(ctx context.Context, client *genent.Client, streamKey string, displayName string, records []sourcecapture.Record, now time.Time) (map[fixtureSourceScopeKey]*genent.SourceScopeState, error) {
	keys := fixtureSourceScopeKeys(records)
	states := make(map[fixtureSourceScopeKey]*genent.SourceScopeState, len(keys))
	for _, key := range keys {
		state, err := ensureFixtureSourceScopeState(ctx, client, streamKey, displayName, key, now)
		if err != nil {
			return nil, err
		}
		states[key] = state
	}
	return states, nil
}

func fixtureSourceScopeKeys(records []sourcecapture.Record) []fixtureSourceScopeKey {
	seen := make(map[fixtureSourceScopeKey]struct{})
	for _, record := range records {
		key := fixtureSourceScopeKey{
			sourceSystem:   strings.TrimSpace(record.SourceKey),
			sourceInstance: strings.TrimSpace(record.SourceInstance),
		}
		if key.sourceSystem == "" || key.sourceInstance == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	keys := make([]fixtureSourceScopeKey, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sourceSystem != keys[j].sourceSystem {
			return keys[i].sourceSystem < keys[j].sourceSystem
		}
		return keys[i].sourceInstance < keys[j].sourceInstance
	})
	return keys
}

func ensureFixtureSourceScopeState(ctx context.Context, client *genent.Client, streamKey string, displayName string, key fixtureSourceScopeKey, now time.Time) (*genent.SourceScopeState, error) {
	conn, err := ensureFixtureSourceConnectionForSource(ctx, client, streamKey, displayName, key, now)
	if err != nil {
		return nil, err
	}
	scope, err := ensureFixtureSourceScopeForSource(ctx, client, conn.ID, streamKey, displayName, key)
	if err != nil {
		return nil, err
	}
	state, err := client.SourceScopeState.Query().Where(sourcescopestate.SourceScopeIDEQ(scope.ID)).Only(ctx)
	if err == nil {
		updated, err := state.Update().
			SetFreshnessState(sourcescopestate.FreshnessStateFresh).
			SetCoverageMode(sourcescopestate.CoverageModePartialScope).
			SetLastAttemptedAt(now).
			ClearLastSuccessfulSyncRun().
			ClearLastSuccessfulAt().
			ClearErrorCode().
			ClearErrorMessage().
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update fixture source scope state %s/%s: %w", key.sourceSystem, key.sourceInstance, err)
		}
		return updated, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture source scope state %s/%s: %w", key.sourceSystem, key.sourceInstance, err)
	}
	return client.SourceScopeState.Create().
		SetScope(scope).
		SetFreshnessState(sourcescopestate.FreshnessStateFresh).
		SetCoverageMode(sourcescopestate.CoverageModePartialScope).
		SetLastAttemptedAt(now).
		Save(ctx)
}

func ensureFixtureSourceConnectionForSource(ctx context.Context, client *genent.Client, streamKey string, displayName string, key fixtureSourceScopeKey, now time.Time) (*genent.SourceConnection, error) {
	connectionKey := "source-connection:" + streamKey + ":" + key.sourceSystem + ":" + key.sourceInstance
	conn, err := client.SourceConnection.Query().Where(sourceconnection.KeyEQ(connectionKey)).Only(ctx)
	if err == nil {
		if err := conn.Update().SetLastSyncedAt(now).Exec(ctx); err != nil {
			return nil, fmt.Errorf("update fixture source connection %s/%s: %w", key.sourceSystem, key.sourceInstance, err)
		}
		return conn, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture source connection %s/%s: %w", key.sourceSystem, key.sourceInstance, err)
	}
	return client.SourceConnection.Create().
		SetKey(connectionKey).
		SetSourceSystem(key.sourceSystem).
		SetSourceInstance(key.sourceInstance).
		SetDisplayName(displayName + " " + key.sourceSystem + " source").
		SetConnectorKind("fixture_replay").
		SetLastSyncedAt(now).
		Save(ctx)
}

func ensureFixtureSourceScopeForSource(ctx context.Context, client *genent.Client, sourceConnectionID int, streamKey string, displayName string, key fixtureSourceScopeKey) (*genent.SourceScope, error) {
	scopeKey := "source-scope:" + streamKey + ":" + key.sourceSystem + ":" + key.sourceInstance
	scope, err := client.SourceScope.Query().Where(sourcescope.KeyEQ(scopeKey)).Only(ctx)
	if err == nil {
		return scope, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture source scope %s/%s: %w", key.sourceSystem, key.sourceInstance, err)
	}
	return client.SourceScope.Create().
		SetKey(scopeKey).
		SetSourceConnectionID(sourceConnectionID).
		SetScopeKind("source_instance").
		SetScopeKey(streamKey + ":" + key.sourceInstance).
		SetDisplayName(displayName + " " + key.sourceSystem + " source").
		SetCrawlPolicy("fixture_replay coverage=partial_source_instance").
		Save(ctx)
}

func sourceScopeStateIDForRecord(states map[fixtureSourceScopeKey]*genent.SourceScopeState, record sourcecapture.Record) int {
	return sourceScopeStateIDForSource(states, record.SourceKey, record.SourceInstance)
}

func sourceScopeStateIDForSource(states map[fixtureSourceScopeKey]*genent.SourceScopeState, sourceSystem string, sourceInstance string) int {
	if len(states) == 0 {
		return 0
	}
	state := states[fixtureSourceScopeKey{
		sourceSystem:   strings.TrimSpace(sourceSystem),
		sourceInstance: strings.TrimSpace(sourceInstance),
	}]
	if state == nil {
		return 0
	}
	return state.ID
}

func sourceScopeStateIDForPullRequestRef(states map[fixtureSourceScopeKey]*genent.SourceScopeState, ref PullRequestRef) int {
	return sourceScopeStateIDForSource(states, SourceGitHub, githubSourceInstance(ref.Repo))
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

// materializeMissingPRBundleIssues marks absent detail endpoints as partial coverage.
func materializeMissingPRBundleIssues(ctx context.Context, client *genent.Client, scopeID int, runID int, records []sourcecapture.Record, result *LoadResult) error {
	for _, missing := range missingPRBundleSnapshots(records) {
		if _, err := client.SourceSyncIssue.Create().
			SetScopeID(scopeID).
			SetSyncRunID(runID).
			SetSeverity(sourcesyncissue.SeverityWarning).
			SetIssueCode("source_missing_snapshot").
			SetMessage(fmt.Sprintf("source snapshot for %s was not captured; treat PR detail as partial, not product absence", missing.ObjectType)).
			SetSourceSystem(SourceGitHub).
			SetSourceInstance(missing.SourceInstance).
			SetExternalKind(missing.ObjectType).
			SetExternalID(missing.ObjectID).
			Save(ctx); err != nil {
			return fmt.Errorf("create missing sync issue for %s %s: %w", missing.ObjectID, missing.ObjectType, err)
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
		Assignee    sourceUser      `json:"assignee"`
		Reporter    sourceUser      `json:"reporter"`
		Status      struct {
			Name string `json:"name"`
		} `json:"status"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		Comment struct {
			Comments []jiraCommentPayload `json:"comments"`
		} `json:"comment"`
		Updated string `json:"updated"`
	} `json:"fields"`
}

type sourceUser struct {
	Login        string            `json:"login"`
	ID           int64             `json:"id"`
	HTMLURL      string            `json:"html_url"`
	AvatarURL    string            `json:"avatar_url"`
	Name         string            `json:"name"`
	Key          string            `json:"key"`
	DisplayName  string            `json:"displayName"`
	EmailAddress string            `json:"emailAddress"`
	Self         string            `json:"self"`
	AvatarURLs   map[string]string `json:"avatarUrls"`
}

type jiraCommentPayload struct {
	ID      string     `json:"id"`
	Self    string     `json:"self"`
	Author  sourceUser `json:"author"`
	Body    string     `json:"body"`
	Created string     `json:"created"`
	Updated string     `json:"updated"`
}

// materializeJiraIssue maps one successful Jira issue snapshot into Ticket + Evidence.
func materializeJiraIssue(ctx context.Context, client *genent.Client, record sourcecapture.Record, now time.Time, sourceScopeStateID int) (*genent.Ticket, loadDetailCounts, error) {
	var payload jiraIssuePayload
	if err := json.Unmarshal(record.Body, &payload); err != nil {
		return nil, loadDetailCounts{}, fmt.Errorf("decode Jira issue %s: %w", record.SnapshotKey, err)
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
		if sourceScopeStateID > 0 {
			updater.SetSourceScopeStateID(sourceScopeStateID)
		}
		if !sourceUpdatedAt.IsZero() {
			updater.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
		}
		t, err = updater.Save(ctx)
		if err != nil {
			return nil, loadDetailCounts{}, fmt.Errorf("update ticket %s: %w", key, err)
		}
		e, err := upsertObjectEvidence(ctx, client, "ticket", t.ID, "jira_issue", record, title, now)
		if err != nil {
			return nil, loadDetailCounts{}, err
		}
		counts, err := materializeJiraIssuePeople(ctx, client, t, payload, record, e, now)
		if err != nil {
			return nil, loadDetailCounts{}, err
		}
		counts.Evidence++
		return t, counts, nil
	}
	if !genent.IsNotFound(err) {
		return nil, loadDetailCounts{}, fmt.Errorf("query ticket %s: %w", key, err)
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
	if sourceScopeStateID > 0 {
		builder.SetSourceScopeStateID(sourceScopeStateID)
	}
	if !sourceUpdatedAt.IsZero() {
		builder.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
	}
	t, err = builder.Save(ctx)
	if err != nil {
		return nil, loadDetailCounts{}, fmt.Errorf("create ticket %s: %w", key, err)
	}
	e, err := upsertObjectEvidence(ctx, client, "ticket", t.ID, "jira_issue", record, title, now)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	counts, err := materializeJiraIssuePeople(ctx, client, t, payload, record, e, now)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	counts.Evidence++
	return t, counts, nil
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

func materializeJiraIssuePeople(ctx context.Context, client *genent.Client, t *genent.Ticket, payload jiraIssuePayload, record sourcecapture.Record, ticketEvidence *genent.Evidence, now time.Time) (loadDetailCounts, error) {
	var counts loadDetailCounts
	for _, assignment := range []struct {
		kind ticketassignment.AssignmentKind
		user sourceUser
	}{
		{kind: ticketassignment.AssignmentKindAssignee, user: payload.Fields.Assignee},
		{kind: ticketassignment.AssignmentKindReporter, user: payload.Fields.Reporter},
	} {
		obs, ok := observedJiraUser(assignment.user)
		if !ok {
			continue
		}
		personRow, personCounts, err := ensureObservedPerson(ctx, client, obs, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(personCounts)
		created, evidenceCreated, err := ensureTicketAssignment(ctx, client, t, personRow, assignment.kind, record, assignment.user.displayNameOrHandle(), now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if created {
			counts.TicketAssignments++
		}
		if evidenceCreated {
			counts.Evidence++
		}
	}
	for _, comment := range payload.Fields.Comment.Comments {
		if strings.TrimSpace(comment.ID) == "" {
			continue
		}
		commentCounts, err := materializeJiraComment(ctx, client, t, comment, record, ticketEvidence, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(commentCounts)
	}
	return counts, nil
}

func (c *loadDetailCounts) add(other loadDetailCounts) {
	c.People += other.People
	c.PersonIdentities += other.PersonIdentities
	c.PullRequestAuthorships += other.PullRequestAuthorships
	c.PullRequestReviews += other.PullRequestReviews
	c.TicketAssignments += other.TicketAssignments
	c.Messages += other.Messages
	c.TicketMessages += other.TicketMessages
	c.UnresolvedReferences += other.UnresolvedReferences
	c.Evidence += other.Evidence
}

type observedPerson struct {
	SourceSystem   string
	SourceInstance string
	ExternalKind   string
	ExternalID     string
	Handle         string
	Email          string
	DisplayName    string
	AvatarURL      string
	SourceURL      string
	GitHubLogin    string
	JiraAccountID  string
}

func observedJiraUser(user sourceUser) (observedPerson, bool) {
	externalID := firstNonEmpty(user.Key, user.Name)
	if externalID == "" {
		return observedPerson{}, false
	}
	return observedPerson{
		SourceSystem:   SourceJira,
		SourceInstance: SourceInstanceJira,
		ExternalKind:   "jira_user",
		ExternalID:     externalID,
		Handle:         firstNonEmpty(user.Name, user.Key),
		Email:          user.EmailAddress,
		DisplayName:    firstNonEmpty(user.DisplayName, user.Name, user.Key),
		AvatarURL:      firstNonEmpty(user.AvatarURL, user.AvatarURLs["48x48"], user.AvatarURLs["32x32"]),
		SourceURL:      user.Self,
		JiraAccountID:  externalID,
	}, true
}

func observedGitHubUser(user sourceUser, fallbackName string, fallbackEmail string) (observedPerson, bool) {
	login := strings.TrimSpace(user.Login)
	if login == "" {
		return observedPerson{}, false
	}
	return observedPerson{
		SourceSystem:   SourceGitHub,
		SourceInstance: "github.com",
		ExternalKind:   "github_user",
		ExternalID:     login,
		Handle:         login,
		Email:          fallbackEmail,
		DisplayName:    firstNonEmpty(fallbackName, login),
		AvatarURL:      user.AvatarURL,
		SourceURL:      user.HTMLURL,
		GitHubLogin:    login,
	}, true
}

func (u sourceUser) displayNameOrHandle() string {
	return firstNonEmpty(u.DisplayName, u.Name, u.Login, u.Key)
}

func ensureObservedPerson(ctx context.Context, client *genent.Client, obs observedPerson, record sourcecapture.Record, now time.Time) (*genent.Person, loadDetailCounts, error) {
	if obs.ExternalID == "" {
		return nil, loadDetailCounts{}, errors.New("observed person external id is required")
	}
	displayName := firstNonEmpty(obs.DisplayName, obs.Handle, obs.Email, obs.ExternalID)
	personRow, created, err := findOrCreatePerson(ctx, client, obs, displayName)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	if err := updatePersonHandles(ctx, personRow, obs, displayName); err != nil {
		return nil, loadDetailCounts{}, err
	}
	personRow, err = client.Person.Get(ctx, personRow.ID)
	if err != nil {
		return nil, loadDetailCounts{}, fmt.Errorf("reload person %s: %w", personRow.Key, err)
	}
	identityRow, identityCreated, err := ensurePersonIdentity(ctx, client, personRow, obs, record, now)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	evidenceCreated, err := refreshIdentityEvidence(ctx, client, identityRow, obs, record, now)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	counts := loadDetailCounts{}
	if created {
		counts.People++
	}
	if identityCreated {
		counts.PersonIdentities++
	}
	if evidenceCreated {
		counts.Evidence++
	}
	return personRow, counts, nil
}

func findOrCreatePerson(ctx context.Context, client *genent.Client, obs observedPerson, displayName string) (*genent.Person, bool, error) {
	for _, query := range personLookupQueries(client, obs, displayName) {
		rows, err := query.All(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("query person identity candidate: %w", err)
		}
		if len(rows) == 1 {
			return rows[0], false, nil
		}
	}
	key := "person:" + obs.SourceSystem + ":" + obs.ExternalID
	row, err := client.Person.Create().
		SetKey(key).
		SetDisplayName(displayName).
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create person %s: %w", key, err)
	}
	return row, true, nil
}

func personLookupQueries(client *genent.Client, obs observedPerson, displayName string) []*genent.PersonQuery {
	var queries []*genent.PersonQuery
	if obs.Email != "" {
		queries = append(queries, client.Person.Query().Where(person.PrimaryEmailEQ(obs.Email)))
	}
	if obs.GitHubLogin != "" {
		queries = append(queries, client.Person.Query().Where(person.GithubLoginEQ(obs.GitHubLogin)))
	}
	if obs.JiraAccountID != "" {
		queries = append(queries, client.Person.Query().Where(person.JiraAccountIDEQ(obs.JiraAccountID)))
	}
	if displayName != "" {
		queries = append(queries, client.Person.Query().Where(person.DisplayNameEQ(displayName)))
	}
	return queries
}

func updatePersonHandles(ctx context.Context, personRow *genent.Person, obs observedPerson, displayName string) error {
	displayName = preferredDisplayName(personRow.DisplayName, displayName, obs.Handle)
	updater := personRow.Update().
		SetDisplayName(displayName).
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic)
	if obs.Email != "" {
		updater.SetPrimaryEmail(obs.Email)
	}
	if obs.GitHubLogin != "" {
		updater.SetGithubLogin(obs.GitHubLogin)
	}
	if obs.JiraAccountID != "" {
		updater.SetJiraAccountID(obs.JiraAccountID)
	}
	if obs.AvatarURL != "" {
		updater.SetAvatarURL(obs.AvatarURL)
	}
	if err := updater.Exec(ctx); err != nil {
		return fmt.Errorf("update person %s handles: %w", personRow.Key, err)
	}
	return nil
}

func preferredDisplayName(existing string, incoming string, handle string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	handle = strings.TrimSpace(handle)
	if existing == "" {
		return incoming
	}
	if incoming == "" {
		return existing
	}
	if incoming == handle && existing != handle {
		return existing
	}
	if existing == handle && incoming != handle {
		return incoming
	}
	return incoming
}

func ensurePersonIdentity(ctx context.Context, client *genent.Client, personRow *genent.Person, obs observedPerson, record sourcecapture.Record, now time.Time) (*genent.PersonIdentity, bool, error) {
	existing, err := client.PersonIdentity.Query().Where(
		personidentity.SourceSystemEQ(obs.SourceSystem),
		personidentity.SourceInstanceEQ(obs.SourceInstance),
		personidentity.ExternalKindEQ(obs.ExternalKind),
		personidentity.ExternalIDEQ(obs.ExternalID),
	).Only(ctx)
	if err == nil {
		updater := existing.Update().
			SetIdentityStatus(personidentity.IdentityStatusActive).
			SetLastSeenAt(now)
		if obs.Handle != "" {
			updater.SetHandle(obs.Handle)
		}
		if obs.Email != "" {
			updater.SetEmail(obs.Email)
		}
		if obs.SourceURL != "" {
			updater.SetSourceURL(obs.SourceURL)
		}
		row, err := updater.Save(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("update person identity %s: %w", obs.ExternalID, err)
		}
		return row, false, nil
	}
	if !genent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query person identity %s: %w", obs.ExternalID, err)
	}
	builder := client.PersonIdentity.Create().
		SetPersonID(personRow.ID).
		SetIdentityStatus(personidentity.IdentityStatusActive).
		SetSourceSystem(obs.SourceSystem).
		SetSourceInstance(obs.SourceInstance).
		SetExternalKind(obs.ExternalKind).
		SetExternalID(obs.ExternalID).
		SetFirstSeenAt(now).
		SetLastSeenAt(now)
	if obs.Handle != "" {
		builder.SetHandle(obs.Handle)
	}
	if obs.Email != "" {
		builder.SetEmail(obs.Email)
	}
	if obs.SourceURL != "" {
		builder.SetSourceURL(obs.SourceURL)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create person identity %s: %w", obs.ExternalID, err)
	}
	return row, true, nil
}

func refreshIdentityEvidence(ctx context.Context, client *genent.Client, identityRow *genent.PersonIdentity, obs observedPerson, record sourcecapture.Record, now time.Time) (bool, error) {
	key := "evidence:person_identity:" + obs.SourceSystem + ":" + obs.ExternalID + ":" + record.BodySHA256
	existed, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query identity evidence %s: %w", key, err)
	}
	e, err := upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:       evidence.ClaimKindIdentity,
		ClaimTargetKind: "person_identity",
		ClaimTargetID:   identityRow.ID,
		LocatorKind:     obs.ExternalKind,
		Locator:         firstNonEmpty(obs.SourceURL, sourceURLForRecord(record)),
		SourceSpanKey:   record.SnapshotKey + ":" + obs.ExternalID,
		Excerpt:         firstNonEmpty(obs.DisplayName, obs.Handle, obs.Email, obs.ExternalID),
		Record:          record,
		ObservedAt:      now,
	})
	if err != nil {
		return false, err
	}
	if err := identityRow.Update().SetLatestEvidenceID(e.ID).SetLastSeenAt(now).Exec(ctx); err != nil {
		return false, fmt.Errorf("update identity evidence pointer: %w", err)
	}
	return !existed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ensureTicketAssignment(ctx context.Context, client *genent.Client, t *genent.Ticket, personRow *genent.Person, kind ticketassignment.AssignmentKind, record sourcecapture.Record, excerpt string, now time.Time) (bool, bool, error) {
	rel, err := client.TicketAssignment.Query().Where(
		ticketassignment.PersonIDEQ(personRow.ID),
		ticketassignment.TicketIDEQ(t.ID),
		ticketassignment.AssignmentKindEQ(kind),
	).Only(ctx)
	created := false
	if err == nil {
		// Refresh below.
	} else if genent.IsNotFound(err) {
		rel, err = client.TicketAssignment.Create().
			SetPersonID(personRow.ID).
			SetTicketID(t.ID).
			SetAssignmentKind(kind).
			SetEvidenceCount(1).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(record.SourceObjectID + "->" + personRow.Key + ":" + kind.String()).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(ticketassignment.FreshnessStateFresh).
			SetVisibility(ticketassignment.VisibilityPublic).
			SetACLState(ticketassignment.ACLStateCurrent).
			SetConfidence(1).
			SetFirstSeenAt(now).
			SetLastActivityAt(now).
			SetLastConfirmedAt(now).
			Save(ctx)
		created = true
	} else {
		return false, false, fmt.Errorf("query ticket assignment %s -> %s: %w", personRow.Key, t.Key, err)
	}
	if err != nil {
		return false, false, fmt.Errorf("create ticket assignment %s -> %s: %w", personRow.Key, t.Key, err)
	}
	e, evidenceCreated, err := upsertParticipationEvidence(ctx, client, "ticket_assignment", kind.String(), rel.ID, record, "jira_issue_user", excerpt, now)
	if err != nil {
		return false, false, err
	}
	if err := rel.Update().
		SetLatestEvidenceID(e.ID).
		SetEvidenceCount(1).
		SetFreshnessState(ticketassignment.FreshnessStateFresh).
		SetLastConfirmedAt(now).
		SetLastActivityAt(now).
		Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update ticket assignment evidence: %w", err)
	}
	return created, evidenceCreated, nil
}

func materializeJiraComment(ctx context.Context, client *genent.Client, t *genent.Ticket, comment jiraCommentPayload, record sourcecapture.Record, ticketEvidence *genent.Evidence, now time.Time) (loadDetailCounts, error) {
	obs, ok := observedJiraUser(comment.Author)
	if !ok {
		return loadDetailCounts{}, nil
	}
	personRow, counts, err := ensureObservedPerson(ctx, client, obs, record, now)
	if err != nil {
		return loadDetailCounts{}, err
	}
	messageRow, messageCreated, messageEvidenceCreated, err := ensureJiraMessage(ctx, client, personRow, comment, record, now)
	if err != nil {
		return loadDetailCounts{}, err
	}
	if messageCreated {
		counts.Messages++
	}
	if messageEvidenceCreated {
		counts.Evidence++
	}
	authCreated, authEvidenceCreated, err := ensureMessageAuthorship(ctx, client, personRow, messageRow, record, comment.Body, now)
	if err != nil {
		return loadDetailCounts{}, err
	}
	if authCreated {
		// Message authorship is useful for traversal but not part of the current LoadResult surface.
	}
	if authEvidenceCreated {
		counts.Evidence++
	}
	ticketMessageCreated, ticketMessageEvidenceCreated, err := ensureTicketMessage(ctx, client, t, messageRow, record, comment.Body, now)
	if err != nil {
		return loadDetailCounts{}, err
	}
	if ticketMessageCreated {
		counts.TicketMessages++
	}
	if ticketMessageEvidenceCreated {
		counts.Evidence++
	}
	_ = ticketEvidence
	return counts, nil
}

func ensureJiraMessage(ctx context.Context, client *genent.Client, personRow *genent.Person, comment jiraCommentPayload, record sourcecapture.Record, now time.Time) (*genent.Message, bool, bool, error) {
	key := "message:jira:" + comment.ID
	sentAt := parseJiraTime(firstNonEmpty(comment.Created, comment.Updated))
	existing, err := client.Message.Query().Where(message.KeyEQ(key)).Only(ctx)
	if err == nil {
		updater := existing.Update().
			SetBody(comment.Body).
			SetAuthorPersonKey(personRow.Key).
			SetSummary(truncateEvidenceExcerpt(comment.Body)).
			SetSearchText(comment.Body).
			SetSourceSystem(SourceJira).
			SetSourceInstance(SourceInstanceJira).
			SetExternalKind("jira_comment").
			SetExternalID(comment.ID).
			SetContentHash(sourcecapture.HashBody([]byte(comment.Body))).
			SetFreshnessState(message.FreshnessStateFresh).
			SetVisibility(message.VisibilityPublic).
			SetACLState(message.ACLStateCurrent).
			SetFreshnessCheckedAt(now).
			SetLastConfirmedAt(now).
			SetLastActivityAt(now)
		if comment.Self != "" {
			updater.SetSourceURL(comment.Self)
		}
		if !sentAt.IsZero() {
			updater.SetSentAt(sentAt).SetSourceUpdatedAt(sentAt).SetLastChangedAt(sentAt)
		}
		row, err := updater.Save(ctx)
		if err != nil {
			return nil, false, false, fmt.Errorf("update Jira message %s: %w", comment.ID, err)
		}
		evidenceCreated, err := refreshMessageEvidence(ctx, client, row, record, comment.Body, now)
		return row, false, evidenceCreated, err
	}
	if !genent.IsNotFound(err) {
		return nil, false, false, fmt.Errorf("query Jira message %s: %w", comment.ID, err)
	}
	builder := client.Message.Create().
		SetKey(key).
		SetBody(comment.Body).
		SetAuthorPersonKey(personRow.Key).
		SetSummary(truncateEvidenceExcerpt(comment.Body)).
		SetSearchText(comment.Body).
		SetSourceSystem(SourceJira).
		SetSourceInstance(SourceInstanceJira).
		SetExternalKind("jira_comment").
		SetExternalID(comment.ID).
		SetContentHash(sourcecapture.HashBody([]byte(comment.Body))).
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetACLState(message.ACLStateCurrent).
		SetConfidence(1).
		SetFreshnessCheckedAt(now).
		SetLastConfirmedAt(now).
		SetFirstSeenAt(now).
		SetLastActivityAt(now)
	if comment.Self != "" {
		builder.SetSourceURL(comment.Self)
	}
	if !sentAt.IsZero() {
		builder.SetSentAt(sentAt).SetSourceUpdatedAt(sentAt).SetLastChangedAt(sentAt)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, false, false, fmt.Errorf("create Jira message %s: %w", comment.ID, err)
	}
	evidenceCreated, err := refreshMessageEvidence(ctx, client, row, record, comment.Body, now)
	return row, true, evidenceCreated, err
}

func refreshMessageEvidence(ctx context.Context, client *genent.Client, messageRow *genent.Message, record sourcecapture.Record, excerpt string, now time.Time) (bool, error) {
	key := "evidence:message:" + messageRow.Key + ":" + sourcecapture.HashBody([]byte(excerpt))
	existed, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query message evidence %s: %w", key, err)
	}
	e, err := upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:       evidence.ClaimKindObjectState,
		ClaimTargetKind: "message",
		ClaimTargetID:   messageRow.ID,
		LocatorKind:     "jira_comment",
		Locator:         sourceURLForRecord(record),
		SourceSpanKey:   record.SnapshotKey + ":" + messageRow.ExternalID,
		Excerpt:         excerpt,
		Record:          record,
		ObservedAt:      now,
	})
	if err != nil {
		return false, err
	}
	if err := messageRow.Update().SetLatestEvidenceID(e.ID).SetEvidenceCount(1).Exec(ctx); err != nil {
		return false, fmt.Errorf("update message evidence pointer: %w", err)
	}
	return !existed, nil
}

func ensureMessageAuthorship(ctx context.Context, client *genent.Client, personRow *genent.Person, messageRow *genent.Message, record sourcecapture.Record, excerpt string, now time.Time) (bool, bool, error) {
	rel, err := client.MessageAuthorship.Query().Where(
		messageauthorship.PersonIDEQ(personRow.ID),
		messageauthorship.MessageIDEQ(messageRow.ID),
		messageauthorship.AuthorshipKindEQ(messageauthorship.AuthorshipKindAuthor),
	).Only(ctx)
	created := false
	if err == nil {
		// Refresh below.
	} else if genent.IsNotFound(err) {
		rel, err = client.MessageAuthorship.Create().
			SetPersonID(personRow.ID).
			SetMessageID(messageRow.ID).
			SetAuthorshipKind(messageauthorship.AuthorshipKindAuthor).
			SetEvidenceCount(1).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind("jira_comment_author").
			SetExternalID(messageRow.ExternalID + "->" + personRow.Key).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(messageauthorship.FreshnessStateFresh).
			SetVisibility(messageauthorship.VisibilityPublic).
			SetACLState(messageauthorship.ACLStateCurrent).
			SetConfidence(1).
			SetFirstSeenAt(now).
			SetLastActivityAt(now).
			SetLastConfirmedAt(now).
			Save(ctx)
		created = true
	} else {
		return false, false, fmt.Errorf("query message authorship %s -> %s: %w", personRow.Key, messageRow.Key, err)
	}
	if err != nil {
		return false, false, fmt.Errorf("create message authorship %s -> %s: %w", personRow.Key, messageRow.Key, err)
	}
	e, evidenceCreated, err := upsertParticipationEvidence(ctx, client, "message_authorship", messageauthorship.AuthorshipKindAuthor.String(), rel.ID, record, "jira_comment_author", excerpt, now)
	if err != nil {
		return false, false, err
	}
	if err := rel.Update().SetLatestEvidenceID(e.ID).SetEvidenceCount(1).SetLastConfirmedAt(now).SetLastActivityAt(now).Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update message authorship evidence: %w", err)
	}
	return created, evidenceCreated, nil
}

func ensureTicketMessage(ctx context.Context, client *genent.Client, t *genent.Ticket, messageRow *genent.Message, record sourcecapture.Record, excerpt string, now time.Time) (bool, bool, error) {
	rel, err := client.TicketMessage.Query().Where(
		ticketmessage.TicketIDEQ(t.ID),
		ticketmessage.MessageIDEQ(messageRow.ID),
		ticketmessage.TicketMessageKindEQ(ticketmessage.TicketMessageKindDiscussedIn),
	).Only(ctx)
	created := false
	if err == nil {
		// Refresh below.
	} else if genent.IsNotFound(err) {
		rel, err = client.TicketMessage.Create().
			SetTicketID(t.ID).
			SetMessageID(messageRow.ID).
			SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
			SetEvidenceCount(1).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind("jira_comment_ticket").
			SetExternalID(t.ExternalID + "->" + messageRow.ExternalID).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(ticketmessage.FreshnessStateFresh).
			SetVisibility(ticketmessage.VisibilityPublic).
			SetACLState(ticketmessage.ACLStateCurrent).
			SetConfidence(1).
			SetFirstSeenAt(now).
			SetLastActivityAt(now).
			SetLastConfirmedAt(now).
			Save(ctx)
		created = true
	} else {
		return false, false, fmt.Errorf("query ticket message %s -> %s: %w", t.Key, messageRow.Key, err)
	}
	if err != nil {
		return false, false, fmt.Errorf("create ticket message %s -> %s: %w", t.Key, messageRow.Key, err)
	}
	e, evidenceCreated, err := upsertParticipationEvidence(ctx, client, "ticket_message", ticketmessage.TicketMessageKindDiscussedIn.String(), rel.ID, record, "jira_comment", excerpt, now)
	if err != nil {
		return false, false, err
	}
	if err := rel.Update().SetLatestEvidenceID(e.ID).SetEvidenceCount(1).SetLastConfirmedAt(now).SetLastActivityAt(now).Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update ticket message evidence: %w", err)
	}
	return created, evidenceCreated, nil
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
	HTMLURL            string       `json:"html_url"`
	Title              string       `json:"title"`
	Body               string       `json:"body"`
	State              string       `json:"state"`
	CreatedAt          string       `json:"created_at"`
	ClosedAt           string       `json:"closed_at"`
	MergedAt           string       `json:"merged_at"`
	Number             int          `json:"number"`
	UpdatedAt          string       `json:"updated_at"`
	Additions          *int         `json:"additions"`
	Deletions          *int         `json:"deletions"`
	ChangedFiles       *int         `json:"changed_files"`
	Commits            *int         `json:"commits"`
	Comments           *int         `json:"comments"`
	ReviewComments     *int         `json:"review_comments"`
	Draft              *bool        `json:"draft"`
	Mergeable          *bool        `json:"mergeable"`
	User               sourceUser   `json:"user"`
	RequestedReviewers []sourceUser `json:"requested_reviewers"`
	Head               struct {
		Ref  string     `json:"ref"`
		User sourceUser `json:"user"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

// materializePullRequest creates a PullRequest from a successful GitHub PR detail snapshot.
func materializePullRequest(ctx context.Context, client *genent.Client, record sourcecapture.Record, now time.Time, sourceScopeStateID int) (*genent.PullRequest, loadDetailCounts, error) {
	var payload githubPullRequestPayload
	if err := json.Unmarshal(record.Body, &payload); err != nil {
		return nil, loadDetailCounts{}, fmt.Errorf("decode GitHub pull request %s: %w", record.SnapshotKey, err)
	}
	ref := PullRequestRef{Repo: payload.Base.Repo.FullName, Number: payload.Number}
	if ref.Repo == "" || ref.Number == 0 {
		parsed, ok := parsePRObjectID(record.SourceObjectID)
		if !ok {
			return nil, loadDetailCounts{}, fmt.Errorf("pull request snapshot %s missing repo/number", record.SnapshotKey)
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
	sourceCreatedAt := parseGitHubTime(payload.CreatedAt)
	closedAt := parseGitHubTime(payload.ClosedAt)
	mergedAt := parseGitHubTime(payload.MergedAt)

	existing, err := client.PullRequest.Query().Where(pullrequest.KeyEQ(prKey(ref))).Only(ctx)
	if err == nil {
		updater := existing.Update().
			SetRepository(ref.Repo).
			SetNumber(ref.Number).
			SetTitle(title).
			SetState(normalizePRState(payload.State, mergedAt)).
			SetSummary(title).
			SetSearchText(strings.TrimSpace(title + "\n" + payload.Body)).
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
		if !sourceCreatedAt.IsZero() {
			updater.SetSourceCreatedAt(sourceCreatedAt)
		}
		if !sourceUpdatedAt.IsZero() {
			updater.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
		}
		if !mergedAt.IsZero() {
			updater.SetMergedAt(mergedAt)
		}
		if !closedAt.IsZero() {
			updater.SetClosedAt(closedAt)
		}
		if sourceScopeStateID > 0 {
			updater.SetSourceScopeStateID(sourceScopeStateID)
		}
		setPullRequestMetricUpdates(updater, payload)
		pr, err := updater.Save(ctx)
		if err != nil {
			return nil, loadDetailCounts{}, err
		}
		counts := loadDetailCounts{}
		if _, err := upsertObjectEvidence(ctx, client, "pull_request", pr.ID, "github_pull_request", record, pr.Title, now); err != nil {
			return nil, loadDetailCounts{}, err
		}
		counts.Evidence++
		return pr, counts, nil
	}
	if !genent.IsNotFound(err) {
		return nil, loadDetailCounts{}, fmt.Errorf("query pull request %s: %w", prExternalID(ref), err)
	}

	builder := client.PullRequest.Create().
		SetKey(prKey(ref)).
		SetRepository(ref.Repo).
		SetNumber(ref.Number).
		SetTitle(title).
		SetState(normalizePRState(payload.State, mergedAt)).
		SetSummary(title).
		SetSearchText(strings.TrimSpace(title + "\n" + payload.Body)).
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
	if !sourceCreatedAt.IsZero() {
		builder.SetSourceCreatedAt(sourceCreatedAt)
	}
	if !sourceUpdatedAt.IsZero() {
		builder.SetSourceUpdatedAt(sourceUpdatedAt).SetLastChangedAt(sourceUpdatedAt)
	}
	if !mergedAt.IsZero() {
		builder.SetMergedAt(mergedAt)
	}
	if !closedAt.IsZero() {
		builder.SetClosedAt(closedAt)
	}
	if sourceScopeStateID > 0 {
		builder.SetSourceScopeStateID(sourceScopeStateID)
	}
	setPullRequestMetricCreate(builder, payload)
	pr, err := builder.Save(ctx)
	if err != nil {
		return nil, loadDetailCounts{}, err
	}
	counts := loadDetailCounts{}
	if _, err := upsertObjectEvidence(ctx, client, "pull_request", pr.ID, "github_pull_request", record, pr.Title, now); err != nil {
		return nil, loadDetailCounts{}, err
	}
	counts.Evidence++
	return pr, counts, nil
}

func setPullRequestMetricUpdates(updater *genent.PullRequestUpdateOne, payload githubPullRequestPayload) {
	if value, ok := nonNegativeInt(payload.Additions); ok {
		updater.SetAdditions(value)
	} else {
		updater.ClearAdditions()
	}
	if value, ok := nonNegativeInt(payload.Deletions); ok {
		updater.SetDeletions(value)
	} else {
		updater.ClearDeletions()
	}
	if value, ok := nonNegativeInt(payload.ChangedFiles); ok {
		updater.SetChangedFilesCount(value)
	} else {
		updater.ClearChangedFilesCount()
	}
	if value, ok := nonNegativeInt(payload.Commits); ok {
		updater.SetCommitCount(value)
	} else {
		updater.ClearCommitCount()
	}
	if value, ok := nonNegativeInt(payload.Comments); ok {
		updater.SetIssueCommentCount(value)
	} else {
		updater.ClearIssueCommentCount()
	}
	if value, ok := nonNegativeInt(payload.ReviewComments); ok {
		updater.SetReviewCommentCount(value)
	} else {
		updater.ClearReviewCommentCount()
	}
	if payload.Draft != nil {
		updater.SetIsDraft(*payload.Draft)
	} else {
		updater.ClearIsDraft()
	}
	if payload.Mergeable != nil {
		updater.SetIsMergeable(*payload.Mergeable)
	} else {
		updater.ClearIsMergeable()
	}
}

func setPullRequestMetricCreate(builder *genent.PullRequestCreate, payload githubPullRequestPayload) {
	if value, ok := nonNegativeInt(payload.Additions); ok {
		builder.SetAdditions(value)
	}
	if value, ok := nonNegativeInt(payload.Deletions); ok {
		builder.SetDeletions(value)
	}
	if value, ok := nonNegativeInt(payload.ChangedFiles); ok {
		builder.SetChangedFilesCount(value)
	}
	if value, ok := nonNegativeInt(payload.Commits); ok {
		builder.SetCommitCount(value)
	}
	if value, ok := nonNegativeInt(payload.Comments); ok {
		builder.SetIssueCommentCount(value)
	}
	if value, ok := nonNegativeInt(payload.ReviewComments); ok {
		builder.SetReviewCommentCount(value)
	}
	if payload.Draft != nil {
		builder.SetIsDraft(*payload.Draft)
	}
	if payload.Mergeable != nil {
		builder.SetIsMergeable(*payload.Mergeable)
	}
}

func nonNegativeInt(value *int) (int, bool) {
	if value == nil || *value < 0 {
		return 0, false
	}
	return *value, true
}

func materializePullRequestPeople(ctx context.Context, client *genent.Client, pr *genent.PullRequest, payload githubPullRequestPayload, record sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var counts loadDetailCounts
	if obs, ok := observedGitHubUser(payload.User, "", ""); ok {
		personRow, personCounts, err := ensureObservedPerson(ctx, client, obs, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(personCounts)
		created, evidenceCreated, err := ensurePullRequestAuthorship(ctx, client, personRow, pr, pullrequestauthorship.AuthorshipKindAuthor, record, payload.Title, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if created {
			counts.PullRequestAuthorships++
		}
		if evidenceCreated {
			counts.Evidence++
		}
	}
	for _, reviewer := range payload.RequestedReviewers {
		obs, ok := observedGitHubUser(reviewer, "", "")
		if !ok {
			continue
		}
		personRow, personCounts, err := ensureObservedPerson(ctx, client, obs, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(personCounts)
		created, evidenceCreated, err := ensurePullRequestReview(ctx, client, personRow, pr, pullrequestreview.ReviewKindRequestedReviewer, record, reviewer.displayNameOrHandle(), now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if created {
			counts.PullRequestReviews++
		}
		if evidenceCreated {
			counts.Evidence++
		}
	}
	return counts, nil
}

func ensurePullRequestAuthorship(ctx context.Context, client *genent.Client, personRow *genent.Person, pr *genent.PullRequest, kind pullrequestauthorship.AuthorshipKind, record sourcecapture.Record, excerpt string, now time.Time) (bool, bool, error) {
	rel, err := client.PullRequestAuthorship.Query().Where(
		pullrequestauthorship.PersonIDEQ(personRow.ID),
		pullrequestauthorship.PullRequestIDEQ(pr.ID),
		pullrequestauthorship.AuthorshipKindEQ(kind),
	).Only(ctx)
	created := false
	if err == nil {
		// Refresh below.
	} else if genent.IsNotFound(err) {
		rel, err = client.PullRequestAuthorship.Create().
			SetPersonID(personRow.ID).
			SetPullRequestID(pr.ID).
			SetAuthorshipKind(kind).
			SetEvidenceCount(1).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(pr.ExternalID + "->" + personRow.Key + ":" + kind.String()).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(pullrequestauthorship.FreshnessStateFresh).
			SetVisibility(pullrequestauthorship.VisibilityPublic).
			SetACLState(pullrequestauthorship.ACLStateCurrent).
			SetConfidence(1).
			SetFirstSeenAt(now).
			SetLastActivityAt(now).
			SetLastConfirmedAt(now).
			Save(ctx)
		created = true
	} else {
		return false, false, fmt.Errorf("query pull-request authorship %s -> %s: %w", personRow.Key, pr.Key, err)
	}
	if err != nil {
		return false, false, fmt.Errorf("create pull-request authorship %s -> %s: %w", personRow.Key, pr.Key, err)
	}
	e, evidenceCreated, err := upsertParticipationEvidence(ctx, client, "pull_request_authorship", kind.String(), rel.ID, record, "github_pull_request_user", excerpt, now)
	if err != nil {
		return false, false, err
	}
	if err := rel.Update().SetLatestEvidenceID(e.ID).SetEvidenceCount(1).SetLastConfirmedAt(now).SetLastActivityAt(now).Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update pull-request authorship evidence: %w", err)
	}
	return created, evidenceCreated, nil
}

func ensurePullRequestReview(ctx context.Context, client *genent.Client, personRow *genent.Person, pr *genent.PullRequest, kind pullrequestreview.ReviewKind, record sourcecapture.Record, excerpt string, now time.Time) (bool, bool, error) {
	rel, err := client.PullRequestReview.Query().Where(
		pullrequestreview.PersonIDEQ(personRow.ID),
		pullrequestreview.PullRequestIDEQ(pr.ID),
		pullrequestreview.ReviewKindEQ(kind),
	).Only(ctx)
	created := false
	if err == nil {
		// Refresh below.
	} else if genent.IsNotFound(err) {
		rel, err = client.PullRequestReview.Create().
			SetPersonID(personRow.ID).
			SetPullRequestID(pr.ID).
			SetReviewKind(kind).
			SetEvidenceCount(1).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(pr.ExternalID + "->" + personRow.Key + ":" + kind.String()).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(pullrequestreview.FreshnessStateFresh).
			SetVisibility(pullrequestreview.VisibilityPublic).
			SetACLState(pullrequestreview.ACLStateCurrent).
			SetConfidence(1).
			SetFirstSeenAt(now).
			SetLastActivityAt(now).
			SetLastConfirmedAt(now).
			Save(ctx)
		created = true
	} else {
		return false, false, fmt.Errorf("query pull-request review %s -> %s: %w", personRow.Key, pr.Key, err)
	}
	if err != nil {
		return false, false, fmt.Errorf("create pull-request review %s -> %s: %w", personRow.Key, pr.Key, err)
	}
	e, evidenceCreated, err := upsertParticipationEvidence(ctx, client, "pull_request_review", kind.String(), rel.ID, record, "github_pull_request_review", excerpt, now)
	if err != nil {
		return false, false, err
	}
	if err := rel.Update().SetLatestEvidenceID(e.ID).SetEvidenceCount(1).SetLastConfirmedAt(now).SetLastActivityAt(now).Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update pull-request review evidence: %w", err)
	}
	return created, evidenceCreated, nil
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

type pullRequestIssueLinkCounts struct {
	TicketPullRequests int
	Details            loadDetailCounts
}

// materializePullRequestIssueLinks promotes PR title/body/head/commit issue keys to typed relationships.
func materializePullRequestIssueLinks(ctx context.Context, client *genent.Client, pr *genent.PullRequest, records map[string]sourcecapture.Record, ticketsByKey map[string]*genent.Ticket, now time.Time, sourceStates map[fixtureSourceScopeKey]*genent.SourceScopeState) (pullRequestIssueLinkCounts, error) {
	if pr == nil || records == nil {
		return pullRequestIssueLinkCounts{}, nil
	}
	var counts pullRequestIssueLinkCounts
	if record, ok := records["github_pull_request"]; ok && record.Response.StatusCode == 200 {
		links, err := issueLinksFromPullRequestRecord(record)
		if err != nil {
			return pullRequestIssueLinkCounts{}, err
		}
		for _, link := range links {
			created, evidenceCreated, err := materializePRIssueLink(ctx, client, pr, ticketsByKey, record, link.IssueKey, link.LocatorKind, link.Excerpt, now, sourceScopeStateIDForRecord(sourceStates, record))
			if err != nil {
				return pullRequestIssueLinkCounts{}, err
			}
			if created {
				counts.TicketPullRequests++
			}
			if evidenceCreated {
				counts.Details.Evidence++
			}
		}
	}
	if record, ok := records["github_pull_request_commits"]; ok && record.Response.StatusCode == 200 {
		links, err := issueLinksFromCommitRecord(record)
		if err != nil {
			return pullRequestIssueLinkCounts{}, err
		}
		for _, link := range links {
			created, evidenceCreated, err := materializePRIssueLink(ctx, client, pr, ticketsByKey, record, link.IssueKey, link.LocatorKind, link.Excerpt, now, sourceScopeStateIDForRecord(sourceStates, record))
			if err != nil {
				return pullRequestIssueLinkCounts{}, err
			}
			if created {
				counts.TicketPullRequests++
			}
			if evidenceCreated {
				counts.Details.Evidence++
			}
		}
	}
	return counts, nil
}

type prIssueLinkEvidence struct {
	IssueKey    string
	LocatorKind string
	Excerpt     string
}

func issueLinksFromPullRequestRecord(record sourcecapture.Record) ([]prIssueLinkEvidence, error) {
	var payload githubPullRequestPayload
	if err := json.Unmarshal(record.Body, &payload); err != nil {
		return nil, fmt.Errorf("decode GitHub pull request issue links %s: %w", record.SnapshotKey, err)
	}
	type sourceText struct {
		locator string
		text    string
	}
	texts := []sourceText{
		{locator: "github_pull_title", text: payload.Title},
		{locator: "github_pull_body", text: payload.Body},
		{locator: "github_pull_head_ref", text: payload.Head.Ref},
		{locator: "github_pull_base_ref", text: payload.Base.Ref},
	}
	var links []prIssueLinkEvidence
	seen := make(map[string]struct{})
	for _, source := range texts {
		for _, key := range ExtractIssueKeys(source.text) {
			dedupe := key + "\x00" + source.locator
			if _, ok := seen[dedupe]; ok {
				continue
			}
			seen[dedupe] = struct{}{}
			links = append(links, prIssueLinkEvidence{
				IssueKey:    key,
				LocatorKind: source.locator,
				Excerpt:     source.text,
			})
		}
	}
	return links, nil
}

func issueLinksFromCommitRecord(record sourcecapture.Record) ([]prIssueLinkEvidence, error) {
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(record.Body, &commits); err != nil {
		return nil, fmt.Errorf("decode GitHub pull request commits %s: %w", record.SnapshotKey, err)
	}
	var links []prIssueLinkEvidence
	seen := make(map[string]struct{})
	for _, commit := range commits {
		for _, key := range ExtractIssueKeys(commit.Commit.Message) {
			dedupe := key + "\x00" + commit.SHA
			if _, ok := seen[dedupe]; ok {
				continue
			}
			seen[dedupe] = struct{}{}
			excerpt := strings.TrimSpace(commit.Commit.Message)
			if commit.SHA != "" {
				excerpt = commit.SHA + ": " + excerpt
			}
			links = append(links, prIssueLinkEvidence{
				IssueKey:    key,
				LocatorKind: "github_commit_message",
				Excerpt:     excerpt,
			})
		}
	}
	return links, nil
}

func materializePRIssueLink(ctx context.Context, client *genent.Client, pr *genent.PullRequest, ticketsByKey map[string]*genent.Ticket, record sourcecapture.Record, issueKey string, locatorKind string, excerpt string, now time.Time, sourceScopeStateID int) (bool, bool, error) {
	t := ticketsByKey[ticketKey(issueKey)]
	if t == nil {
		return false, false, nil
	}
	rel, created, err := ensureTicketPullRequest(ctx, client, t, pr, record, now, sourceScopeStateID)
	if err != nil {
		return false, false, err
	}
	e, evidenceCreated, err := upsertRelationshipEvidenceWithLocator(ctx, client, rel.ID, record, locatorKind, excerpt, now)
	if err != nil {
		return false, false, err
	}
	evidenceCount, err := relationshipEvidenceCount(ctx, client, rel.ID)
	if err != nil {
		return false, false, err
	}
	updater := rel.Update().
		SetLatestEvidenceID(e.ID).
		SetEvidenceCount(evidenceCount).
		SetSourceSystem(record.SourceKey).
		SetSourceInstance(record.SourceInstance).
		SetExternalKind(record.SourceObjectType).
		SetExternalID(record.SourceObjectID + "->" + pr.ExternalID).
		SetContentHash(record.BodySHA256).
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetLastConfirmedAt(now)
	if sourceURL := sourceURLForRecord(record); sourceURL != "" {
		updater.SetSourceURL(sourceURL)
	}
	if sourceScopeStateID > 0 {
		updater.SetSourceScopeStateID(sourceScopeStateID)
	}
	if err := updater.Exec(ctx); err != nil {
		return false, false, fmt.Errorf("update PR issue relationship evidence: %w", err)
	}
	return created, evidenceCreated, nil
}

func materializePullRequestParticipation(ctx context.Context, client *genent.Client, pr *genent.PullRequest, records map[string]sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var counts loadDetailCounts
	if record, ok := records["github_pull_request_commits"]; ok && record.Response.StatusCode == 200 {
		commitCounts, err := materializeCommitIdentities(ctx, client, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(commitCounts)
	}
	if record, ok := records["github_pull_request"]; ok && record.Response.StatusCode == 200 {
		var payload githubPullRequestPayload
		if err := json.Unmarshal(record.Body, &payload); err != nil {
			return loadDetailCounts{}, fmt.Errorf("decode GitHub pull request participation %s: %w", record.SnapshotKey, err)
		}
		prCounts, err := materializePullRequestPeople(ctx, client, pr, payload, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(prCounts)
	}
	if record, ok := records["github_issue_comments"]; ok && record.Response.StatusCode == 200 {
		commentCounts, err := materializeGitHubIssueComments(ctx, client, pr, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(commentCounts)
	}
	if record, ok := records["github_pull_request_review_comments"]; ok && record.Response.StatusCode == 200 {
		commentCounts, err := materializeGitHubReviewComments(ctx, client, pr, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(commentCounts)
	}
	if record, ok := records["github_pull_request_reviews"]; ok && record.Response.StatusCode == 200 {
		reviewCounts, err := materializeGitHubReviews(ctx, client, pr, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(reviewCounts)
	}
	return counts, nil
}

func materializeCommitIdentities(ctx context.Context, client *genent.Client, record sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var commits []struct {
		Author sourceUser `json:"author"`
		Commit struct {
			Author struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(record.Body, &commits); err != nil {
		return loadDetailCounts{}, fmt.Errorf("decode GitHub commit identities %s: %w", record.SnapshotKey, err)
	}
	var counts loadDetailCounts
	for _, commit := range commits {
		obs, ok := observedGitHubUser(commit.Author, commit.Commit.Author.Name, commit.Commit.Author.Email)
		if !ok {
			continue
		}
		_, personCounts, err := ensureObservedPerson(ctx, client, obs, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(personCounts)
	}
	return counts, nil
}

type githubCommentPayload struct {
	ID        int64      `json:"id"`
	HTMLURL   string     `json:"html_url"`
	User      sourceUser `json:"user"`
	Body      string     `json:"body"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
}

func materializeGitHubIssueComments(ctx context.Context, client *genent.Client, pr *genent.PullRequest, record sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var comments []githubCommentPayload
	if err := json.Unmarshal(record.Body, &comments); err != nil {
		return loadDetailCounts{}, fmt.Errorf("decode GitHub issue comments %s: %w", record.SnapshotKey, err)
	}
	var counts loadDetailCounts
	for _, comment := range comments {
		personRow, commentCounts, ok, err := materializeGitHubCommenter(ctx, client, comment.User, pr, record, pullrequestreview.ReviewKindCommenter, comment.Body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if !ok {
			continue
		}
		_ = personRow
		counts.add(commentCounts)
		unresolvedCounts, err := materializeCommentIssueReferences(ctx, client, pr, record, comment.Body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(unresolvedCounts)
	}
	return counts, nil
}

func materializeGitHubReviewComments(ctx context.Context, client *genent.Client, pr *genent.PullRequest, record sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var comments []githubCommentPayload
	if err := json.Unmarshal(record.Body, &comments); err != nil {
		return loadDetailCounts{}, fmt.Errorf("decode GitHub review comments %s: %w", record.SnapshotKey, err)
	}
	var counts loadDetailCounts
	for _, comment := range comments {
		_, commentCounts, ok, err := materializeGitHubCommenter(ctx, client, comment.User, pr, record, pullrequestreview.ReviewKindCommenter, comment.Body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if !ok {
			continue
		}
		counts.add(commentCounts)
		unresolvedCounts, err := materializeCommentIssueReferences(ctx, client, pr, record, comment.Body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(unresolvedCounts)
	}
	return counts, nil
}

func materializeGitHubCommenter(ctx context.Context, client *genent.Client, user sourceUser, pr *genent.PullRequest, record sourcecapture.Record, kind pullrequestreview.ReviewKind, excerpt string, now time.Time) (*genent.Person, loadDetailCounts, bool, error) {
	obs, ok := observedGitHubUser(user, "", "")
	if !ok {
		return nil, loadDetailCounts{}, false, nil
	}
	personRow, counts, err := ensureObservedPerson(ctx, client, obs, record, now)
	if err != nil {
		return nil, loadDetailCounts{}, false, err
	}
	created, evidenceCreated, err := ensurePullRequestReview(ctx, client, personRow, pr, kind, record, excerpt, now)
	if err != nil {
		return nil, loadDetailCounts{}, false, err
	}
	if created {
		counts.PullRequestReviews++
	}
	if evidenceCreated {
		counts.Evidence++
	}
	return personRow, counts, true, nil
}

func materializeGitHubReviews(ctx context.Context, client *genent.Client, pr *genent.PullRequest, record sourcecapture.Record, now time.Time) (loadDetailCounts, error) {
	var reviews []struct {
		ID          int64      `json:"id"`
		User        sourceUser `json:"user"`
		Body        string     `json:"body"`
		State       string     `json:"state"`
		SubmittedAt string     `json:"submitted_at"`
		HTMLURL     string     `json:"html_url"`
	}
	if err := json.Unmarshal(record.Body, &reviews); err != nil {
		return loadDetailCounts{}, fmt.Errorf("decode GitHub reviews %s: %w", record.SnapshotKey, err)
	}
	var counts loadDetailCounts
	for _, review := range reviews {
		obs, ok := observedGitHubUser(review.User, "", "")
		if !ok {
			continue
		}
		personRow, personCounts, err := ensureObservedPerson(ctx, client, obs, record, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(personCounts)
		kind := reviewKindFromState(review.State)
		created, evidenceCreated, err := ensurePullRequestReview(ctx, client, personRow, pr, kind, record, firstNonEmpty(review.Body, review.State), now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if created {
			counts.PullRequestReviews++
		}
		if evidenceCreated {
			counts.Evidence++
		}
		unresolvedCounts, err := materializeCommentIssueReferences(ctx, client, pr, record, review.Body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		counts.add(unresolvedCounts)
	}
	return counts, nil
}

func reviewKindFromState(state string) pullrequestreview.ReviewKind {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "APPROVED":
		return pullrequestreview.ReviewKindApprover
	case "COMMENTED", "CHANGES_REQUESTED":
		return pullrequestreview.ReviewKindCommenter
	default:
		return pullrequestreview.ReviewKindReviewer
	}
}

func materializeCommentIssueReferences(ctx context.Context, client *genent.Client, pr *genent.PullRequest, record sourcecapture.Record, body string, now time.Time) (loadDetailCounts, error) {
	var counts loadDetailCounts
	for _, key := range ExtractIssueKeys(body) {
		created, evidenceCreated, err := ensureUnresolvedIssueReference(ctx, client, pr, record, key, body, now)
		if err != nil {
			return loadDetailCounts{}, err
		}
		if created {
			counts.UnresolvedReferences++
		}
		if evidenceCreated {
			counts.Evidence++
		}
	}
	return counts, nil
}

func materializeGitHubSearchUnresolvedReferences(ctx context.Context, client *genent.Client, record sourcecapture.Record, prsByKey map[string]*genent.PullRequest, now time.Time) (loadDetailCounts, error) {
	var page struct {
		Items []struct {
			HTMLURL     string `json:"html_url"`
			Number      int    `json:"number"`
			Title       string `json:"title"`
			Body        string `json:"body"`
			TextMatches []struct {
				ObjectURL  string `json:"object_url"`
				ObjectType string `json:"object_type"`
				Property   string `json:"property"`
				Fragment   string `json:"fragment"`
			} `json:"text_matches"`
		} `json:"items"`
	}
	if err := json.Unmarshal(record.Body, &page); err != nil {
		return loadDetailCounts{}, fmt.Errorf("decode GitHub search unresolved refs %s: %w", record.SnapshotKey, err)
	}
	var counts loadDetailCounts
	for _, item := range page.Items {
		ref, ok := ParseGitHubPRURL(item.HTMLURL)
		if !ok && item.Number > 0 {
			if parsed, parsedOK := parsePRObjectID(record.SourceObjectID); parsedOK {
				ref = PullRequestRef{Repo: parsed.Repo, Number: item.Number}
				ok = true
			}
		}
		if !ok {
			continue
		}
		pr := prsByKey[prKey(ref)]
		if pr == nil {
			continue
		}
		highConfidence := make(map[string]struct{})
		for _, key := range ExtractIssueKeys(item.Title + "\n" + item.Body) {
			highConfidence[key] = struct{}{}
		}
		for _, textMatch := range item.TextMatches {
			for _, key := range ExtractIssueKeys(textMatch.Fragment) {
				if _, ok := highConfidence[key]; ok {
					continue
				}
				created, evidenceCreated, err := ensureUnresolvedIssueReference(ctx, client, pr, record, key, textMatch.Fragment, now)
				if err != nil {
					return loadDetailCounts{}, err
				}
				if created {
					counts.UnresolvedReferences++
				}
				if evidenceCreated {
					counts.Evidence++
				}
			}
		}
	}
	return counts, nil
}

func ensureUnresolvedIssueReference(ctx context.Context, client *genent.Client, pr *genent.PullRequest, record sourcecapture.Record, issueKey string, excerpt string, now time.Time) (bool, bool, error) {
	if linked, err := issueAlreadyLinkedToPR(ctx, client, pr, issueKey); err != nil {
		return false, false, err
	} else if linked {
		return false, false, nil
	}
	row, err := client.UnresolvedReference.Query().Where(
		unresolvedreference.FromProductKindEQ("pull_request"),
		unresolvedreference.FromProductIDEQ(pr.ID),
		unresolvedreference.ReferenceKindEQ(unresolvedreference.ReferenceKindIssueKey),
		unresolvedreference.RawRefEQ(issueKey),
	).Only(ctx)
	created := false
	if err == nil {
		updater := row.Update().
			SetFromProductKey(pr.Key).
			SetNormalizedRef(issueKey).
			SetResolutionState(unresolvedreference.ResolutionStateUnresolved).
			SetResolver("flink_fixture_issue_key_text").
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(record.SourceObjectID + "->" + issueKey)
		if sourceURL := sourceURLForRecord(record); sourceURL != "" {
			updater.SetSourceURL(sourceURL)
		}
		row, err = updater.Save(ctx)
		if err != nil {
			return false, false, fmt.Errorf("update unresolved reference %s -> %s: %w", pr.Key, issueKey, err)
		}
	} else if genent.IsNotFound(err) {
		builder := client.UnresolvedReference.Create().
			SetFromProductKind("pull_request").
			SetFromProductID(pr.ID).
			SetFromProductKey(pr.Key).
			SetReferenceKind(unresolvedreference.ReferenceKindIssueKey).
			SetRawRef(issueKey).
			SetNormalizedRef(issueKey).
			SetResolutionState(unresolvedreference.ResolutionStateUnresolved).
			SetResolver("flink_fixture_issue_key_text").
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(record.SourceObjectID + "->" + issueKey)
		if sourceURL := sourceURLForRecord(record); sourceURL != "" {
			builder.SetSourceURL(sourceURL)
		}
		row, err = builder.Save(ctx)
		if err != nil {
			return false, false, fmt.Errorf("create unresolved reference %s -> %s: %w", pr.Key, issueKey, err)
		}
		created = true
	} else {
		return false, false, fmt.Errorf("query unresolved reference %s -> %s: %w", pr.Key, issueKey, err)
	}
	evidenceCreated, err := refreshUnresolvedReferenceEvidence(ctx, client, row, record, excerpt, now)
	if err != nil {
		return false, false, err
	}
	return created, evidenceCreated, nil
}

func issueAlreadyLinkedToPR(ctx context.Context, client *genent.Client, pr *genent.PullRequest, issueKey string) (bool, error) {
	t, err := client.Ticket.Query().Where(ticket.KeyEQ(ticketKey(issueKey))).Only(ctx)
	if genent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query linked ticket %s: %w", issueKey, err)
	}
	exists, err := client.TicketPullRequest.Query().Where(
		ticketpullrequest.TicketIDEQ(t.ID),
		ticketpullrequest.PullRequestIDEQ(pr.ID),
		ticketpullrequest.TicketPullRequestKindEQ(ticketpullrequest.TicketPullRequestKindImplementedBy),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query existing ticket-PR link %s -> %s: %w", issueKey, pr.Key, err)
	}
	return exists, nil
}

func refreshUnresolvedReferenceEvidence(ctx context.Context, client *genent.Client, ref *genent.UnresolvedReference, record sourcecapture.Record, excerpt string, now time.Time) (bool, error) {
	key := "evidence:unresolved_reference:" + strconv.Itoa(ref.ID) + ":" + sourcecapture.HashBody([]byte(excerpt))
	existed, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query unresolved reference evidence %s: %w", key, err)
	}
	e, err := upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:       evidence.ClaimKindCandidate,
		ClaimTargetKind: "unresolved_reference",
		ClaimTargetID:   ref.ID,
		LocatorKind:     "source_text_match",
		Locator:         sourceURLForRecord(record),
		SourceSpanKey:   record.SnapshotKey + ":" + ref.RawRef,
		Excerpt:         excerpt,
		Record:          record,
		ObservedAt:      now,
	})
	if err != nil {
		return false, err
	}
	if err := ref.Update().SetLatestEvidenceID(e.ID).Exec(ctx); err != nil {
		return false, fmt.Errorf("update unresolved reference evidence pointer: %w", err)
	}
	return !existed, nil
}

// materializeJiraRemoteLinks turns Jira PR links into TicketPullRequest edges with evidence.
func materializeJiraRemoteLinks(ctx context.Context, client *genent.Client, t *genent.Ticket, record sourcecapture.Record, now time.Time, prsByKey map[string]*genent.PullRequest, sourceStates map[fixtureSourceScopeKey]*genent.SourceScopeState) (int, int, error) {
	prURLs, err := PRURLsFromJiraRemoteLinks(record)
	if err != nil {
		return 0, 0, err
	}
	relationshipSourceScopeStateID := sourceScopeStateIDForRecord(sourceStates, record)
	var edges int
	var evidenceCount int
	for _, rawURL := range prURLs {
		ref, ok := ParseGitHubPRURL(rawURL)
		if !ok {
			continue
		}
		pr := prsByKey[prKey(ref)]
		if pr == nil {
			pr, err = ensureMinimalPullRequest(ctx, client, ref, now, sourceScopeStateIDForPullRequestRef(sourceStates, ref))
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
		rel, created, err := ensureTicketPullRequest(ctx, client, t, pr, record, now, relationshipSourceScopeStateID)
		if err != nil {
			return 0, 0, err
		}
		e, err := upsertRelationshipEvidence(ctx, client, rel.ID, record, rawURL, now)
		if err != nil {
			return 0, 0, err
		}
		evidenceCountForRelationship, err := relationshipEvidenceCount(ctx, client, rel.ID)
		if err != nil {
			return 0, 0, err
		}
		updater := rel.Update().
			SetLatestEvidenceID(e.ID).
			SetEvidenceCount(evidenceCountForRelationship).
			SetSourceSystem(record.SourceKey).
			SetSourceInstance(record.SourceInstance).
			SetExternalKind(record.SourceObjectType).
			SetExternalID(record.SourceObjectID + "->" + pr.ExternalID).
			SetContentHash(record.BodySHA256).
			SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
			SetLastConfirmedAt(now)
		if sourceURL := sourceURLForRecord(record); sourceURL != "" {
			updater.SetSourceURL(sourceURL)
		}
		if relationshipSourceScopeStateID > 0 {
			updater.SetSourceScopeStateID(relationshipSourceScopeStateID)
		}
		if err := updater.Exec(ctx); err != nil {
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
func ensureMinimalPullRequest(ctx context.Context, client *genent.Client, ref PullRequestRef, now time.Time, sourceScopeStateID int) (*genent.PullRequest, error) {
	existing, err := client.PullRequest.Query().Where(pullrequest.KeyEQ(prKey(ref))).Only(ctx)
	if err == nil {
		if sourceScopeStateID > 0 && existing.SourceScopeStateID != sourceScopeStateID {
			updated, err := existing.Update().SetSourceScopeStateID(sourceScopeStateID).Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("update minimal pull request source scope %s: %w", prExternalID(ref), err)
			}
			return updated, nil
		}
		return existing, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query minimal pull request %s: %w", prExternalID(ref), err)
	}
	builder := client.PullRequest.Create().
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
		SetLastConfirmedAt(now)
	if sourceScopeStateID > 0 {
		builder.SetSourceScopeStateID(sourceScopeStateID)
	}
	return builder.Save(ctx)
}

// ensureTicketPullRequest upserts the typed "ticket implemented by PR" relationship.
func ensureTicketPullRequest(ctx context.Context, client *genent.Client, t *genent.Ticket, pr *genent.PullRequest, record sourcecapture.Record, now time.Time, sourceScopeStateID int) (*genent.TicketPullRequest, bool, error) {
	rel, err := client.TicketPullRequest.Query().Where(
		ticketpullrequest.TicketIDEQ(t.ID),
		ticketpullrequest.PullRequestIDEQ(pr.ID),
		ticketpullrequest.TicketPullRequestKindEQ(ticketpullrequest.TicketPullRequestKindImplementedBy),
	).Only(ctx)
	if err == nil {
		if sourceScopeStateID > 0 && rel.SourceScopeStateID != sourceScopeStateID {
			updated, err := rel.Update().SetSourceScopeStateID(sourceScopeStateID).Save(ctx)
			if err != nil {
				return nil, false, fmt.Errorf("update ticket-pull-request source scope %s -> %s: %w", t.Key, pr.Key, err)
			}
			return updated, false, nil
		}
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
	if sourceScopeStateID > 0 {
		builder.SetSourceScopeStateID(sourceScopeStateID)
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
	e, _, err := upsertRelationshipEvidenceWithLocator(ctx, client, relationshipID, record, "jira_remote_link", excerpt, now)
	return e, err
}

func upsertRelationshipEvidenceWithLocator(ctx context.Context, client *genent.Client, relationshipID int, record sourcecapture.Record, locatorKind string, excerpt string, now time.Time) (*genent.Evidence, bool, error) {
	key := "evidence:ticket_pull_request:" + record.SourceObjectID + ":" + sourcecapture.HashBody([]byte(excerpt))
	existed, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Exist(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("query evidence %s: %w", key, err)
	}
	e, err := upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:        evidence.ClaimKindRelationship,
		ClaimTargetKind:  "ticket_pull_request",
		ClaimTargetID:    relationshipID,
		RelationshipKind: ticketpullrequest.TicketPullRequestKindImplementedBy.String(),
		RelationshipID:   relationshipID,
		LocatorKind:      locatorKind,
		Locator:          sourceURLForRecord(record),
		SourceSpanKey:    record.SnapshotKey + ":" + excerpt,
		Excerpt:          excerpt,
		Record:           record,
		ObservedAt:       now,
	})
	if err != nil {
		return nil, false, err
	}
	return e, !existed, nil
}

func relationshipEvidenceCount(ctx context.Context, client *genent.Client, relationshipID int) (int, error) {
	count, err := client.Evidence.Query().Where(
		evidence.ClaimKindEQ(evidence.ClaimKindRelationship),
		evidence.RelationshipIDEQ(relationshipID),
	).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count relationship evidence %d: %w", relationshipID, err)
	}
	return count, nil
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

func upsertParticipationEvidence(ctx context.Context, client *genent.Client, targetKind string, relationshipKind string, relationshipID int, record sourcecapture.Record, locatorKind string, excerpt string, now time.Time) (*genent.Evidence, bool, error) {
	key := "evidence:" + targetKind + ":" + strconv.Itoa(relationshipID) + ":" + sourcecapture.HashBody([]byte(excerpt))
	existed, err := client.Evidence.Query().Where(evidence.KeyEQ(key)).Exist(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("query participation evidence %s: %w", key, err)
	}
	e, err := upsertEvidence(ctx, client, key, evidenceSpec{
		ClaimKind:        evidence.ClaimKindRelationship,
		ClaimTargetKind:  targetKind,
		ClaimTargetID:    relationshipID,
		RelationshipKind: relationshipKind,
		RelationshipID:   relationshipID,
		LocatorKind:      locatorKind,
		Locator:          sourceURLForRecord(record),
		SourceSpanKey:    record.SnapshotKey + ":" + targetKind + ":" + strconv.Itoa(relationshipID),
		Excerpt:          excerpt,
		Record:           record,
		ObservedAt:       now,
	})
	if err != nil {
		return nil, false, err
	}
	return e, !existed, nil
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
	excerpt = redactEvidenceExcerpt(excerpt)
	if len(excerpt) <= 512 {
		return excerpt
	}
	return excerpt[:512]
}

func redactEvidenceExcerpt(excerpt string) string {
	return secretTokenPattern.ReplaceAllString(excerpt, "[REDACTED_TOKEN]")
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

func ensureFixtureTicketLensWindow(ctx context.Context, client *genent.Client, streamKey string, displayName string, resultCount int, complete bool, now time.Time) (*genent.WorkLensWindow, error) {
	personRow, err := ensureFixturePerson(ctx, client, streamKey)
	if err != nil {
		return nil, err
	}
	area, err := ensureFixtureWorkAreaForKind(ctx, client, personRow.ID, streamKey, workarea.WorkAreaKindTickets, "Tickets")
	if err != nil {
		return nil, err
	}
	lens, err := ensureFixtureTicketWorkLens(ctx, client, area.ID, streamKey, displayName, resultCount, complete, now)
	if err != nil {
		return nil, err
	}
	key := "work-lens-window:" + streamKey + ":tickets:source"
	existing, err := client.WorkLensWindow.Query().Where(worklenswindow.KeyEQ(key)).Only(ctx)
	if err == nil {
		return existing.Update().
			SetResultCount(resultCount).
			SetIsComplete(complete).
			SetLastIndexedAt(now).
			SetCheckpoint("fixture:" + streamKey + ":ticket-records").
			SetFreshnessState(worklenswindow.FreshnessStatePartial).
			SetVisibility(worklenswindow.VisibilityPublic).
			SetConfidence(1).
			SetLastActivityAt(now).
			Save(ctx)
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query fixture ticket work lens window: %w", err)
	}
	return client.WorkLensWindow.Create().
		SetKey(key).
		SetWorkLensID(lens.ID).
		SetLensWindowKind(worklenswindow.LensWindowKindSource).
		SetCheckpoint("fixture:" + streamKey + ":ticket-records").
		SetResultCount(resultCount).
		SetIsComplete(complete).
		SetLastIndexedAt(now).
		SetSourceSystem(fixtureSourceSystem).
		SetSourceInstance(streamKey).
		SetExternalKind(fixtureExternalKind).
		SetExternalID(streamKey + ":tickets").
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
	return ensureFixtureWorkAreaForKind(ctx, client, personID, streamKey, workarea.WorkAreaKindCode, "Code")
}

func ensureFixtureWorkAreaForKind(ctx context.Context, client *genent.Client, personID int, streamKey string, kind workarea.WorkAreaKind, displayName string) (*genent.WorkArea, error) {
	key := "work-area:fixture:" + streamKey + ":code"
	if kind != workarea.WorkAreaKindCode {
		key = "work-area:fixture:" + streamKey + ":" + kind.String()
	}
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
		SetWorkAreaKind(kind).
		SetDisplayName(displayName).
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

func ensureFixtureTicketWorkLens(ctx context.Context, client *genent.Client, workAreaID int, streamKey string, displayName string, resultCount int, complete bool, now time.Time) (*genent.WorkLens, error) {
	key := "work-lens:fixture:" + streamKey + ":tickets"
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
		return nil, fmt.Errorf("query fixture ticket work lens: %w", err)
	}
	return client.WorkLens.Create().
		SetKey(key).
		SetWorkAreaID(workAreaID).
		SetWorkLensKind(worklens.WorkLensKindTicketsOwned).
		SetLensTargetKind(worklens.LensTargetKindTicket).
		SetDisplayName(displayName + " tickets").
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

func materializePullRequestLensResults(ctx context.Context, client *genent.Client, window *genent.WorkLensWindow, prsByKey map[string]*genent.PullRequest, now time.Time) (int, error) {
	var created int
	for _, key := range sortedRecordKeys(prsByKey) {
		pr := prsByKey[key]
		existing, err := client.PullRequestLensResult.Query().Where(
			pullrequestlensresult.WorkLensIDEQ(window.WorkLensID),
			pullrequestlensresult.PullRequestIDEQ(pr.ID),
			pullrequestlensresult.RelationKindEQ(pullrequestlensresult.RelationKindAuthored),
		).Only(ctx)
		if err == nil {
			if err := existing.Update().
				SetFreshnessState(pullrequestlensresult.FreshnessStateFresh).
				SetVisibility(pullrequestlensresult.VisibilityPublic).
				SetRankScore(prLensRankScore(pr)).
				SetLastActivityAt(lensActivityAt(pr.SourceUpdatedAt, pr.LastActivityAt, now)).
				SetLastConfirmedAt(now).
				Exec(ctx); err != nil {
				return 0, fmt.Errorf("update pull-request lens result %s: %w", pr.Key, err)
			}
			continue
		}
		if !genent.IsNotFound(err) {
			return 0, fmt.Errorf("query pull-request lens result %s: %w", pr.Key, err)
		}
		builder := client.PullRequestLensResult.Create().
			SetWorkLensID(window.WorkLensID).
			SetWorkLensWindowID(window.ID).
			SetPullRequestID(pr.ID).
			SetRelationKind(pullrequestlensresult.RelationKindAuthored).
			SetEvidenceCount(pr.EvidenceCount).
			SetSourceSystem(fixtureSourceSystem).
			SetSourceInstance(window.SourceInstance).
			SetExternalKind("pull_request_lens_result").
			SetExternalID(pr.ExternalID).
			SetFreshnessState(pullrequestlensresult.FreshnessStateFresh).
			SetVisibility(pullrequestlensresult.VisibilityPublic).
			SetConfidence(pr.Confidence).
			SetRankScore(prLensRankScore(pr)).
			SetFirstSeenAt(now).
			SetLastActivityAt(lensActivityAt(pr.SourceUpdatedAt, pr.LastActivityAt, now)).
			SetLastConfirmedAt(now)
		if pr.LatestEvidenceID != 0 {
			builder.SetLatestEvidenceID(pr.LatestEvidenceID)
		}
		if _, err := builder.Save(ctx); err != nil {
			return 0, fmt.Errorf("create pull-request lens result %s: %w", pr.Key, err)
		}
		created++
	}
	return created, nil
}

func materializeTicketLensResults(ctx context.Context, client *genent.Client, window *genent.WorkLensWindow, ticketsByKey map[string]*genent.Ticket, now time.Time) (int, error) {
	var created int
	for _, key := range sortedRecordKeys(ticketsByKey) {
		t := ticketsByKey[key]
		existing, err := client.TicketLensResult.Query().Where(
			ticketlensresult.WorkLensIDEQ(window.WorkLensID),
			ticketlensresult.TicketIDEQ(t.ID),
			ticketlensresult.RelationKindEQ(ticketlensresult.RelationKindOwned),
		).Only(ctx)
		if err == nil {
			if err := existing.Update().
				SetFreshnessState(ticketlensresult.FreshnessStateFresh).
				SetVisibility(ticketlensresult.VisibilityPublic).
				SetRankScore(ticketLensRankScore(t)).
				SetLastActivityAt(lensActivityAt(t.SourceUpdatedAt, t.LastActivityAt, now)).
				SetLastConfirmedAt(now).
				Exec(ctx); err != nil {
				return 0, fmt.Errorf("update ticket lens result %s: %w", t.Key, err)
			}
			continue
		}
		if !genent.IsNotFound(err) {
			return 0, fmt.Errorf("query ticket lens result %s: %w", t.Key, err)
		}
		builder := client.TicketLensResult.Create().
			SetWorkLensID(window.WorkLensID).
			SetWorkLensWindowID(window.ID).
			SetTicketID(t.ID).
			SetRelationKind(ticketlensresult.RelationKindOwned).
			SetEvidenceCount(t.EvidenceCount).
			SetSourceSystem(fixtureSourceSystem).
			SetSourceInstance(window.SourceInstance).
			SetExternalKind("ticket_lens_result").
			SetExternalID(t.ExternalID).
			SetFreshnessState(ticketlensresult.FreshnessStateFresh).
			SetVisibility(ticketlensresult.VisibilityPublic).
			SetConfidence(t.Confidence).
			SetRankScore(ticketLensRankScore(t)).
			SetFirstSeenAt(now).
			SetLastActivityAt(lensActivityAt(t.SourceUpdatedAt, t.LastActivityAt, now)).
			SetLastConfirmedAt(now)
		if t.LatestEvidenceID != 0 {
			builder.SetLatestEvidenceID(t.LatestEvidenceID)
		}
		if _, err := builder.Save(ctx); err != nil {
			return 0, fmt.Errorf("create ticket lens result %s: %w", t.Key, err)
		}
		created++
	}
	return created, nil
}

func prLensRankScore(pr *genent.PullRequest) float64 {
	score := pr.Confidence
	if pr.FreshnessState == pullrequest.FreshnessStateFresh {
		score += 1
	}
	if pr.State == pullrequest.StateOpen {
		score += 0.5
	}
	return score
}

func ticketLensRankScore(t *genent.Ticket) float64 {
	score := t.Confidence
	if t.FreshnessState == ticket.FreshnessStateFresh {
		score += 1
	}
	if t.Status == ticket.StatusOpen {
		score += 0.5
	}
	return score
}

func lensActivityAt(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

// completePRBundles admits PRs only when every required GitHub endpoint succeeded.
func completePRBundles(records []sourcecapture.Record) map[string]map[string]sourcecapture.Record {
	grouped := prBundleRecords(records)
	complete := make(map[string]map[string]sourcecapture.Record)
	for objectID, byType := range grouped {
		if hasCompletePRBundle(byType) {
			complete[objectID] = byType
		}
	}
	return complete
}

type missingPRBundleSnapshot struct {
	ObjectID       string
	ObjectType     string
	SourceInstance string
}

func countMissingPRBundleSnapshots(records []sourcecapture.Record) int {
	return len(missingPRBundleSnapshots(records))
}

func missingPRBundleSnapshots(records []sourcecapture.Record) []missingPRBundleSnapshot {
	grouped := prBundleRecords(records)
	pullRecords := successfulPullRequestRecords(records)
	missing := make([]missingPRBundleSnapshot, 0)
	for _, objectID := range sortedRecordKeys(pullRecords) {
		byType := grouped[objectID]
		for _, objectType := range requiredPRBundleTypes {
			if _, ok := byType[objectType]; ok {
				continue
			}
			missing = append(missing, missingPRBundleSnapshot{
				ObjectID:       objectID,
				ObjectType:     objectType,
				SourceInstance: pullRecords[objectID].SourceInstance,
			})
		}
	}
	return missing
}

func prBundleRecords(records []sourcecapture.Record) map[string]map[string]sourcecapture.Record {
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
	return grouped
}

func successfulPullRequestRecords(records []sourcecapture.Record) map[string]sourcecapture.Record {
	successful := make(map[string]sourcecapture.Record)
	for _, record := range records {
		if record.SourceObjectType == "github_pull_request" && record.Response.StatusCode == 200 {
			successful[record.SourceObjectID] = record
		}
	}
	return successful
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

func sortedRecordKeys[V any](records map[string]V) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isJiraIssueRecordType(objectType string) bool {
	return objectType == "jira_issue" || objectType == "jira_correlation_issue"
}

func isJiraRemoteLinksRecordType(objectType string) bool {
	return objectType == "jira_remote_links" || objectType == "jira_correlation_remote_links"
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
		case "jira_remote_links", "jira_correlation_remote_links":
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
