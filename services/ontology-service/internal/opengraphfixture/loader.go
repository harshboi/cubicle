package opengraphfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/opengraphassociation"
	"cubicle/services/ontology-service/ent/opengraphobject"
	"cubicle/services/ontology-service/ent/sourceconnection"
	"cubicle/services/ontology-service/ent/sourcescope"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
)

// Fixture is a small connector-shaped open graph import contract.
type Fixture struct {
	SourceInstance string           `json:"sourceInstance"`
	SourceSystem   string           `json:"sourceSystem"`
	ConnectorKind  string           `json:"connectorKind"`
	ObservedAt     string           `json:"observedAt"`
	SourceScope    *SourceScopeRow  `json:"sourceScope"`
	Objects        []ObjectRow      `json:"objects"`
	Associations   []AssociationRow `json:"associations"`
}

// SourceScopeRow declares source coverage for this fixture when it was captured
// by a production-like connector. Omit it for generic hand-authored fixtures.
type SourceScopeRow struct {
	SourceSystem    string `json:"sourceSystem"`
	SourceInstance  string `json:"sourceInstance"`
	ScopeKind       string `json:"scopeKind"`
	ScopeKey        string `json:"scopeKey"`
	DisplayName     string `json:"displayName"`
	CrawlPolicy     string `json:"crawlPolicy"`
	RunKey          string `json:"runKey"`
	SyncMode        string `json:"syncMode"`
	Status          string `json:"status"`
	FreshnessState  string `json:"freshnessState"`
	CoverageMode    string `json:"coverageMode"`
	StartedAt       string `json:"startedAt"`
	CompletedAt     string `json:"completedAt"`
	CoverageStartAt string `json:"coverageStartAt"`
	CoverageEndAt   string `json:"coverageEndAt"`
	CheckpointToken string `json:"checkpointToken"`
	ErrorCode       string `json:"errorCode"`
	ErrorMessage    string `json:"errorMessage"`
}

// ObjectRow is one open graph object row.
type ObjectRow struct {
	ObjectType     string  `json:"objectType"`
	Key            string  `json:"key"`
	Title          string  `json:"title"`
	SourceSystem   string  `json:"sourceSystem"`
	SourceInstance string  `json:"sourceInstance"`
	ExternalKind   string  `json:"externalKind"`
	ExternalID     string  `json:"externalID"`
	SourceURL      string  `json:"sourceURL"`
	SourceVersion  string  `json:"sourceVersion"`
	Visibility     string  `json:"visibility"`
	ACLPolicyKey   string  `json:"aclPolicyKey"`
	VisibilityHash string  `json:"visibilityHash"`
	FreshnessState string  `json:"freshnessState"`
	PropertiesJSON string  `json:"propertiesJSON"`
	Confidence     float64 `json:"confidence"`
	RankScore      float64 `json:"rankScore"`
	ObservedAt     string  `json:"observedAt"`
}

// AssociationRow is one directed open graph relationship row.
type AssociationRow struct {
	From            Ref     `json:"from"`
	To              Ref     `json:"to"`
	AssociationType string  `json:"associationType"`
	SourceSystem    string  `json:"sourceSystem"`
	SourceInstance  string  `json:"sourceInstance"`
	ExternalKind    string  `json:"externalKind"`
	ExternalID      string  `json:"externalID"`
	SourceURL       string  `json:"sourceURL"`
	SourceVersion   string  `json:"sourceVersion"`
	LocatorKind     string  `json:"locatorKind"`
	Locator         string  `json:"locator"`
	EvidenceKey     string  `json:"evidenceKey"`
	Visibility      string  `json:"visibility"`
	ACLPolicyKey    string  `json:"aclPolicyKey"`
	VisibilityHash  string  `json:"visibilityHash"`
	FreshnessState  string  `json:"freshnessState"`
	PropertiesJSON  string  `json:"propertiesJSON"`
	Confidence      float64 `json:"confidence"`
	RankScore       float64 `json:"rankScore"`
	ObservedAt      string  `json:"observedAt"`
}

// Ref addresses one open graph object.
type Ref struct {
	ObjectType string `json:"objectType"`
	Key        string `json:"key"`
}

// Summary reports the number of persisted rows.
type Summary struct {
	ObjectCount      int `json:"objectCount"`
	AssociationCount int `json:"associationCount"`
	EvidenceCount    int `json:"evidenceCount"`
}

// LoadFile reads and persists an open graph fixture.
func LoadFile(ctx context.Context, client *genent.Client, path string) (Summary, error) {
	if client == nil {
		return Summary{}, fmt.Errorf("open graph fixture load: ent client is required")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, fmt.Errorf("read open graph fixture %s: %w", path, err)
	}
	var fixture Fixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		return Summary{}, fmt.Errorf("decode open graph fixture %s: %w", path, err)
	}
	return Load(ctx, client, fixture)
}

// Load persists all fixture rows into Ent open graph tables.
func Load(ctx context.Context, client *genent.Client, fixture Fixture) (Summary, error) {
	if client == nil {
		return Summary{}, fmt.Errorf("open graph fixture load: ent client is required")
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("open graph fixture load: start transaction: %w", err)
	}
	summary, err := load(ctx, tx.Client(), fixture)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return Summary{}, fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return Summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, fmt.Errorf("open graph fixture load: commit: %w", err)
	}
	return summary, nil
}

