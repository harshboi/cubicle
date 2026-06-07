package graphstore

import (
	"context"
	"fmt"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/ingestrun"
	entobject "cubicle/services/ontology-service/ent/object"
	"cubicle/services/ontology-service/ent/sourcecheckpoint"
	"cubicle/services/ontology-service/ent/sourceevent"
	"cubicle/services/ontology-service/ent/sourcesnapshot"
	"cubicle/services/ontology-service/internal/domain"
)

func (s *EntStore) BeginIngestRun(ctx context.Context, start domain.IngestRunStart) (domain.IngestRun, error) {
	if err := start.Validate(); err != nil {
		return domain.IngestRun{}, fmt.Errorf("%w: %v", ErrInvalidIngest, err)
	}
	if start.StartedAt.IsZero() {
		start.StartedAt = time.Now().UTC()
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domain.IngestRun{}, err
	}
	committed := false
	defer rollbackIfUncommitted(tx, &committed)

	existing, err := tx.IngestRun.Query().
		Where(ingestrun.RunKeyEQ(start.RunKey)).
		Only(ctx)
	if err == nil {
		if existing.Source != start.Source || existing.SourceInstance != start.SourceInstance || existing.Slice != start.Slice {
			return domain.IngestRun{}, fmt.Errorf("%w: run_key %s already belongs to another source", ErrIngestConflict, start.RunKey)
		}
		if err := tx.Commit(); err != nil {
			return domain.IngestRun{}, err
		}
		committed = true
		return ingestRunToDomain(existing), nil
	}
	if !ent.IsNotFound(err) {
		return domain.IngestRun{}, err
	}

	run, err := tx.IngestRun.Create().
		SetRunKey(start.RunKey).
		SetSource(start.Source).
		SetSourceInstance(start.SourceInstance).
		SetSlice(start.Slice).
		SetMapperVersion(start.MapperVersion).
		SetStatus(string(domain.IngestRunOpen)).
		SetStartedAt(start.StartedAt).
		Save(ctx)
	if err != nil {
		return domain.IngestRun{}, err
	}
	if err := upsertSourceCheckpoint(ctx, tx.Client(), sourceCheckpointUpdate{
		Source:              start.Source,
		SourceInstance:      start.SourceInstance,
		Slice:               start.Slice,
		Status:              string(domain.SourceStatusRunning),
		LastAttemptedRunKey: start.RunKey,
		UpdatedAt:           start.StartedAt,
	}); err != nil {
		return domain.IngestRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.IngestRun{}, err
	}
	committed = true
	return ingestRunToDomain(run), nil
}

func (s *EntStore) WriteSnapshot(ctx context.Context, write domain.SourceSnapshotWrite) (domain.SourceSnapshot, error) {
	if err := write.Validate(); err != nil {
		return domain.SourceSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidIngest, err)
	}
	now := time.Now().UTC()
	if write.FetchedAt.IsZero() {
		write.FetchedAt = now
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domain.SourceSnapshot{}, err
	}
	committed := false
	defer rollbackIfUncommitted(tx, &committed)

	run, err := openRun(ctx, tx.Client(), write.RunKey)
	if err != nil {
		return domain.SourceSnapshot{}, err
	}
	if err := ensureSameSource(run, write.Source, write.SourceInstance); err != nil {
		return domain.SourceSnapshot{}, err
	}

	existing, err := tx.SourceSnapshot.Query().
		Where(sourcesnapshot.SnapshotKeyEQ(write.SnapshotKey)).
		Only(ctx)
	if err == nil {
		if existing.Source != write.Source || existing.SourceInstance != write.SourceInstance {
			return domain.SourceSnapshot{}, fmt.Errorf("%w: snapshot_key %s belongs to another source", ErrIngestConflict, write.SnapshotKey)
		}
		if existing.BodySha256 != write.BodySHA256 {
			return domain.SourceSnapshot{}, fmt.Errorf("%w: snapshot_key %s changed body hash", ErrIngestConflict, write.SnapshotKey)
		}
		if err := tx.Commit(); err != nil {
			return domain.SourceSnapshot{}, err
		}
		committed = true
		return sourceSnapshotToDomain(existing), nil
	}
	if !ent.IsNotFound(err) {
		return domain.SourceSnapshot{}, err
	}

	snapshot, err := tx.SourceSnapshot.Create().
		SetSnapshotKey(write.SnapshotKey).
		SetRunKey(write.RunKey).
		SetSource(write.Source).
		SetSourceInstance(write.SourceInstance).
		SetSourceObjectType(write.SourceObjectType).
		SetSourceObjectID(write.SourceObjectID).
		SetBodySha256(write.BodySHA256).
		SetBodyRef(write.BodyRef).
		SetSourceURL(write.SourceURL).
		SetFetchedAt(write.FetchedAt).
		SetHeadersJSON(write.HeadersJSON).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		return domain.SourceSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SourceSnapshot{}, err
	}
	committed = true
	return sourceSnapshotToDomain(snapshot), nil
}

