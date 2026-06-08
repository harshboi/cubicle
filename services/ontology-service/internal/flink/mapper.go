package flink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

const (
	// AutoscalerSlice is the workstream slice name used by the offline Flink autoscaler fixture.
	AutoscalerSlice = "flink-autoscaler"

	// FlinkFixtureMapperVersion identifies the mapper logic used to produce ontology facts from the fixture.
	FlinkFixtureMapperVersion = "flink-fixture/v1"

	// SourceJira is the fixture source label for Apache Jira issue snapshots.
	SourceJira = "jira"

	// SourceGitHub is the fixture source label for GitHub pull request snapshots.
	SourceGitHub = "github"

	// SourceDocs is the fixture source label for Apache Flink documentation snapshots.
	SourceDocs = "docs"

	// SourcePonyMail is the fixture source label for Apache mailing list snapshots.
	SourcePonyMail = "ponymail"
)

// FlinkFixtureMapper turns the offline Apache Flink fixture snapshots into Cubicle ontology ingest batches.
type FlinkFixtureMapper struct {
	MapperVersion string // MapperVersion records which mapping code produced the emitted ontology facts.
}

// Map groups fixture snapshots by source and emits one ingest batch per source identity.
func (m FlinkFixtureMapper) Map(records []SnapshotRecord, opts MapOptions) ([]domain.IngestBatch, error) {
	if opts.RunKeyPrefix == "" {
		opts.RunKeyPrefix = "run:flink-fixture"
	}
	if opts.ObservedAt.IsZero() {
		opts.ObservedAt = time.Now().UTC()
	}
	if opts.Slice == "" {
		opts.Slice = AutoscalerSlice
	}
	if m.MapperVersion == "" {
		m.MapperVersion = FlinkFixtureMapperVersion
	}

	bySource := groupRecords(records)
	order := []string{SourceJira, SourceGitHub, SourceDocs, SourcePonyMail}
	batches := make([]domain.IngestBatch, 0, len(bySource))
	for _, source := range order {
		group := bySource[source]
		if len(group) == 0 {
			continue
		}
		batch, err := m.mapSource(group, opts)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, nil
}

// mapSource maps all fixture snapshots for one source into a single ingest batch.
func (m FlinkFixtureMapper) mapSource(records []SnapshotRecord, opts MapOptions) (domain.IngestBatch, error) {
	sort.Slice(records, func(i, j int) bool { return records[i].SnapshotKey < records[j].SnapshotKey })
	source := records[0].Source
	sourceInstance := records[0].SourceInstance
	batch := domain.IngestBatch{
		RunKey:         runKey(opts.RunKeyPrefix, source),
		Source:         source,
		SourceInstance: sourceInstance,
		Slice:          opts.Slice,
		MapperVersion:  m.MapperVersion,
		ObservedAt:     opts.ObservedAt,
		Checkpoint: &domain.SourceCheckpointWrite{
			CheckpointKey:   "fixture-manifest",
			CheckpointValue: records[len(records)-1].SnapshotKey,
			UpdatedAt:       opts.ObservedAt,
		},
	}
	for _, record := range records {
		batch.SnapshotKeys = append(batch.SnapshotKeys, record.SnapshotKey)
		batch.Events = append(batch.Events, sourceEvent(record, opts.ObservedAt))
		switch record.Source {
		case SourceJira:
			if err := mapJira(record, &batch, opts.ObservedAt, m.MapperVersion); err != nil {
				return domain.IngestBatch{}, err
			}
		case SourceGitHub:
			if err := mapGitHub(record, &batch, opts.ObservedAt, m.MapperVersion); err != nil {
				return domain.IngestBatch{}, err
			}
		case SourceDocs:
			mapDocs(record, &batch, opts.ObservedAt, m.MapperVersion)
		case SourcePonyMail:
			if err := mapPonyMail(record, &batch, opts.ObservedAt, m.MapperVersion); err != nil {
				return domain.IngestBatch{}, err
			}
		}
	}
	return batch.WithDefaults(), nil
}

func mapJira(record SnapshotRecord, batch *domain.IngestBatch, observedAt time.Time, mapperVersion string) error {
	switch record.SourceObjectType {
	case "jira_search_page":
		var page jiraSearchPage
		if err := json.Unmarshal(record.Body, &page); err != nil {
			return fmt.Errorf("decode Jira search page: %w", err)
		}
		batch.Objects = append(batch.Objects, workstreamObject(record, observedAt, mapperVersion))
		for _, issue := range page.Issues {
			ticket := ticketObject(issue, record, observedAt, mapperVersion)
			action := actionObject(issue.Key, record, observedAt, mapperVersion)
			containsEvidence := evidence(record, "jira-component:"+issue.Key, issue.Fields.Summary, "Jira Autoscaler component includes "+issue.Key, 1, observedAt)
			actionEvidence := evidence(record, "jira-action:"+issue.Key, issue.Fields.Summary, "Ticket still needs action in Jira status "+issue.Fields.Status.Name, 0.85, observedAt)
			batch.Objects = append(batch.Objects, ticket, action)
			batch.Evidence = append(batch.Evidence, containsEvidence, actionEvidence)
			batch.Associations = append(batch.Associations,
				association(domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"}, ticket.Ref(), ontology.AssocContains, containsEvidence, record, observedAt, mapperVersion),
				association(ticket.Ref(), action.Ref(), ontology.AssocNeedsAction, actionEvidence, record, observedAt, mapperVersion),
			)
		}
	case "jira_remote_links":
		var links jiraRemoteLinks
		if err := json.Unmarshal(record.Body, &links); err != nil {
			return fmt.Errorf("decode Jira remote links: %w", err)
		}
		for _, link := range links.Links {
			repo, number, ok := parseGitHubPR(link.URL)
			if !ok {
				continue
			}
			pr := pullRequestObject(repo, number, link.Title, link.URL, record, observedAt, mapperVersion)
			ev := evidence(record, "jira-remote-link:"+links.IssueKey+":pr:"+strconv.Itoa(number), link.URL, "Jira remote link connects "+links.IssueKey+" to PR #"+strconv.Itoa(number), 1, observedAt)
			batch.Objects = append(batch.Objects, pr)
			batch.Evidence = append(batch.Evidence, ev)
			batch.Associations = append(batch.Associations, association(
				domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + links.IssueKey},
				pr.Ref(),
				ontology.AssocImplementedBy,
				ev,
				record,
				observedAt,
				mapperVersion,
			))
		}
	}
	return nil
}

func mapGitHub(record SnapshotRecord, batch *domain.IngestBatch, observedAt time.Time, mapperVersion string) error {
	var pr githubPR
	if err := json.Unmarshal(record.Body, &pr); err != nil {
		return fmt.Errorf("decode GitHub PR: %w", err)
	}
	repo := "apache/flink-kubernetes-operator"
	prObject := pullRequestObject(repo, pr.Number, pr.Title, pr.HTMLURL, record, observedAt, mapperVersion)
	batch.Objects = append(batch.Objects, prObject)
	for _, issueKey := range issueKeys(pr.Title + " " + pr.Body) {
		ev := evidence(record, "github-pr:"+strconv.Itoa(pr.Number)+":issue:"+issueKey, pr.Title, "GitHub PR references "+issueKey, 0.95, observedAt)
		batch.Evidence = append(batch.Evidence, ev)
		batch.Associations = append(batch.Associations, association(
			domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + issueKey},
			prObject.Ref(),
			ontology.AssocImplementedBy,
			ev,
			record,
			observedAt,
			mapperVersion,
		))
	}
	for _, file := range pr.Files {
		codeFile := codeFileObject(repo, file.Filename, record, observedAt, mapperVersion)
		ev := evidence(record, "github-pr:"+strconv.Itoa(pr.Number)+":file:"+file.Filename, file.Filename, "PR changes "+file.Filename, 1, observedAt)
		batch.Objects = append(batch.Objects, codeFile)
		batch.Evidence = append(batch.Evidence, ev)
		batch.Associations = append(batch.Associations, association(prObject.Ref(), codeFile.Ref(), ontology.AssocChangesFile, ev, record, observedAt, mapperVersion))
	}
	return nil
}