func load(ctx context.Context, client *genent.Client, fixture Fixture) (Summary, error) {
	defaultObservedAt, err := parseFixtureTime(fixture.ObservedAt, time.Time{})
	if err != nil {
		return Summary{}, err
	}
	rowsBefore, err := countOpenGraphRows(ctx, client)
	if err != nil {
		return Summary{}, err
	}
	provenance, err := prepareSourceProvenance(ctx, client, fixture, defaultObservedAt)
	if err != nil {
		return Summary{}, err
	}
	sourceScopeStateID := 0
	if provenance != nil {
		sourceScopeStateID = provenance.state.ID
	}
	objectsByRef := make(map[string]*genent.OpenGraphObject, len(fixture.Objects))
	for index, row := range fixture.Objects {
		object, err := upsertObject(ctx, client, fixture, row, defaultObservedAt, sourceScopeStateID)
		if err != nil {
			return Summary{}, fmt.Errorf("objects[%d]: %w", index, err)
		}
		objectsByRef[refKey(Ref{ObjectType: row.ObjectType, Key: row.Key})] = object
	}

	summary := Summary{ObjectCount: len(fixture.Objects)}
	for index, row := range fixture.Associations {
		if err := createAssociation(ctx, client, fixture, row, defaultObservedAt, objectsByRef, sourceScopeStateID); err != nil {
			return Summary{}, fmt.Errorf("associations[%d]: %w", index, err)
		}
		summary.AssociationCount++
		summary.EvidenceCount++
	}
	if provenance != nil {
		rowsAfter, err := countOpenGraphRows(ctx, client)
		if err != nil {
			return Summary{}, err
		}
		if err := completeSourceProvenance(ctx, client, provenance, summary, rowsAfter.subtract(rowsBefore)); err != nil {
			return Summary{}, err
		}
	}
	return summary, nil
}

func upsertObject(ctx context.Context, client *genent.Client, fixture Fixture, row ObjectRow, defaultObservedAt time.Time, sourceScopeStateID int) (*genent.OpenGraphObject, error) {
	objectType := strings.TrimSpace(row.ObjectType)
	key := strings.TrimSpace(row.Key)
	title := strings.TrimSpace(row.Title)
	if objectType == "" || key == "" || title == "" {
		return nil, fmt.Errorf("objectType, key, and title are required")
	}
	observedAt, err := parseFixtureTime(row.ObservedAt, defaultObservedAt)
	if err != nil {
		return nil, err
	}
	confidence := row.Confidence
	if confidence == 0 {
		confidence = 1
	}
	existing, err := client.OpenGraphObject.Query().
		Where(
			opengraphobject.ObjectTypeEQ(objectType),
			opengraphobject.KeyEQ(key),
		).
		Only(ctx)
	if err != nil && !genent.IsNotFound(err) {
		return nil, fmt.Errorf("lookup object %s/%s: %w", objectType, key, err)
	}
	propertiesJSON := strings.TrimSpace(row.PropertiesJSON)
	if existing == nil {
		create := client.OpenGraphObject.Create().
			SetObjectType(objectType).
			SetKey(key).
			SetTitle(title).
			SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
			SetSourceInstance(defaultString(row.SourceInstance, fixture.SourceInstance)).
			SetExternalKind(strings.TrimSpace(row.ExternalKind)).
			SetExternalID(strings.TrimSpace(row.ExternalID)).
			SetSourceURL(strings.TrimSpace(row.SourceURL)).
			SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
			SetFreshnessState(objectFreshnessState(row.FreshnessState)).
			SetVisibility(objectVisibility(row.Visibility)).
			SetConfidence(confidence).
			SetRankScore(row.RankScore)
		if !observedAt.IsZero() {
			create.SetSourceUpdatedAt(observedAt).
				SetFirstSeenAt(observedAt).
				SetLastConfirmedAt(observedAt).
				SetLastActivityAt(observedAt)
		}
		if propertiesJSON != "" {
			create.SetPropertiesJSON(propertiesJSON)
		}
		applyObjectSourceScopeCreate(create, sourceScopeStateID)
		applyObjectACLCreate(create, openGraphACLMetadata(row.Visibility, row.ACLPolicyKey, row.VisibilityHash, strings.TrimSpace(row.SourceSystem), defaultString(row.SourceInstance, fixture.SourceInstance), objectType, key, observedAt))
		return create.Save(ctx)
	}
	update := existing.Update().
		SetTitle(title).
		SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
		SetSourceInstance(defaultString(row.SourceInstance, fixture.SourceInstance)).
		SetExternalKind(strings.TrimSpace(row.ExternalKind)).
		SetExternalID(strings.TrimSpace(row.ExternalID)).
		SetSourceURL(strings.TrimSpace(row.SourceURL)).
		SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
		SetFreshnessState(objectFreshnessState(row.FreshnessState)).
		SetVisibility(objectVisibility(row.Visibility)).
		SetConfidence(confidence).
		SetRankScore(row.RankScore)
	if !observedAt.IsZero() {
		update.SetSourceUpdatedAt(observedAt).
			SetLastConfirmedAt(observedAt).
			SetLastActivityAt(observedAt)
	}
	if propertiesJSON != "" {
		update.SetPropertiesJSON(propertiesJSON)
	} else {
		update.ClearPropertiesJSON()
	}
	applyObjectSourceScopeUpdate(update, sourceScopeStateID)
	applyObjectACLUpdate(update, openGraphACLMetadata(row.Visibility, row.ACLPolicyKey, row.VisibilityHash, strings.TrimSpace(row.SourceSystem), defaultString(row.SourceInstance, fixture.SourceInstance), objectType, key, observedAt))
	return update.Save(ctx)
}

