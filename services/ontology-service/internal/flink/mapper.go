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
	AutoscalerSlice      = "flink-autoscaler"
	FixtureMapperVersion = "flink-fixture/v1"

	SourceJira     = "jira"
	SourceGitHub   = "github"
	SourceDocs     = "docs"
	SourcePonyMail = "ponymail"
)

type Mapper struct {
	MapperVersion string
}

func (m Mapper) Map(records []SnapshotRecord, opts MapOptions) ([]domain.IngestBatch, error) {
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
		m.MapperVersion = FixtureMapperVersion
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

func (m Mapper) mapSource(records []SnapshotRecord, opts MapOptions) (domain.IngestBatch, error) {
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
	switch record.SourceObjectKind {
	case "jira_search_page":
		var page jiraSearchPage
		if err := json.Unmarshal(record.Body, &page); err != nil {
			return fmt.Errorf("decode Jira search page: %w", err)
		}
		batch.Objects = append(batch.Objects, workstreamNode(record, observedAt, mapperVersion))
		for _, issue := range page.Issues {
			ticket := ticketNode(issue, record, observedAt, mapperVersion)
			action := actionNode(issue.Key, record, observedAt, mapperVersion)
			containsEvidence := evidence(record, "jira-component:"+issue.Key, issue.Fields.Summary, "Jira Autoscaler component includes "+issue.Key, 1, observedAt)
			actionEvidence := evidence(record, "jira-action:"+issue.Key, issue.Fields.Summary, "Ticket still needs action in Jira status "+issue.Fields.Status.Name, 0.85, observedAt)
			batch.Objects = append(batch.Objects, ticket, action)
			batch.Evidence = append(batch.Evidence, containsEvidence, actionEvidence)
			batch.Associations = append(batch.Associations,
				edge(domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"}, ticket.Ref(), ontology.AssocContains, containsEvidence, record, observedAt, mapperVersion),
				edge(ticket.Ref(), action.Ref(), ontology.AssocNeedsAction, actionEvidence, record, observedAt, mapperVersion),
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
			pr := prNode(repo, number, link.Title, link.URL, record, observedAt, mapperVersion)
			ev := evidence(record, "jira-remote-link:"+links.IssueKey+":pr:"+strconv.Itoa(number), link.URL, "Jira remote link connects "+links.IssueKey+" to PR #"+strconv.Itoa(number), 1, observedAt)
			batch.Objects = append(batch.Objects, pr)
			batch.Evidence = append(batch.Evidence, ev)
			batch.Associations = append(batch.Associations, edge(
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
	prNode := prNode(repo, pr.Number, pr.Title, pr.HTMLURL, record, observedAt, mapperVersion)
	batch.Objects = append(batch.Objects, prNode)
	for _, issueKey := range issueKeys(pr.Title + " " + pr.Body) {
		ev := evidence(record, "github-pr:"+strconv.Itoa(pr.Number)+":issue:"+issueKey, pr.Title, "GitHub PR references "+issueKey, 0.95, observedAt)
		batch.Evidence = append(batch.Evidence, ev)
		batch.Associations = append(batch.Associations, edge(
			domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + issueKey},
			prNode.Ref(),
			ontology.AssocImplementedBy,
			ev,
			record,
			observedAt,
			mapperVersion,
		))
	}
	for _, file := range pr.Files {
		codeFile := codeFileNode(repo, file.Filename, record, observedAt, mapperVersion)
		ev := evidence(record, "github-pr:"+strconv.Itoa(pr.Number)+":file:"+file.Filename, file.Filename, "PR changes "+file.Filename, 1, observedAt)
		batch.Objects = append(batch.Objects, codeFile)
		batch.Evidence = append(batch.Evidence, ev)
		batch.Associations = append(batch.Associations, edge(prNode.Ref(), codeFile.Ref(), ontology.AssocChangesFile, ev, record, observedAt, mapperVersion))
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
			edge(doc.Ref(), fragment.Ref(), ontology.AssocContains, containsEv, record, observedAt, mapperVersion),
			edge(domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:" + issueKey}, fragment.Ref(), ontology.AssocSupports, supportsEv, record, observedAt, mapperVersion),
			edge(fragment.Ref(), doc.Ref(), ontology.AssocDocuments, containsEv, record, observedAt, mapperVersion),
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
			batch.Associations = append(batch.Associations, edge(
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

func workstreamNode(record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
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

func ticketNode(issue jiraIssue, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
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

func actionNode(issueKey string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
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

func prNode(repo string, number int, title, sourceURL string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
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

func codeFileNode(repo, path string, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Object {
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

func edge(from, to domain.ObjectRef, predicate domain.AssociationType, ev domain.Evidence, record SnapshotRecord, observedAt time.Time, mapperVersion string) domain.Association {
	return domain.Association{
		From:            from,
		To:              to,
		AssociationType: predicate,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    ev.EvidenceKey,
			SourceURL:      record.SourceURL,
			SnapshotKey:    record.SnapshotKey,
			MapperVersion:  mapperVersion,
			Confidence:     ev.Confidence,
			ObservedAt:     observedAt,
			PropertiesJSON: `{"source_object_kind":"` + record.SourceObjectKind + `"}`,
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
		SourceObjectKind: record.SourceObjectKind,
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

type jiraSearchPage struct {
	Issues []jiraIssue `json:"issues"`
}

type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

type jiraRemoteLinks struct {
	IssueKey string `json:"issueKey"`
	Links    []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"links"`
}

type githubPR struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Files   []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
	} `json:"files"`
}

type ponymailSearch struct {
	Messages []struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	} `json:"messages"`
}