func mapDocs(record SnapshotRecord, batch *domain.IngestBatch, observedAt time.Time, mapperVersion string) {
	path := record.SourceObjectID
	doc := domain.Object{
		ObjectType:     ontology.ObjectDocument,
		Key:            "document:apache/flink-kubernetes-operator:" + path,
		Title:          filepath.Base(path),
		SourceURL:      record.SourceURL,
		SnapshotKey:    record.SnapshotKey,
		MapperVersion:  mapperVersion,
		ObservedAt:     observedAt,
		PropertiesJSON: `{"format":"markdown"}`,
	}
	batch.Objects = append(batch.Objects, doc)
	text := string(record.Body)
	for _, issueKey := range issueKeys(text) {
		fragment := domain.Object{
			ObjectType:     ontology.ObjectDocumentFragment,
			Key:            "document_fragment:apache/flink-kubernetes-operator:" + path + "#" + strings.ToLower(issueKey),
			Title:          "Docs mention " + issueKey,
			SourceURL:      record.SourceURL,
			SnapshotKey:    record.SnapshotKey,
			MapperVersion:  mapperVersion,
			ObservedAt:     observedAt,
			PropertiesJSON: `{"issue_key":"` + issueKey + `"}`,
		}
		containsEv := evidence(record, "docs:"+issueKey+":contains", issueKey, "Document contains fragment for "+issueKey, 1, observedAt)
		supportsEv := evidence(record, "docs:"+issueKey+":supports", issueKey, "Docs describe expected behavior for "+issueKey, 0.75, observedAt)
		batch.Objects = append(batch.Objects, fragment)
		batch.Evidence = append(batch.Evidence, containsEv, supportsEv)
		batch.Associations = append(batch.Associations,
			association(doc.Ref(), fragment.Ref(), ontology.AssocContains, containsEv, record, observedAt, mapperVersion),
			association(domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + issueKey}, fragment.Ref(), ontology.AssocSupports, supportsEv, record, observedAt, mapperVersion),
			association(fragment.Ref(), doc.Ref(), ontology.AssocDocuments, containsEv, record, observedAt, mapperVersion),
		)
	}
}