func createAssociation(ctx context.Context, client *genent.Client, fixture Fixture, row AssociationRow, defaultObservedAt time.Time, objectsByRef map[string]*genent.OpenGraphObject, sourceScopeStateID int) error {
	from := objectsByRef[refKey(row.From)]
	to := objectsByRef[refKey(row.To)]
	if from == nil || to == nil {
		return fmt.Errorf("from and to objects must exist in fixture")
	}
	if strings.TrimSpace(row.AssociationType) == "" || strings.TrimSpace(row.SourceSystem) == "" || strings.TrimSpace(row.LocatorKind) == "" {
		return fmt.Errorf("associationType, sourceSystem, and locatorKind are required")
	}
	observedAt, err := parseFixtureTime(row.ObservedAt, defaultObservedAt)
	if err != nil {
		return err
	}
	confidence := row.Confidence
	if confidence == 0 {
		confidence = 1
	}
	associationType := strings.TrimSpace(row.AssociationType)
	sourceInstance := defaultString(row.SourceInstance, fixture.SourceInstance)
	externalID := defaultString(row.ExternalID, from.Key+"->"+to.Key)
	externalKind := defaultString(row.ExternalKind, row.LocatorKind)
	evidenceKey := defaultString(row.EvidenceKey, "evidence:"+row.AssociationType+":"+from.Key+"->"+to.Key)
	locator := defaultString(row.Locator, from.Key+" -> "+to.Key)
	aclMetadata := openGraphACLMetadata(row.Visibility, row.ACLPolicyKey, row.VisibilityHash, strings.TrimSpace(row.SourceSystem), sourceInstance, associationType, externalID, observedAt)

	evidenceRow, err := upsertAssociationEvidence(ctx, client, row, evidenceKey, sourceInstance, externalKind, externalID, locator, observedAt, confidence, aclMetadata, sourceScopeStateID)
	if err != nil {
		return fmt.Errorf("upsert evidence %s: %w", evidenceKey, err)
	}

	existing, err := client.OpenGraphAssociation.Query().
		Where(
			opengraphassociation.FromObjectIDEQ(from.ID),
			opengraphassociation.ToObjectIDEQ(to.ID),
			opengraphassociation.AssociationTypeEQ(associationType),
		).
		Only(ctx)
	if err != nil && !genent.IsNotFound(err) {
		return fmt.Errorf("lookup association %s %s->%s: %w", row.AssociationType, from.Key, to.Key, err)
	}
	propertiesJSON := strings.TrimSpace(row.PropertiesJSON)
	if existing == nil {
		associationCreate := client.OpenGraphAssociation.Create().
			SetFromObject(from).
			SetToObject(to).
			SetAssociationType(associationType).
			SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
			SetSourceInstance(sourceInstance).
			SetExternalKind(externalKind).
			SetExternalID(externalID).
			SetSourceURL(strings.TrimSpace(row.SourceURL)).
			SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
			SetFreshnessState(associationFreshnessState(row.FreshnessState)).
			SetVisibility(associationVisibility(row.Visibility)).
			SetConfidence(confidence).
			SetRankScore(row.RankScore).
			SetEvidenceCount(1).
			SetLatestEvidence(evidenceRow)
		if !observedAt.IsZero() {
			associationCreate.SetSourceUpdatedAt(observedAt).
				SetFirstSeenAt(observedAt).
				SetLastConfirmedAt(observedAt).
				SetLastActivityAt(observedAt)
		}
		if propertiesJSON != "" {
			associationCreate.SetPropertiesJSON(propertiesJSON)
		}
		applyAssociationSourceScopeCreate(associationCreate, sourceScopeStateID)
		applyAssociationACLCreate(associationCreate, aclMetadata)
		if _, err := associationCreate.Save(ctx); err != nil {
			return fmt.Errorf("create association %s %s->%s: %w", row.AssociationType, from.Key, to.Key, err)
		}
		return nil
	}

	evidenceCount := existing.EvidenceCount
	if evidenceCount < 1 {
		evidenceCount = 1
	}
	if existing.LatestEvidenceID != 0 && existing.LatestEvidenceID != evidenceRow.ID {
		evidenceCount++
	}
	update := existing.Update().
		SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
		SetSourceInstance(sourceInstance).
		SetExternalKind(externalKind).
		SetExternalID(externalID).
		SetSourceURL(strings.TrimSpace(row.SourceURL)).
		SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
		SetFreshnessState(associationFreshnessState(row.FreshnessState)).
		SetVisibility(associationVisibility(row.Visibility)).
		SetConfidence(confidence).
		SetRankScore(row.RankScore).
		SetEvidenceCount(evidenceCount).
		SetLatestEvidence(evidenceRow)
	if !observedAt.IsZero() {
		update.SetSourceUpdatedAt(observedAt).
			SetLastConfirmedAt(observedAt).
			SetLastActivityAt(observedAt)
	}
	if propertiesJSON != "" {
		update.SetPropertiesJSON(propertiesJSON)
	} else {
		update.ClearPropertiesJSON()
	}
	applyAssociationSourceScopeUpdate(update, sourceScopeStateID)
	applyAssociationACLUpdate(update, aclMetadata)
	if _, err := update.Save(ctx); err != nil {
		return fmt.Errorf("update association %s %s->%s: %w", row.AssociationType, from.Key, to.Key, err)
	}
	return nil
}

func upsertAssociationEvidence(ctx context.Context, client *genent.Client, row AssociationRow, evidenceKey string, sourceInstance string, externalKind string, externalID string, locator string, observedAt time.Time, confidence float64, aclMetadata sourceACLMetadata, sourceScopeStateID int) (*genent.Evidence, error) {
	existing, err := client.Evidence.Query().
		Where(evidence.KeyEQ(evidenceKey)).
		Only(ctx)
	if err != nil && !genent.IsNotFound(err) {
		return nil, err
	}
	if existing == nil {
		evidenceCreate := client.Evidence.Create().
			SetKey(evidenceKey).
			SetClaimKind(evidence.ClaimKindRelationship).
			SetClaimTargetKind("open_graph_association").
			SetRelationshipKind(strings.TrimSpace(row.AssociationType)).
			SetLocatorKind(strings.TrimSpace(row.LocatorKind)).
			SetLocator(locator).
			SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
			SetSourceInstance(sourceInstance).
			SetExternalKind(externalKind).
			SetExternalID(externalID).
			SetSourceURL(strings.TrimSpace(row.SourceURL)).
			SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
			SetProofState(evidence.ProofStateCurrent).
			SetFreshnessState(evidenceFreshnessState(row.FreshnessState)).
			SetVisibility(evidenceVisibility(row.Visibility)).
			SetConfidence(confidence)
		if !observedAt.IsZero() {
			evidenceCreate.SetObservedAt(observedAt).SetSourceUpdatedAt(observedAt)
		}
		applyEvidenceSourceScopeCreate(evidenceCreate, sourceScopeStateID)
		applyEvidenceACLCreate(evidenceCreate, aclMetadata)
		return evidenceCreate.Save(ctx)
	}
	update := existing.Update().
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("open_graph_association").
		SetRelationshipKind(strings.TrimSpace(row.AssociationType)).
		SetLocatorKind(strings.TrimSpace(row.LocatorKind)).
		SetLocator(locator).
		SetSourceSystem(strings.TrimSpace(row.SourceSystem)).
		SetSourceInstance(sourceInstance).
		SetExternalKind(externalKind).
		SetExternalID(externalID).
		SetSourceURL(strings.TrimSpace(row.SourceURL)).
		SetSourceVersion(strings.TrimSpace(row.SourceVersion)).
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidenceFreshnessState(row.FreshnessState)).
		SetVisibility(evidenceVisibility(row.Visibility)).
		SetConfidence(confidence)
	if !observedAt.IsZero() {
		update.SetObservedAt(observedAt).SetSourceUpdatedAt(observedAt)
	}
	applyEvidenceSourceScopeUpdate(update, sourceScopeStateID)
	applyEvidenceACLUpdate(update, aclMetadata)
	return update.Save(ctx)
}