func (s *EntStore) WriteMappedBatch(ctx context.Context, batch domain.IngestBatch) (domain.IngestBatchResult, error) {
	batch = batch.WithDefaults()
	if err := batch.Validate(); err != nil {
		return domain.IngestBatchResult{}, fmt.Errorf("%w: %v", ErrInvalidIngest, err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domain.IngestBatchResult{}, err
	}
	committed := false
	defer rollbackIfUncommitted(tx, &committed)
	client := tx.Client()

	run, err := openRun(ctx, client, batch.RunKey)
	if err != nil {
		return domain.IngestBatchResult{}, err
	}
	if err := ensureSameSource(run, batch.Source, batch.SourceInstance); err != nil {
		return domain.IngestBatchResult{}, err
	}
	if err := ensureSnapshotsExist(ctx, client, snapshotKeys(batch), batch.Source, batch.SourceInstance); err != nil {
		return domain.IngestBatchResult{}, err
	}
	if err := ensureEvidenceReferences(ctx, client, batch); err != nil {
		return domain.IngestBatchResult{}, err
	}
	if err := ensureAssociationEndpoints(ctx, client, batch); err != nil {
		return domain.IngestBatchResult{}, err
	}

	for _, event := range batch.Events {
		if err := upsertSourceEvent(ctx, client, batch, event); err != nil {
			return domain.IngestBatchResult{}, err
		}
	}
	for _, evidence := range batch.Evidence {
		if err := upsertEvidence(ctx, client, batch, evidence); err != nil {
			return domain.IngestBatchResult{}, err
		}
	}
	for _, object := range batch.Objects {
		if err := upsertObject(ctx, client, object); err != nil {
			return domain.IngestBatchResult{}, err
		}
	}
	for _, association := range batch.Associations {
		if err := upsertAssociation(ctx, client, association); err != nil {
			return domain.IngestBatchResult{}, err
		}
	}
	checkpointUpdated := false
	if batch.Checkpoint != nil {
		if err := upsertBatchCheckpoint(ctx, client, batch); err != nil {
			return domain.IngestBatchResult{}, err
		}
		checkpointUpdated = true
	}

	if err := tx.Commit(); err != nil {
		return domain.IngestBatchResult{}, err
	}
	committed = true
	return domain.IngestBatchResult{
		RunKey:               batch.RunKey,
		ObjectsUpserted:      len(batch.Objects),
		AssociationsUpserted: len(batch.Associations),
		EvidenceUpserted:     len(batch.Evidence),
		EventsUpserted:       len(batch.Events),
		CheckpointUpdated:    checkpointUpdated,
	}, nil
}

func (s *EntStore) CompleteIngestRun(ctx context.Context, complete domain.IngestRunComplete) (domain.IngestRun, error) {
	if err := complete.Validate(); err != nil {
		return domain.IngestRun{}, fmt.Errorf("%w: %v", ErrInvalidIngest, err)
	}
	if complete.CompletedAt.IsZero() {
		complete.CompletedAt = time.Now().UTC()
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return domain.IngestRun{}, err
	}
	committed := false
	defer rollbackIfUncommitted(tx, &committed)
	client := tx.Client()

	run, err := client.IngestRun.Query().
		Where(ingestrun.RunKeyEQ(complete.RunKey)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return domain.IngestRun{}, fmt.Errorf("%w: %s", ErrInvalidIngest, complete.RunKey)
	}
	if err != nil {
		return domain.IngestRun{}, err
	}
	if run.Status != string(domain.IngestRunOpen) {
		if err := tx.Commit(); err != nil {
			return domain.IngestRun{}, err
		}
		committed = true
		return ingestRunToDomain(run), nil
	}

	updated, err := run.Update().
		SetStatus(string(complete.Status)).
		SetCompletedAt(complete.CompletedAt).
		SetErrorCode(string(complete.ErrorCode)).
		SetErrorMessage(complete.ErrorMessage).
		Save(ctx)
	if err != nil {
		return domain.IngestRun{}, err
	}

	checkpoint := sourceCheckpointUpdate{
		Source:              run.Source,
		SourceInstance:      run.SourceInstance,
		Slice:               run.Slice,
		Status:              string(domain.SourceStatusHealthy),
		LastAttemptedRunKey: run.RunKey,
		UpdatedAt:           complete.CompletedAt,
	}
	if complete.Status == domain.IngestRunCompleted {
		checkpoint.LastSuccessfulRunKey = run.RunKey
	}
	if complete.Status == domain.IngestRunFailed {
		checkpoint.Status = string(domain.SourceStatusFailed)
		checkpoint.LastErrorKey = string(complete.ErrorCode)
	}
	if err := upsertSourceCheckpoint(ctx, client, checkpoint); err != nil {
		return domain.IngestRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.IngestRun{}, err
	}
	committed = true
	return ingestRunToDomain(updated), nil
}

func (s *EntStore) GetIngestRun(ctx context.Context, runKey string) (domain.IngestRun, error) {
	run, err := s.client.IngestRun.Query().
		Where(ingestrun.RunKeyEQ(runKey)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return domain.IngestRun{}, fmt.Errorf("%w: %s", ErrInvalidIngest, runKey)
	}
	if err != nil {
		return domain.IngestRun{}, err
	}
	return ingestRunToDomain(run), nil
}

func (s *EntStore) ListSourceStatus(ctx context.Context) ([]domain.SourceStatus, error) {
	checkpoints, err := s.client.SourceCheckpoint.Query().
		Order(sourcecheckpoint.BySource(), sourcecheckpoint.BySourceInstance(), sourcecheckpoint.BySlice()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]domain.SourceStatus, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		counts, err := s.countObjectsByType(ctx, checkpoint.Source, checkpoint.SourceInstance)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, domain.SourceStatus{
			Source:               checkpoint.Source,
			SourceInstance:       checkpoint.SourceInstance,
			Slice:                checkpoint.Slice,
			Status:               domain.SourceStatusValue(checkpoint.Status),
			LastSuccessfulRunKey: checkpoint.LastSuccessfulRunKey,
			LastAttemptedRunKey:  checkpoint.LastAttemptedRunKey,
			LastErrorKey:         checkpoint.LastErrorKey,
			NextAllowedAt:        checkpoint.NextAllowedAt,
			CountsByObjectType:   counts,
		})
	}
	return statuses, nil
}