func mapPonyMail(record SnapshotRecord, batch *domain.IngestBatch, observedAt time.Time, mapperVersion string) error {
	var search ponymailSearch
	if err := json.Unmarshal(record.Body, &search); err != nil {
		return fmt.Errorf("decode Pony Mail search: %w", err)
	}
	for _, message := range search.Messages {
		msg := domain.Object{
			ObjectType:     ontology.ObjectMessage,
			Key:            "message:ponymail:" + message.ID,
			Title:          message.Subject,
			ExternalID:     message.ID,
			SourceURL:      record.SourceURL,
			SnapshotKey:    record.SnapshotKey,
			MapperVersion:  mapperVersion,
			ObservedAt:     observedAt,
			PropertiesJSON: `{"list":"dev@flink.apache.org"}`,
		}
		batch.Objects = append(batch.Objects, msg)
		for _, issueKey := range issueKeys(message.Subject + " " + message.Body) {
			ev := evidence(record, "ponymail:"+message.ID+":"+issueKey, message.Subject, "Mailing list discusses "+issueKey, 0.7, observedAt)
			batch.Evidence = append(batch.Evidence, ev)
			batch.Associations = append(batch.Associations, association(
				domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + issueKey},
				msg.Ref(),
				ontology.AssocDiscussedIn,
				ev,
				record,
				observedAt,
				mapperVersion,
			))
		}
	}
	return nil
}

func workstreamObject(record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
	return domain.Object{
		ObjectType:    ontology.ObjectWorkstream,
		Key:           "workstream:flink-autoscaler",
		Title:         "Flink Autoscaler",
		SourceURL:     record.SourceURL,
		SnapshotKey:   record.SnapshotKey,
		MapperVersion: mapperVersion,
		ObservedAt:    observedAt,
	}
}

func ticketObject(issue jiraIssue, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
	return domain.Object{
		ObjectType:     ontology.ObjectTicket,
		Key:            "ticket:" + issue.Key,
		Title:          issue.Fields.Summary,
		ExternalID:     issue.Key,
		SourceURL:      "https://issues.apache.org/jira/browse/" + issue.Key,
		SnapshotKey:    record.SnapshotKey,
		MapperVersion:  mapperVersion,
		ObservedAt:     observedAt,
		PropertiesJSON: `{"status":"` + issue.Fields.Status.Name + `"}`,
	}
}

func actionObject(issueKey string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
	return domain.Object{
		ObjectType:    ontology.ObjectActionCandidate,
		Key:           "action_candidate:" + issueKey + ":follow-up",
		Title:         "Follow up on " + issueKey,
		SourceURL:     "https://issues.apache.org/jira/browse/" + issueKey,
		SnapshotKey:   record.SnapshotKey,
		MapperVersion: mapperVersion,
		ObservedAt:    observedAt,
	}
}

func pullRequestObject(repo string, number int, title, sourceURL string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
	return domain.Object{
		ObjectType:     ontology.ObjectPullRequest,
		Key:            "pr:" + repo + "#" + strconv.Itoa(number),
		Title:          title,
		ExternalID:     repo + "#" + strconv.Itoa(number),
		SourceURL:      sourceURL,
		SnapshotKey:    record.SnapshotKey,
		MapperVersion:  mapperVersion,
		ObservedAt:     observedAt,
		PropertiesJSON: `{"repo":"` + repo + `"}`,
	}
}

func codeFileObject(repo, path string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
	return domain.Object{
		ObjectType:    ontology.ObjectCodeFile,
		Key:           "code_file:" + repo + ":" + path,
		Title:         filepath.Base(path),
		SourceURL:     "https://github.com/" + repo + "/blob/main/" + path,
		SnapshotKey:   record.SnapshotKey,
		MapperVersion: mapperVersion,
		ObservedAt:    observedAt,
	}
}