type openGraphRowCounts struct {
	Objects      int
	Associations int
	Evidence     int
}

func countOpenGraphRows(ctx context.Context, client *genent.Client) (openGraphRowCounts, error) {
	objects, err := client.OpenGraphObject.Query().Count(ctx)
	if err != nil {
		return openGraphRowCounts{}, fmt.Errorf("count open graph objects: %w", err)
	}
	associations, err := client.OpenGraphAssociation.Query().Count(ctx)
	if err != nil {
		return openGraphRowCounts{}, fmt.Errorf("count open graph associations: %w", err)
	}
	evidenceRows, err := client.Evidence.Query().Count(ctx)
	if err != nil {
		return openGraphRowCounts{}, fmt.Errorf("count evidence: %w", err)
	}
	return openGraphRowCounts{Objects: objects, Associations: associations, Evidence: evidenceRows}, nil
}

func (counts openGraphRowCounts) subtract(before openGraphRowCounts) openGraphRowCounts {
	return openGraphRowCounts{
		Objects:      nonNegativeDelta(counts.Objects, before.Objects),
		Associations: nonNegativeDelta(counts.Associations, before.Associations),
		Evidence:     nonNegativeDelta(counts.Evidence, before.Evidence),
	}
}

func nonNegativeDelta(after int, before int) int {
	if after <= before {
		return 0
	}
	return after - before
}

type sourceProvenance struct {
	connection      *genent.SourceConnection
	scope           *genent.SourceScope
	state           *genent.SourceScopeState
	runKey          string
	syncMode        sourcesyncrun.SyncMode
	status          sourcesyncrun.Status
	freshnessState  sourcescopestate.FreshnessState
	coverageMode    sourcesyncrun.CoverageMode
	startedAt       time.Time
	completedAt     time.Time
	coverageStartAt time.Time
	coverageEndAt   time.Time
	checkpointToken string
	errorCode       string
	errorMessage    string
}

func prepareSourceProvenance(ctx context.Context, client *genent.Client, fixture Fixture, defaultObservedAt time.Time) (*sourceProvenance, error) {
	if !sourceProvenanceRequested(fixture) {
		return nil, nil
	}
	if strings.TrimSpace(fixture.ConnectorKind) == "" || fixture.SourceScope == nil {
		return nil, fmt.Errorf("source provenance requires both connectorKind and sourceScope")
	}
	scopeRow := *fixture.SourceScope
	sourceSystem, err := fixtureSourceSystem(fixture, scopeRow)
	if err != nil {
		return nil, err
	}
	sourceInstance := defaultString(scopeRow.SourceInstance, fixture.SourceInstance)
	connectorKind := strings.TrimSpace(fixture.ConnectorKind)
	scopeKind := strings.TrimSpace(scopeRow.ScopeKind)
	scopeKey := strings.TrimSpace(scopeRow.ScopeKey)
	if sourceSystem == "" || sourceInstance == "" || scopeKind == "" || scopeKey == "" {
		return nil, fmt.Errorf("source provenance requires source system, source instance, scopeKind, and scopeKey")
	}
	syncMode, err := sourceSyncModeForFixture(scopeRow.SyncMode)
	if err != nil {
		return nil, err
	}
	status, err := sourceSyncStatusForFixture(scopeRow.Status)
	if err != nil {
		return nil, err
	}
	freshnessState, err := sourceFreshnessStateForFixture(scopeRow.FreshnessState)
	if err != nil {
		return nil, err
	}
	coverageMode, err := sourceCoverageModeForFixture(scopeRow.CoverageMode)
	if err != nil {
		return nil, err
	}
	startedAt, err := parseFixtureTime(scopeRow.StartedAt, defaultObservedAt)
	if err != nil {
		return nil, err
	}
	completedAt, err := parseFixtureTime(scopeRow.CompletedAt, defaultObservedAt)
	if err != nil {
		return nil, err
	}
	coverageStartAt, err := parseFixtureTime(scopeRow.CoverageStartAt, time.Time{})
	if err != nil {
		return nil, err
	}
	coverageEndAt, err := parseFixtureTime(scopeRow.CoverageEndAt, time.Time{})
	if err != nil {
		return nil, err
	}
	runKey := strings.TrimSpace(scopeRow.RunKey)
	if runKey == "" {
		runKey = "source-sync-run:" + sourceSystem + ":" + scopeKind + ":" + scopeKey + ":" + completedAt.Format("20060102T150405Z")
	}
	connection, err := ensureSourceConnection(ctx, client, sourceSystem, sourceInstance, connectorKind, defaultString(scopeRow.DisplayName, sourceSystem+" "+sourceInstance), completedAt)
	if err != nil {
		return nil, err
	}
	scope, err := ensureSourceScope(ctx, client, connection.ID, sourceSystem, sourceInstance, scopeKind, scopeKey, scopeRow.DisplayName, scopeRow.CrawlPolicy)
	if err != nil {
		return nil, err
	}
	state, err := ensureSourceScopeState(ctx, client, scope.ID, freshnessState, sourcescopestate.CoverageMode(coverageMode), completedAt, strings.TrimSpace(scopeRow.ErrorCode), strings.TrimSpace(scopeRow.ErrorMessage))
	if err != nil {
		return nil, err
	}
	return &sourceProvenance{
		connection:      connection,
		scope:           scope,
		state:           state,
		runKey:          runKey,
		syncMode:        syncMode,
		status:          status,
		freshnessState:  freshnessState,
		coverageMode:    coverageMode,
		startedAt:       startedAt,
		completedAt:     completedAt,
		coverageStartAt: coverageStartAt,
		coverageEndAt:   coverageEndAt,
		checkpointToken: strings.TrimSpace(scopeRow.CheckpointToken),
		errorCode:       strings.TrimSpace(scopeRow.ErrorCode),
		errorMessage:    strings.TrimSpace(scopeRow.ErrorMessage),
	}, nil
}