func rollbackIfUncommitted(tx *ent.Tx, committed *bool) {
	if !*committed {
		_ = tx.Rollback()
	}
}

func openRun(ctx context.Context, client *ent.Client, runKey string) (*ent.IngestRun, error) {
	run, err := client.IngestRun.Query().
		Where(ingestrun.RunKeyEQ(runKey)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrRunNotOpen, runKey)
	}
	if err != nil {
		return nil, err
	}
	if run.Status != string(domain.IngestRunOpen) {
		return nil, fmt.Errorf("%w: %s is %s", ErrRunNotOpen, runKey, run.Status)
	}
	return run, nil
}

func ensureSameSource(run *ent.IngestRun, source, sourceInstance string) error {
	if run.Source != source || run.SourceInstance != sourceInstance {
		return fmt.Errorf("%w: run %s belongs to %s/%s", ErrIngestConflict, run.RunKey, run.Source, run.SourceInstance)
	}
	return nil
}

func ensureSnapshotsExist(ctx context.Context, client *ent.Client, keys []string, source, sourceInstance string) error {
	if len(keys) == 0 {
		return fmt.Errorf("%w: batch must reference at least one snapshot", ErrSnapshotNotFound)
	}
	for _, key := range keys {
		snapshot, err := client.SourceSnapshot.Query().Where(sourcesnapshot.SnapshotKeyEQ(key)).Only(ctx)
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: %s", ErrSnapshotNotFound, key)
		} else if err != nil {
			return err
		}
		if snapshot.Source != source || snapshot.SourceInstance != sourceInstance {
			return fmt.Errorf("%w: snapshot_key %s belongs to another source", ErrIngestConflict, key)
		}
	}
	return nil
}