func association(from, to domain.ObjectRef, associationType domain.AssociationType, ev domain.Evidence, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Association {
	return domain.Association{
		From:            from,
		To:              to,
		AssociationType: associationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    ev.EvidenceKey,
			SourceURL:      record.SourceURL,
			SnapshotKey:    record.SnapshotKey,
			MapperVersion:  mapperVersion,
			Confidence:     ev.Confidence,
			ObservedAt:     observedAt,
			PropertiesJSON: `{"source_object_type":"` + record.SourceObjectType + `"}`,
		},
	}
}

func evidence(record SnapshotRecord, suffix, quoted, summary string, confidence float64, observedAt time.Time) domain.Evidence {
	return domain.Evidence{
		EvidenceKey: "evidence:" + record.Source + ":" + cleanKey(suffix),
		SnapshotKey: record.SnapshotKey,
		SourceURL:   record.SourceURL,
		TextHash:    textHash(record.Source + ":" + suffix + ":" + quoted),
		Summary:     summary,
		QuotedText:  quoted,
		Confidence:  confidence,
		ObservedAt:  observedAt,
	}
}

func sourceEvent(record SnapshotRecord, observedAt time.Time) domain.SourceEvent {
	return domain.SourceEvent{
		EventKey:         "event:" + record.SnapshotKey,
		SnapshotKey:      record.SnapshotKey,
		SourceObjectType: record.SourceObjectType,
		SourceObjectID:   record.SourceObjectID,
		EventType:        "snapshot_observed",
		ObservedAt:       observedAt,
		PayloadJSON:      `{"body_sha256":"` + record.BodySHA256 + `"}`,
	}
}

func groupRecords(records []SnapshotRecord) map[string][]SnapshotRecord {
	groups := make(map[string][]SnapshotRecord)
	for _, record := range records {
		groups[record.Source] = append(groups[record.Source], record)
	}
	return groups
}

func runKey(prefix, source string) string {
	return prefix + ":" + source
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cleanKey(key string) string {
	key = strings.ToLower(key)
	replacer := strings.NewReplacer("/", "-", "#", "-", ":", "-", " ", "-")
	return replacer.Replace(key)
}

func issueKeys(text string) []string {
	matches := regexp.MustCompile(`FLINK-\d+`).FindAllString(text, -1)
	seen := make(map[string]bool)
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			keys = append(keys, match)
		}
	}
	return keys
}

func parseGitHubPR(rawURL string) (string, int, bool) {
	parts := regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/(\d+)`).FindStringSubmatch(rawURL)
	if len(parts) != 3 {
		return "", 0, false
	}
	number, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, false
	}
	return parts[1], number, true
}

// jiraSearchPage mirrors the Jira search response shape captured in the offline Flink fixture.
type jiraSearchPage struct {
	Issues []jiraIssue `json:"issues"` // Issues are the Jira tickets returned by the fixture search payload.
}

// jiraIssue mirrors the Jira issue fields needed by the offline Flink fixture mapper.
type jiraIssue struct {
	Key    string `json:"key"` // Key is the Jira ticket key, such as FLINK-39743.
	Fields struct {
		Summary string `json:"summary"` // Summary is the human-readable Jira issue title.
		Status  struct {
			Name string `json:"name"` // Name is the Jira workflow status label.
		} `json:"status"` // Status is the Jira workflow status object.
	} `json:"fields"` // Fields contains the Jira issue fields consumed by the mapper.
}

// jiraRemoteLinks mirrors Jira remote-link payloads that connect tickets to GitHub PRs.
type jiraRemoteLinks struct {
	IssueKey string `json:"issueKey"` // IssueKey is the Jira ticket that owns the remote links.
	Links    []struct {
		Title string `json:"title"` // Title is the source-provided link title.
		URL   string `json:"url"`   // URL is the remote link target, often a GitHub pull request.
	} `json:"links"` // Links are external references attached to the Jira issue.
}

// githubPR mirrors the GitHub pull request fixture payload consumed by the mapper.
type githubPR struct {
	Number  int    `json:"number"`   // Number is the repository-local pull request number.
	HTMLURL string `json:"html_url"` // HTMLURL is the browser URL used as evidence provenance.
	Title   string `json:"title"`    // Title is the pull request title used for issue-key extraction.
	Body    string `json:"body"`     // Body is the pull request body used for issue-key extraction.
	Files   []struct {
		Filename string `json:"filename"` // Filename is the repository path changed by the pull request.
		Status   string `json:"status"`   // Status is the GitHub file change status captured for debugging.
	} `json:"files"` // Files are the changed files included in the fixture payload.
}

// ponymailSearch mirrors the Apache Pony Mail search response captured in the fixture.
type ponymailSearch struct {
	Messages []struct {
		ID      string `json:"id"`      // ID is the mailing-list message identifier.
		Subject string `json:"subject"` // Subject is searched for Jira issue references.
		Body    string `json:"body"`    // Body is searched for Jira issue references.
	} `json:"messages"` // Messages are mailing-list search hits in the fixture payload.
}