func sourceProvenanceRequested(fixture Fixture) bool {
	return strings.TrimSpace(fixture.ConnectorKind) != "" || fixture.SourceScope != nil
}

func fixtureSourceSystem(fixture Fixture, scopeRow SourceScopeRow) (string, error) {
	if sourceSystem := strings.TrimSpace(scopeRow.SourceSystem); sourceSystem != "" {
		return sourceSystem, nil
	}
	if sourceSystem := strings.TrimSpace(fixture.SourceSystem); sourceSystem != "" {
		return sourceSystem, nil
	}
	seen := make(map[string]struct{})
	for _, object := range fixture.Objects {
		if sourceSystem := strings.TrimSpace(object.SourceSystem); sourceSystem != "" {
			seen[sourceSystem] = struct{}{}
		}
	}
	for _, association := range fixture.Associations {
		if sourceSystem := strings.TrimSpace(association.SourceSystem); sourceSystem != "" {
			seen[sourceSystem] = struct{}{}
		}
	}
	if len(seen) == 1 {
		for sourceSystem := range seen {
			return sourceSystem, nil
		}
	}
	if len(seen) == 0 {
		return "", fmt.Errorf("source provenance requires a source system")
	}
	return "", fmt.Errorf("source provenance requires explicit sourceSystem when fixture contains multiple source systems")
}

func ensureSourceConnection(ctx context.Context, client *genent.Client, sourceSystem string, sourceInstance string, connectorKind string, displayName string, syncedAt time.Time) (*genent.SourceConnection, error) {
	connection, err := client.SourceConnection.Query().
		Where(sourceconnection.SourceSystemEQ(sourceSystem), sourceconnection.SourceInstanceEQ(sourceInstance)).
		Only(ctx)
	if err == nil {
		update := connection.Update().
			SetDisplayName(displayName).
			SetConnectorKind(connectorKind)
		if !syncedAt.IsZero() {
			update.SetLastSyncedAt(syncedAt)
		}
		updated, err := update.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update source connection %s/%s: %w", sourceSystem, sourceInstance, err)
		}
		return updated, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query source connection %s/%s: %w", sourceSystem, sourceInstance, err)
	}
	create := client.SourceConnection.Create().
		SetKey("source-connection:" + sourceSystem + ":" + sourceInstance).
		SetSourceSystem(sourceSystem).
		SetSourceInstance(sourceInstance).
		SetDisplayName(displayName).
		SetConnectorKind(connectorKind)
	if !syncedAt.IsZero() {
		create.SetLastSyncedAt(syncedAt)
	}
	return create.Save(ctx)
}

func ensureSourceScope(ctx context.Context, client *genent.Client, sourceConnectionID int, sourceSystem string, sourceInstance string, scopeKind string, scopeKey string, displayName string, crawlPolicy string) (*genent.SourceScope, error) {
	scope, err := client.SourceScope.Query().
		Where(
			sourcescope.SourceConnectionIDEQ(sourceConnectionID),
			sourcescope.ScopeKindEQ(scopeKind),
			sourcescope.ScopeKeyEQ(scopeKey),
		).
		Only(ctx)
	if err == nil {
		update := scope.Update()
		if strings.TrimSpace(displayName) != "" {
			update.SetDisplayName(strings.TrimSpace(displayName))
		}
		if strings.TrimSpace(crawlPolicy) != "" {
			update.SetCrawlPolicy(strings.TrimSpace(crawlPolicy))
		}
		updated, err := update.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update source scope %s/%s/%s: %w", sourceSystem, scopeKind, scopeKey, err)
		}
		return updated, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query source scope %s/%s/%s: %w", sourceSystem, scopeKind, scopeKey, err)
	}
	create := client.SourceScope.Create().
		SetKey("source-scope:" + sourceSystem + ":" + sourceInstance + ":" + scopeKind + ":" + scopeKey).
		SetSourceConnectionID(sourceConnectionID).
		SetScopeKind(scopeKind).
		SetScopeKey(scopeKey)
	if strings.TrimSpace(displayName) != "" {
		create.SetDisplayName(strings.TrimSpace(displayName))
	}
	if strings.TrimSpace(crawlPolicy) != "" {
		create.SetCrawlPolicy(strings.TrimSpace(crawlPolicy))
	}
	return create.Save(ctx)
}

func ensureSourceScopeState(ctx context.Context, client *genent.Client, sourceScopeID int, freshnessState sourcescopestate.FreshnessState, coverageMode sourcescopestate.CoverageMode, attemptedAt time.Time, errorCode string, errorMessage string) (*genent.SourceScopeState, error) {
	state, err := client.SourceScopeState.Query().Where(sourcescopestate.SourceScopeIDEQ(sourceScopeID)).Only(ctx)
	if err == nil {
		update := state.Update().
			SetFreshnessState(freshnessState).
			SetCoverageMode(coverageMode)
		if !attemptedAt.IsZero() {
			update.SetLastAttemptedAt(attemptedAt)
		}
		if errorCode != "" {
			update.SetErrorCode(errorCode)
		} else {
			update.ClearErrorCode()
		}
		if errorMessage != "" {
			update.SetErrorMessage(errorMessage)
		} else {
			update.ClearErrorMessage()
		}
		updated, err := update.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update source scope state %d: %w", sourceScopeID, err)
		}
		return updated, nil
	}
	if !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query source scope state %d: %w", sourceScopeID, err)
	}
	create := client.SourceScopeState.Create().
		SetSourceScopeID(sourceScopeID).
		SetFreshnessState(freshnessState).
		SetCoverageMode(coverageMode)
	if !attemptedAt.IsZero() {
		create.SetLastAttemptedAt(attemptedAt)
	}
	if errorCode != "" {
		create.SetErrorCode(errorCode)
	}
	if errorMessage != "" {
		create.SetErrorMessage(errorMessage)
	}
	return create.Save(ctx)
}