func snapshotKeys(batch domain.IngestBatch) []string {
	seen := make(map[string]bool)
	for _, key := range batch.SnapshotKeys {
		if key != "" {
			seen[key] = true
		}
	}
	for _, object := range batch.Objects {
		if object.SnapshotKey != "" {
			seen[object.SnapshotKey] = true
		}
	}
	for _, association := range batch.Associations {
		if association.Metadata.SnapshotKey != "" {
			seen[association.Metadata.SnapshotKey] = true
		}
	}
	for _, evidence := range batch.Evidence {
		if evidence.SnapshotKey != "" {
			seen[evidence.SnapshotKey] = true
		}
	}
	for _, event := range batch.Events {
		if event.SnapshotKey != "" {
			seen[event.SnapshotKey] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	return keys
}

func ensureEvidenceReferences(ctx context.Context, client *ent.Client, batch domain.IngestBatch) error {
	batchEvidence := make(map[string]bool)
	for _, evidence := range batch.Evidence {
		batchEvidence[evidence.EvidenceKey] = true
	}
	for _, association := range batch.Associations {
		if batchEvidence[association.Metadata.EvidenceKey] {
			continue
		}
		if _, err := client.Evidence.Query().Where(evidence.EvidenceKeyEQ(association.Metadata.EvidenceKey)).Only(ctx); ent.IsNotFound(err) {
			return fmt.Errorf("%w: missing evidence %s", ErrInvalidIngest, association.Metadata.EvidenceKey)
		} else if err != nil {
			return err
		}
	}
	return nil
}

func ensureAssociationEndpoints(ctx context.Context, client *ent.Client, batch domain.IngestBatch) error {
	batchObjects := make(map[string]bool)
	for _, object := range batch.Objects {
		batchObjects[object.Key] = true
	}
	for _, association := range batch.Associations {
		if !batchObjects[association.From.Key] {
			if _, err := objectByKeyWithClient(ctx, client, association.From.Key); err != nil {
				return err
			}
		}
		if !batchObjects[association.To.Key] {
			if _, err := objectByKeyWithClient(ctx, client, association.To.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertEvidence(ctx context.Context, client *ent.Client, batch domain.IngestBatch, row domain.Evidence) error {
	if row.RunKey == "" {
		row.RunKey = batch.RunKey
	}
	if row.Source == "" {
		row.Source = batch.Source
	}
	if row.SourceInstance == "" {
		row.SourceInstance = batch.SourceInstance
	}
	if row.ObservedAt.IsZero() {
		row.ObservedAt = batch.ObservedAt
	}

	existing, err := client.Evidence.Query().
		Where(evidence.EvidenceKeyEQ(row.EvidenceKey)).
		Only(ctx)
	if err == nil {
		if existing.TextHash != row.TextHash {
			return fmt.Errorf("%w: evidence_key %s changed text hash", ErrIngestConflict, row.EvidenceKey)
		}
		return existing.Update().
			SetRunKey(row.RunKey).
			SetSource(row.Source).
			SetSourceInstance(row.SourceInstance).
			SetSnapshotKey(row.SnapshotKey).
			SetSourceURL(row.SourceURL).
			SetSummary(row.Summary).
			SetQuotedText(row.QuotedText).
			SetConfidence(row.Confidence).
			SetObservedAt(row.ObservedAt).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return client.Evidence.Create().
		SetEvidenceKey(row.EvidenceKey).
		SetRunKey(row.RunKey).
		SetSource(row.Source).
		SetSourceInstance(row.SourceInstance).
		SetSnapshotKey(row.SnapshotKey).
		SetSourceURL(row.SourceURL).
		SetTextHash(row.TextHash).
		SetSummary(row.Summary).
		SetQuotedText(row.QuotedText).
		SetConfidence(row.Confidence).
		SetObservedAt(row.ObservedAt).
		Exec(ctx)
}

func upsertSourceEvent(ctx context.Context, client *ent.Client, batch domain.IngestBatch, event domain.SourceEvent) error {
	if event.RunKey == "" {
		event.RunKey = batch.RunKey
	}
	if event.Source == "" {
		event.Source = batch.Source
	}
	if event.SourceInstance == "" {
		event.SourceInstance = batch.SourceInstance
	}
	if event.ObservedAt.IsZero() {
		event.ObservedAt = batch.ObservedAt
	}

	existing, err := client.SourceEvent.Query().
		Where(sourceevent.EventKeyEQ(event.EventKey)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetRunKey(event.RunKey).
			SetSource(event.Source).
			SetSourceInstance(event.SourceInstance).
			SetSnapshotKey(event.SnapshotKey).
			SetSourceObjectType(event.SourceObjectType).
			SetSourceObjectID(event.SourceObjectID).
			SetEventType(event.EventType).
			SetObservedAt(event.ObservedAt).
			SetPayloadJSON(event.PayloadJSON).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return client.SourceEvent.Create().
		SetEventKey(event.EventKey).
		SetRunKey(event.RunKey).
		SetSource(event.Source).
		SetSourceInstance(event.SourceInstance).
		SetSnapshotKey(event.SnapshotKey).
		SetSourceObjectType(event.SourceObjectType).
		SetSourceObjectID(event.SourceObjectID).
		SetEventType(event.EventType).
		SetObservedAt(event.ObservedAt).
		SetPayloadJSON(event.PayloadJSON).
		Exec(ctx)
}

func upsertBatchCheckpoint(ctx context.Context, client *ent.Client, batch domain.IngestBatch) error {
	checkpoint := *batch.Checkpoint
	if checkpoint.Source == "" {
		checkpoint.Source = batch.Source
	}
	if checkpoint.SourceInstance == "" {
		checkpoint.SourceInstance = batch.SourceInstance
	}
	if checkpoint.Slice == "" {
		checkpoint.Slice = batch.Slice
	}
	if checkpoint.UpdatedAt.IsZero() {
		checkpoint.UpdatedAt = batch.ObservedAt
	}
	return upsertSourceCheckpoint(ctx, client, sourceCheckpointUpdate{
		Source:              checkpoint.Source,
		SourceInstance:      checkpoint.SourceInstance,
		Slice:               checkpoint.Slice,
		Status:              string(domain.SourceStatusRunning),
		CheckpointKey:       checkpoint.CheckpointKey,
		CheckpointValue:     checkpoint.CheckpointValue,
		LastAttemptedRunKey: batch.RunKey,
		UpdatedAt:           checkpoint.UpdatedAt,
	})
}

type sourceCheckpointUpdate struct {
	Source               string
	SourceInstance       string
	Slice                string
	Status               string
	CheckpointKey        string
	CheckpointValue      string
	LastSuccessfulRunKey string
	LastAttemptedRunKey  string
	LastErrorKey         string
	UpdatedAt            time.Time
}

func upsertSourceCheckpoint(ctx context.Context, client *ent.Client, update sourceCheckpointUpdate) error {
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = time.Now().UTC()
	}
	existing, err := client.SourceCheckpoint.Query().
		Where(
			sourcecheckpoint.SourceEQ(update.Source),
			sourcecheckpoint.SourceInstanceEQ(update.SourceInstance),
			sourcecheckpoint.SliceEQ(update.Slice),
		).
		Only(ctx)
	if err == nil {
		builder := existing.Update().
			SetStatus(update.Status).
			SetUpdatedAt(update.UpdatedAt)
		if update.CheckpointKey != "" {
			builder.SetCheckpointKey(update.CheckpointKey)
		}
		if update.CheckpointValue != "" {
			builder.SetCheckpointValue(update.CheckpointValue)
		}
		if update.LastSuccessfulRunKey != "" {
			builder.SetLastSuccessfulRunKey(update.LastSuccessfulRunKey)
		}
		if update.LastAttemptedRunKey != "" {
			builder.SetLastAttemptedRunKey(update.LastAttemptedRunKey)
		}
		if update.LastErrorKey != "" {
			builder.SetLastErrorKey(update.LastErrorKey)
		}
		return builder.Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return client.SourceCheckpoint.Create().
		SetSource(update.Source).
		SetSourceInstance(update.SourceInstance).
		SetSlice(update.Slice).
		SetStatus(update.Status).
		SetCheckpointKey(update.CheckpointKey).
		SetCheckpointValue(update.CheckpointValue).
		SetLastSuccessfulRunKey(update.LastSuccessfulRunKey).
		SetLastAttemptedRunKey(update.LastAttemptedRunKey).
		SetLastErrorKey(update.LastErrorKey).
		SetUpdatedAt(update.UpdatedAt).
		Exec(ctx)
}

func (s *EntStore) countObjectsByType(ctx context.Context, source, sourceInstance string) (map[domain.ObjectType]int, error) {
	objects, err := s.client.Object.Query().
		Where(entobject.SourceEQ(source), entobject.SourceInstanceEQ(sourceInstance)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[domain.ObjectType]int)
	for _, object := range objects {
		counts[domain.ObjectType(object.ObjectType)]++
	}
	return counts, nil
}

func ingestRunToDomain(run *ent.IngestRun) domain.IngestRun {
	return domain.IngestRun{
		RunKey:         run.RunKey,
		Source:         run.Source,
		SourceInstance: run.SourceInstance,
		Slice:          run.Slice,
		MapperVersion:  run.MapperVersion,
		Status:         domain.IngestRunStatus(run.Status),
		StartedAt:      run.StartedAt,
		CompletedAt:    run.CompletedAt,
		ErrorCode:      domain.IngestErrorCode(run.ErrorCode),
		ErrorMessage:   run.ErrorMessage,
	}
}

func sourceSnapshotToDomain(snapshot *ent.SourceSnapshot) domain.SourceSnapshot {
	return domain.SourceSnapshot{
		SourceSnapshotWrite: domain.SourceSnapshotWrite{
			RunKey:           snapshot.RunKey,
			Source:           snapshot.Source,
			SourceInstance:   snapshot.SourceInstance,
			SnapshotKey:      snapshot.SnapshotKey,
			SourceObjectType: snapshot.SourceObjectType,
			SourceObjectID:   snapshot.SourceObjectID,
			BodySHA256:       snapshot.BodySha256,
			BodyRef:          snapshot.BodyRef,
			SourceURL:        snapshot.SourceURL,
			FetchedAt:        snapshot.FetchedAt,
			HeadersJSON:      snapshot.HeadersJSON,
		},
		CreatedAt: snapshot.CreatedAt,
	}
}