func completeSourceProvenance(ctx context.Context, client *genent.Client, provenance *sourceProvenance, summary Summary, rowDelta openGraphRowCounts) error {
	run, err := upsertSourceSyncRun(ctx, client, provenance, summary, rowDelta)
	if err != nil {
		return err
	}
	update := provenance.state.Update().
		SetFreshnessState(provenance.freshnessState).
		SetCoverageMode(sourcescopestate.CoverageMode(provenance.coverageMode))
	if !provenance.completedAt.IsZero() {
		update.SetLastAttemptedAt(provenance.completedAt)
	}
	if sourceSyncStatusSucceeded(provenance.status) {
		update.SetLastSuccessfulSyncRun(run)
		if !provenance.completedAt.IsZero() {
			update.SetLastSuccessfulAt(provenance.completedAt)
		}
		update.ClearErrorCode().ClearErrorMessage()
	} else {
		update.ClearLastSuccessfulSyncRun().ClearLastSuccessfulAt()
		if provenance.errorCode != "" {
			update.SetErrorCode(provenance.errorCode)
		}
		if provenance.errorMessage != "" {
			update.SetErrorMessage(provenance.errorMessage)
		}
	}
	if _, err := update.Save(ctx); err != nil {
		return fmt.Errorf("update source scope state after run: %w", err)
	}
	if !provenance.completedAt.IsZero() {
		if err := provenance.connection.Update().SetLastSyncedAt(provenance.completedAt).Exec(ctx); err != nil {
			return fmt.Errorf("update source connection last synced: %w", err)
		}
	}
	return nil
}

func upsertSourceSyncRun(ctx context.Context, client *genent.Client, provenance *sourceProvenance, summary Summary, rowDelta openGraphRowCounts) (*genent.SourceSyncRun, error) {
	existing, err := client.SourceSyncRun.Query().
		Where(sourcesyncrun.SourceScopeIDEQ(provenance.scope.ID), sourcesyncrun.RunKeyEQ(provenance.runKey)).
		Only(ctx)
	if err != nil && !genent.IsNotFound(err) {
		return nil, fmt.Errorf("query source sync run %s: %w", provenance.runKey, err)
	}
	objectsUpdated := summary.ObjectCount - rowDelta.Objects
	if objectsUpdated < 0 {
		objectsUpdated = 0
	}
	relationshipsUpdated := summary.AssociationCount - rowDelta.Associations
	if relationshipsUpdated < 0 {
		relationshipsUpdated = 0
	}
	if existing == nil {
		create := client.SourceSyncRun.Create().
			SetSourceScopeID(provenance.scope.ID).
			SetRunKey(provenance.runKey).
			SetSyncMode(provenance.syncMode).
			SetCoverageMode(provenance.coverageMode).
			SetStatus(provenance.status).
			SetObjectsSeenCount(summary.ObjectCount).
			SetObjectsCreatedCount(rowDelta.Objects).
			SetObjectsUpdatedCount(objectsUpdated).
			SetRelationshipsCreatedCount(rowDelta.Associations).
			SetRelationshipsUpdatedCount(relationshipsUpdated).
			SetEvidenceCreatedCount(rowDelta.Evidence)
		applySourceSyncRunOptionalFields(create, provenance)
		return create.Save(ctx)
	}
	update := existing.Update().
		SetSyncMode(provenance.syncMode).
		SetCoverageMode(provenance.coverageMode).
		SetStatus(provenance.status).
		SetObjectsSeenCount(summary.ObjectCount).
		SetObjectsCreatedCount(rowDelta.Objects).
		SetObjectsUpdatedCount(objectsUpdated).
		SetRelationshipsCreatedCount(rowDelta.Associations).
		SetRelationshipsUpdatedCount(relationshipsUpdated).
		SetEvidenceCreatedCount(rowDelta.Evidence)
	applySourceSyncRunUpdateOptionalFields(update, provenance)
	updated, err := update.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update source sync run %s: %w", provenance.runKey, err)
	}
	return updated, nil
}

func applySourceSyncRunOptionalFields(create *genent.SourceSyncRunCreate, provenance *sourceProvenance) {
	if !provenance.startedAt.IsZero() {
		create.SetStartedAt(provenance.startedAt)
	}
	if !provenance.completedAt.IsZero() {
		create.SetCompletedAt(provenance.completedAt)
	}
	if !provenance.coverageStartAt.IsZero() {
		create.SetCoverageStartAt(provenance.coverageStartAt)
	}
	if !provenance.coverageEndAt.IsZero() {
		create.SetCoverageEndAt(provenance.coverageEndAt)
	}
	if provenance.checkpointToken != "" {
		create.SetCheckpointToken(provenance.checkpointToken)
	}
	if provenance.errorCode != "" {
		create.SetErrorCode(provenance.errorCode)
	}
	if provenance.errorMessage != "" {
		create.SetErrorMessage(provenance.errorMessage)
	}
}

func applySourceSyncRunUpdateOptionalFields(update *genent.SourceSyncRunUpdateOne, provenance *sourceProvenance) {
	if !provenance.startedAt.IsZero() {
		update.SetStartedAt(provenance.startedAt)
	} else {
		update.ClearStartedAt()
	}
	if !provenance.completedAt.IsZero() {
		update.SetCompletedAt(provenance.completedAt)
	} else {
		update.ClearCompletedAt()
	}
	if !provenance.coverageStartAt.IsZero() {
		update.SetCoverageStartAt(provenance.coverageStartAt)
	} else {
		update.ClearCoverageStartAt()
	}
	if !provenance.coverageEndAt.IsZero() {
		update.SetCoverageEndAt(provenance.coverageEndAt)
	} else {
		update.ClearCoverageEndAt()
	}
	if provenance.checkpointToken != "" {
		update.SetCheckpointToken(provenance.checkpointToken)
	} else {
		update.ClearCheckpointToken()
	}
	if provenance.errorCode != "" {
		update.SetErrorCode(provenance.errorCode)
	} else {
		update.ClearErrorCode()
	}
	if provenance.errorMessage != "" {
		update.SetErrorMessage(provenance.errorMessage)
	} else {
		update.ClearErrorMessage()
	}
}

func sourceSyncStatusSucceeded(status sourcesyncrun.Status) bool {
	return status == sourcesyncrun.StatusComplete || status == sourcesyncrun.StatusPartial
}

func sourceSyncModeForFixture(value string) (sourcesyncrun.SyncMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(sourcesyncrun.SyncModeSnapshot):
		return sourcesyncrun.SyncModeSnapshot, nil
	case string(sourcesyncrun.SyncModeIncremental):
		return sourcesyncrun.SyncModeIncremental, nil
	case string(sourcesyncrun.SyncModeFederatedLive):
		return sourcesyncrun.SyncModeFederatedLive, nil
	default:
		return "", fmt.Errorf("invalid sourceScope.syncMode %q", value)
	}
}

func sourceSyncStatusForFixture(value string) (sourcesyncrun.Status, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(sourcesyncrun.StatusComplete):
		return sourcesyncrun.StatusComplete, nil
	case string(sourcesyncrun.StatusRunning):
		return sourcesyncrun.StatusRunning, nil
	case string(sourcesyncrun.StatusPartial):
		return sourcesyncrun.StatusPartial, nil
	case string(sourcesyncrun.StatusFailed):
		return sourcesyncrun.StatusFailed, nil
	case string(sourcesyncrun.StatusRateLimited):
		return sourcesyncrun.StatusRateLimited, nil
	default:
		return "", fmt.Errorf("invalid sourceScope.status %q", value)
	}
}

func sourceFreshnessStateForFixture(value string) (sourcescopestate.FreshnessState, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(sourcescopestate.FreshnessStateFresh):
		return sourcescopestate.FreshnessStateFresh, nil
	case string(sourcescopestate.FreshnessStatePartial):
		return sourcescopestate.FreshnessStatePartial, nil
	case string(sourcescopestate.FreshnessStateStale):
		return sourcescopestate.FreshnessStateStale, nil
	case string(sourcescopestate.FreshnessStateUnknown):
		return sourcescopestate.FreshnessStateUnknown, nil
	default:
		return "", fmt.Errorf("invalid sourceScope.freshnessState %q", value)
	}
}

func sourceCoverageModeForFixture(value string) (sourcesyncrun.CoverageMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(sourcesyncrun.CoverageModePartialScope):
		return sourcesyncrun.CoverageModePartialScope, nil
	case string(sourcesyncrun.CoverageModeUnknown):
		return sourcesyncrun.CoverageModeUnknown, nil
	case string(sourcesyncrun.CoverageModeExactScope):
		return sourcesyncrun.CoverageModeExactScope, nil
	case string(sourcesyncrun.CoverageModeMetadataOnly):
		return sourcesyncrun.CoverageModeMetadataOnly, nil
	case string(sourcesyncrun.CoverageModeIdentityOnly):
		return sourcesyncrun.CoverageModeIdentityOnly, nil
	case string(sourcesyncrun.CoverageModeLiveOnly):
		return sourcesyncrun.CoverageModeLiveOnly, nil
	default:
		return "", fmt.Errorf("invalid sourceScope.coverageMode %q", value)
	}
}

func parseFixtureTime(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp %q", value)
}

func refKey(ref Ref) string {
	return strings.TrimSpace(ref.ObjectType) + "\x00" + strings.TrimSpace(ref.Key)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

type sourceACLMetadata struct {
	Current        bool
	ACLPolicyKey   string
	VisibilityHash string
	CheckedAt      time.Time
}

func openGraphACLMetadata(visibility string, aclPolicyKey string, visibilityHash string, sourceSystem string, sourceInstance string, objectType string, key string, checkedAt time.Time) sourceACLMetadata {
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	switch visibility {
	case "public", "private", "team", "restricted":
	default:
		return sourceACLMetadata{}
	}
	policyKey := strings.TrimSpace(aclPolicyKey)
	if policyKey == "" {
		policyKey = "open_graph_fixture:" + strings.TrimSpace(sourceSystem) + ":" + strings.TrimSpace(sourceInstance)
	}
	hash := strings.TrimSpace(visibilityHash)
	if hash == "" {
		hash = "visibility:" + visibility + ":" + strings.TrimSpace(sourceSystem) + ":" + strings.TrimSpace(sourceInstance) + ":" + strings.TrimSpace(objectType) + ":" + strings.TrimSpace(key)
	}
	return sourceACLMetadata{
		Current:        true,
		ACLPolicyKey:   policyKey,
		VisibilityHash: hash,
		CheckedAt:      checkedAt,
	}
}

func applyObjectACLCreate(create *genent.OpenGraphObjectCreate, metadata sourceACLMetadata) {
	if !metadata.Current {
		return
	}
	create.SetACLState(opengraphobject.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		create.SetACLCheckedAt(metadata.CheckedAt)
	}
}

func applyObjectACLUpdate(update *genent.OpenGraphObjectUpdateOne, metadata sourceACLMetadata) {
	if !metadata.Current {
		update.SetACLState(opengraphobject.ACLStateUnknown).
			ClearACLPolicyKey().
			ClearVisibilityHash().
			ClearACLCheckedAt()
		return
	}
	update.SetACLState(opengraphobject.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		update.SetACLCheckedAt(metadata.CheckedAt)
	} else {
		update.ClearACLCheckedAt()
	}
}

func applyObjectSourceScopeCreate(create *genent.OpenGraphObjectCreate, sourceScopeStateID int) {
	if sourceScopeStateID != 0 {
		create.SetSourceScopeStateID(sourceScopeStateID)
	}
}

func applyObjectSourceScopeUpdate(update *genent.OpenGraphObjectUpdateOne, sourceScopeStateID int) {
	if sourceScopeStateID == 0 {
		update.ClearSourceScopeStateID()
		return
	}
	update.SetSourceScopeStateID(sourceScopeStateID)
}

func applyAssociationACLCreate(create *genent.OpenGraphAssociationCreate, metadata sourceACLMetadata) {
	if !metadata.Current {
		return
	}
	create.SetACLState(opengraphassociation.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		create.SetACLCheckedAt(metadata.CheckedAt)
	}
}

func applyAssociationACLUpdate(update *genent.OpenGraphAssociationUpdateOne, metadata sourceACLMetadata) {
	if !metadata.Current {
		update.SetACLState(opengraphassociation.ACLStateUnknown).
			ClearACLPolicyKey().
			ClearVisibilityHash().
			ClearACLCheckedAt()
		return
	}
	update.SetACLState(opengraphassociation.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		update.SetACLCheckedAt(metadata.CheckedAt)
	} else {
		update.ClearACLCheckedAt()
	}
}

func applyAssociationSourceScopeCreate(create *genent.OpenGraphAssociationCreate, sourceScopeStateID int) {
	if sourceScopeStateID != 0 {
		create.SetSourceScopeStateID(sourceScopeStateID)
	}
}

func applyAssociationSourceScopeUpdate(update *genent.OpenGraphAssociationUpdateOne, sourceScopeStateID int) {
	if sourceScopeStateID == 0 {
		update.ClearSourceScopeStateID()
		return
	}
	update.SetSourceScopeStateID(sourceScopeStateID)
}

func applyEvidenceACLCreate(create *genent.EvidenceCreate, metadata sourceACLMetadata) {
	if !metadata.Current {
		return
	}
	create.SetACLState(evidence.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash).
		SetACLPolicyKeySnapshot(metadata.ACLPolicyKey).
		SetVisibilityHashSnapshot(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		create.SetACLCheckedAt(metadata.CheckedAt)
	}
}

func applyEvidenceACLUpdate(update *genent.EvidenceUpdateOne, metadata sourceACLMetadata) {
	if !metadata.Current {
		update.SetACLState(evidence.ACLStateUnknown).
			ClearACLPolicyKey().
			ClearVisibilityHash().
			ClearACLPolicyKeySnapshot().
			ClearVisibilityHashSnapshot().
			ClearACLCheckedAt()
		return
	}
	update.SetACLState(evidence.ACLStateCurrent).
		SetACLPolicyKey(metadata.ACLPolicyKey).
		SetVisibilityHash(metadata.VisibilityHash).
		SetACLPolicyKeySnapshot(metadata.ACLPolicyKey).
		SetVisibilityHashSnapshot(metadata.VisibilityHash)
	if !metadata.CheckedAt.IsZero() {
		update.SetACLCheckedAt(metadata.CheckedAt)
	} else {
		update.ClearACLCheckedAt()
	}
}

func applyEvidenceSourceScopeCreate(create *genent.EvidenceCreate, sourceScopeStateID int) {
	if sourceScopeStateID != 0 {
		create.SetSourceScopeStateID(sourceScopeStateID)
	}
}

func applyEvidenceSourceScopeUpdate(update *genent.EvidenceUpdateOne, sourceScopeStateID int) {
	if sourceScopeStateID == 0 {
		update.ClearSourceScopeStateID()
		return
	}
	update.SetSourceScopeStateID(sourceScopeStateID)
}

func objectFreshnessState(value string) opengraphobject.FreshnessState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fresh":
		return opengraphobject.FreshnessStateFresh
	case "partial":
		return opengraphobject.FreshnessStatePartial
	case "stale":
		return opengraphobject.FreshnessStateStale
	case "unknown":
		return opengraphobject.FreshnessStateUnknown
	default:
		return opengraphobject.FreshnessStateUnknown
	}
}

func associationFreshnessState(value string) opengraphassociation.FreshnessState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fresh":
		return opengraphassociation.FreshnessStateFresh
	case "partial":
		return opengraphassociation.FreshnessStatePartial
	case "stale":
		return opengraphassociation.FreshnessStateStale
	case "unknown":
		return opengraphassociation.FreshnessStateUnknown
	default:
		return opengraphassociation.FreshnessStateUnknown
	}
}

func evidenceFreshnessState(value string) evidence.FreshnessState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fresh":
		return evidence.FreshnessStateFresh
	case "partial":
		return evidence.FreshnessStatePartial
	case "stale":
		return evidence.FreshnessStateStale
	case "unknown":
		return evidence.FreshnessStateUnknown
	default:
		return evidence.FreshnessStateUnknown
	}
}

func objectVisibility(value string) opengraphobject.Visibility {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public":
		return opengraphobject.VisibilityPublic
	case "private":
		return opengraphobject.VisibilityPrivate
	case "team":
		return opengraphobject.VisibilityTeam
	case "restricted":
		return opengraphobject.VisibilityRestricted
	case "unknown":
		return opengraphobject.VisibilityUnknown
	default:
		return opengraphobject.VisibilityUnknown
	}
}

func associationVisibility(value string) opengraphassociation.Visibility {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public":
		return opengraphassociation.VisibilityPublic
	case "private":
		return opengraphassociation.VisibilityPrivate
	case "team":
		return opengraphassociation.VisibilityTeam
	case "restricted":
		return opengraphassociation.VisibilityRestricted
	case "unknown":
		return opengraphassociation.VisibilityUnknown
	default:
		return opengraphassociation.VisibilityUnknown
	}
}

func evidenceVisibility(value string) evidence.Visibility {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public":
		return evidence.VisibilityPublic
	case "private":
		return evidence.VisibilityPrivate
	case "team":
		return evidence.VisibilityTeam
	case "restricted":
		return evidence.VisibilityRestricted
	case "unknown":
		return evidence.VisibilityUnknown
	default:
		return evidence.VisibilityUnknown
	}
}
