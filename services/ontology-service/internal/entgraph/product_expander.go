package entgraph

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/documentlink"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/pullrequestauthorship"
	"cubicle/services/ontology-service/ent/pullrequestreview"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketassignment"
	"cubicle/services/ontology-service/ent/ticketdocument"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"

	entsql "entgo.io/ent/dialect/sql"
)

// ProductExpander reads a bounded neighborhood from typed product Ent rows.
//
// This is intentionally narrower than the WorkProgram/TPM operating graph. It
// starts with source-backed product objects and typed relationship rows that
// already carry evidence, so generic graph prompts do not crawl raw source
// replay, sync diagnostics, or analytics projections.
type ProductExpander struct {
	client *genent.Client
}

type rankedAssociation struct {
	association    domain.Association
	rankScore      float64
	lastActivityAt time.Time
	updatedAt      time.Time
}

// NewProductExpander returns an Ent-backed graphstore.Expander for product rows.
func NewProductExpander(client *genent.Client) *ProductExpander {
	return &ProductExpander{client: client}
}

// Expand returns a deterministic bounded neighborhood around a product object.
func (e *ProductExpander) Expand(ctx context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if e == nil || e.client == nil {
		return domain.Neighborhood{}, fmt.Errorf("ent product expander: client is required")
	}
	if req.Start.ObjectType == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerObject <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", graphstore.ErrInvalidExpansion)
	}
	start, err := e.object(ctx, req.Start)
	if err != nil {
		return domain.Neighborhood{}, err
	}
	if !objectAllowed(req.ReadFilter, start) {
		return domain.Neighborhood{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, req.Start.Key)
	}

	allowed := associationTypeSet(req.AssociationTypes)
	candidateLimit := expansionCandidateLimit(req.LimitPerObject)
	startRefKey := objectRefKey(start.Ref())
	seenObjects := map[string]domain.Object{startRefKey: start}
	objectOrder := []string{startRefKey}
	seenAssociations := make(map[string]domain.Association)
	associationOrder := make([]string, 0)
	frontier := []domain.ObjectRef{start.Ref()}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.ObjectRef, 0)
		for _, ref := range frontier {
			associations, err := e.associations(ctx, ref, candidateLimit, allowed)
			if err != nil {
				return domain.Neighborhood{}, err
			}
			used := 0
			for _, association := range associations {
				if used >= req.LimitPerObject {
					break
				}
				if !associationAllowed(req.ReadFilter, association) {
					continue
				}
				endpoints, ok, err := e.readableAssociationEndpointObjects(ctx, req.ReadFilter, association)
				if err != nil {
					return domain.Neighborhood{}, err
				}
				if !ok {
					continue
				}
				used++
				key := associationKey(association)
				if _, ok := seenAssociations[key]; !ok {
					seenAssociations[key] = association
					associationOrder = append(associationOrder, key)
				}
				for _, object := range endpoints {
					if objectRefKey(object.Ref()) == objectRefKey(ref) {
						continue
					}
					endpointRefKey := objectRefKey(object.Ref())
					if _, ok := seenObjects[endpointRefKey]; ok {
						continue
					}
					objectKey := objectRefKey(object.Ref())
					seenObjects[objectKey] = object
					objectOrder = append(objectOrder, objectKey)
					next = append(next, object.Ref())
				}
			}
		}
		frontier = next
	}

	out := domain.Neighborhood{
		Objects:      make([]domain.Object, 0, len(objectOrder)),
		Associations: make([]domain.Association, 0, len(associationOrder)),
	}
	for _, key := range objectOrder {
		out.Objects = append(out.Objects, seenObjects[key])
	}
	for _, key := range associationOrder {
		out.Associations = append(out.Associations, seenAssociations[key])
	}
	return out, nil
}

func (e *ProductExpander) readableAssociationEndpointObjects(ctx context.Context, filter domain.ExpandReadFilter, association domain.Association) ([]domain.Object, bool, error) {
	endpoints := []domain.ObjectRef{association.From, association.To}
	out := make([]domain.Object, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Key == "" {
			return nil, false, nil
		}
		object, err := e.object(ctx, endpoint)
		if err != nil {
			return nil, false, err
		}
		if !objectAllowed(filter, object) {
			return nil, false, nil
		}
		out = append(out, object)
	}
	return out, true, nil
}

func (e *ProductExpander) object(ctx context.Context, ref domain.ObjectRef) (domain.Object, error) {
	switch ref.ObjectType {
	case ontology.ObjectTicket:
		row, err := e.client.Ticket.Query().
			Where(ticket.KeyEQ(ref.Key)).
			Only(ctx)
		if genent.IsNotFound(err) {
			return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return domain.Object{}, err
		}
		return ticketObject(row), nil
	case ontology.ObjectPullRequest:
		row, err := e.client.PullRequest.Query().
			Where(pullrequest.KeyEQ(ref.Key)).
			Only(ctx)
		if genent.IsNotFound(err) {
			return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return domain.Object{}, err
		}
		return pullRequestObject(row), nil
	case ontology.ObjectDocument:
		row, err := e.client.Document.Query().
			Where(document.KeyEQ(ref.Key)).
			Only(ctx)
		if genent.IsNotFound(err) {
			return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return domain.Object{}, err
		}
		return documentObject(row), nil
	case ontology.ObjectMessage:
		row, err := e.client.Message.Query().
			Where(message.KeyEQ(ref.Key)).
			Only(ctx)
		if genent.IsNotFound(err) {
			return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return domain.Object{}, err
		}
		return messageObject(row), nil
	case ontology.ObjectPerson:
		row, err := e.client.Person.Query().
			Where(person.KeyEQ(ref.Key)).
			Only(ctx)
		if genent.IsNotFound(err) {
			return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return domain.Object{}, err
		}
		return personObject(row), nil
	default:
		return domain.Object{}, fmt.Errorf("%w: unsupported object type %q", graphstore.ErrMissingObject, ref.ObjectType)
	}
}

func (e *ProductExpander) associations(ctx context.Context, ref domain.ObjectRef, limit int, allowed map[domain.AssociationType]bool) ([]domain.Association, error) {
	ranked := make([]rankedAssociation, 0)
	if anyAssociationTypeAllowed(allowed, ontology.AssocImplementedBy) {
		associations, err := e.ticketPullRequestAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, ontology.AssocDocuments) {
		associations, err := e.ticketDocumentAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, ontology.AssocDiscussedIn) {
		associations, err := e.ticketMessageAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, assignmentAssociationTypes()...) {
		associations, err := e.ticketAssignmentAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, pullRequestAuthorshipAssociationTypes()...) {
		associations, err := e.pullRequestAuthorshipAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, pullRequestReviewAssociationTypes()...) {
		associations, err := e.pullRequestReviewAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	if anyAssociationTypeAllowed(allowed, documentLinkAssociationTypes()...) {
		associations, err := e.documentLinkAssociations(ctx, ref, limit)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, associations...)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return rankedAssociationLess(ranked[i], ranked[j])
	})
	if len(allowed) > 0 {
		filtered := ranked[:0]
		for _, association := range ranked {
			if allowed[association.association.AssociationType] {
				filtered = append(filtered, association)
			}
		}
		ranked = filtered
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]domain.Association, 0, len(ranked))
	for _, association := range ranked {
		out = append(out, association.association)
	}
	return out, nil
}

func (e *ProductExpander) ticketPullRequestAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.TicketPullRequest.Query().
		WithTicket().
		WithPullRequest().
		WithLatestEvidence().
		Limit(limit).
		Order(
			ticketpullrequest.ByRankScore(entsql.OrderDesc()),
			ticketpullrequest.ByLastActivityAt(entsql.OrderDesc()),
			ticketpullrequest.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectTicket:
		ticketRow, err := e.client.Ticket.Query().Where(ticket.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketpullrequest.TicketIDEQ(ticketRow.ID))
	case ontology.ObjectPullRequest:
		pullRequestRow, err := e.client.PullRequest.Query().Where(pullrequest.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketpullrequest.PullRequestIDEQ(pullRequestRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.Ticket == nil || row.Edges.PullRequest == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    ticketPullRequestAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) ticketDocumentAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.TicketDocument.Query().
		WithTicket().
		WithDocument().
		WithLatestEvidence().
		Limit(limit).
		Order(
			ticketdocument.ByRankScore(entsql.OrderDesc()),
			ticketdocument.ByLastActivityAt(entsql.OrderDesc()),
			ticketdocument.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectTicket:
		ticketRow, err := e.client.Ticket.Query().Where(ticket.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketdocument.TicketIDEQ(ticketRow.ID))
	case ontology.ObjectDocument:
		documentRow, err := e.client.Document.Query().Where(document.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketdocument.DocumentIDEQ(documentRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.Ticket == nil || row.Edges.Document == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    ticketDocumentAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) ticketMessageAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.TicketMessage.Query().
		WithTicket().
		WithMessage().
		WithLatestEvidence().
		Limit(limit).
		Order(
			ticketmessage.ByRankScore(entsql.OrderDesc()),
			ticketmessage.ByLastActivityAt(entsql.OrderDesc()),
			ticketmessage.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectTicket:
		ticketRow, err := e.client.Ticket.Query().Where(ticket.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketmessage.TicketIDEQ(ticketRow.ID))
	case ontology.ObjectMessage:
		messageRow, err := e.client.Message.Query().Where(message.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketmessage.MessageIDEQ(messageRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.Ticket == nil || row.Edges.Message == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    ticketMessageAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) ticketAssignmentAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.TicketAssignment.Query().
		WithPerson().
		WithTicket().
		WithLatestEvidence().
		Limit(limit).
		Order(
			ticketassignment.ByRankScore(entsql.OrderDesc()),
			ticketassignment.ByLastActivityAt(entsql.OrderDesc()),
			ticketassignment.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectTicket:
		ticketRow, err := e.client.Ticket.Query().Where(ticket.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketassignment.TicketIDEQ(ticketRow.ID))
	case ontology.ObjectPerson:
		personRow, err := e.client.Person.Query().Where(person.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(ticketassignment.PersonIDEQ(personRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.Ticket == nil || row.Edges.Person == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    ticketAssignmentAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) pullRequestAuthorshipAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.PullRequestAuthorship.Query().
		WithPerson().
		WithPullRequest().
		WithLatestEvidence().
		Limit(limit).
		Order(
			pullrequestauthorship.ByRankScore(entsql.OrderDesc()),
			pullrequestauthorship.ByLastActivityAt(entsql.OrderDesc()),
			pullrequestauthorship.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectPullRequest:
		pullRequestRow, err := e.client.PullRequest.Query().Where(pullrequest.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(pullrequestauthorship.PullRequestIDEQ(pullRequestRow.ID))
	case ontology.ObjectPerson:
		personRow, err := e.client.Person.Query().Where(person.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(pullrequestauthorship.PersonIDEQ(personRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.PullRequest == nil || row.Edges.Person == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    pullRequestAuthorshipAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) pullRequestReviewAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.PullRequestReview.Query().
		WithPerson().
		WithPullRequest().
		WithLatestEvidence().
		Limit(limit).
		Order(
			pullrequestreview.ByRankScore(entsql.OrderDesc()),
			pullrequestreview.ByLastActivityAt(entsql.OrderDesc()),
			pullrequestreview.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectPullRequest:
		pullRequestRow, err := e.client.PullRequest.Query().Where(pullrequest.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(pullrequestreview.PullRequestIDEQ(pullRequestRow.ID))
	case ontology.ObjectPerson:
		personRow, err := e.client.Person.Query().Where(person.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(pullrequestreview.PersonIDEQ(personRow.ID))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.PullRequest == nil || row.Edges.Person == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    pullRequestReviewAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func (e *ProductExpander) documentLinkAssociations(ctx context.Context, ref domain.ObjectRef, limit int) ([]rankedAssociation, error) {
	query := e.client.DocumentLink.Query().
		WithFromDocument().
		WithToDocument().
		WithLatestEvidence().
		Limit(limit).
		Order(
			documentlink.ByRankScore(entsql.OrderDesc()),
			documentlink.ByLastActivityAt(entsql.OrderDesc()),
			documentlink.ByUpdatedAt(entsql.OrderDesc()),
		)

	switch ref.ObjectType {
	case ontology.ObjectDocument:
		documentRow, err := e.client.Document.Query().Where(document.KeyEQ(ref.Key)).Only(ctx)
		if genent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
		}
		if err != nil {
			return nil, err
		}
		query = query.Where(documentlink.Or(documentlink.FromDocumentIDEQ(documentRow.ID), documentlink.ToDocumentIDEQ(documentRow.ID)))
	default:
		return nil, nil
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.FromDocument == nil || row.Edges.ToDocument == nil {
			continue
		}
		out = append(out, rankedAssociation{
			association:    documentLinkAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rankedAssociationLess(out[i], out[j])
	})
	return out, nil
}

func ticketObject(row *genent.Ticket) domain.Object {
	return domain.Object{
		ObjectType:      ontology.ObjectTicket,
		Key:             row.Key,
		Title:           row.Title,
		Source:          row.SourceSystem,
		SourceInstance:  row.SourceInstance,
		ExternalID:      row.ExternalID,
		SourceURL:       row.SourceURL,
		MapperVersion:   row.SourceVersion,
		Visibility:      row.Visibility.String(),
		FreshnessState:  row.FreshnessState.String(),
		ObservedAt:      row.LastConfirmedAt,
		SourceUpdatedAt: row.SourceUpdatedAt,
	}
}

func pullRequestObject(row *genent.PullRequest) domain.Object {
	return domain.Object{
		ObjectType:      ontology.ObjectPullRequest,
		Key:             row.Key,
		Title:           row.Title,
		Source:          row.SourceSystem,
		SourceInstance:  row.SourceInstance,
		ExternalID:      row.ExternalID,
		SourceURL:       row.SourceURL,
		MapperVersion:   row.SourceVersion,
		Visibility:      row.Visibility.String(),
		FreshnessState:  row.FreshnessState.String(),
		ObservedAt:      row.LastConfirmedAt,
		SourceUpdatedAt: row.SourceUpdatedAt,
	}
}

func documentObject(row *genent.Document) domain.Object {
	return domain.Object{
		ObjectType:      ontology.ObjectDocument,
		Key:             row.Key,
		Title:           row.Title,
		Source:          row.SourceSystem,
		SourceInstance:  row.SourceInstance,
		ExternalID:      row.ExternalID,
		SourceURL:       row.SourceURL,
		MapperVersion:   row.SourceVersion,
		Visibility:      row.Visibility.String(),
		FreshnessState:  row.FreshnessState.String(),
		ObservedAt:      row.LastConfirmedAt,
		SourceUpdatedAt: row.SourceUpdatedAt,
	}
}

func messageObject(row *genent.Message) domain.Object {
	return domain.Object{
		ObjectType:      ontology.ObjectMessage,
		Key:             row.Key,
		Title:           row.Key,
		Source:          row.SourceSystem,
		SourceInstance:  row.SourceInstance,
		ExternalID:      row.ExternalID,
		SourceURL:       row.SourceURL,
		MapperVersion:   row.SourceVersion,
		Visibility:      row.Visibility.String(),
		FreshnessState:  row.FreshnessState.String(),
		ObservedAt:      row.LastConfirmedAt,
		SourceUpdatedAt: row.SourceUpdatedAt,
	}
}

func personObject(row *genent.Person) domain.Object {
	title := row.DisplayName
	if strings.TrimSpace(title) == "" {
		title = row.Key
	}
	return domain.Object{
		ObjectType:     ontology.ObjectPerson,
		Key:            row.Key,
		Title:          title,
		Visibility:     row.Visibility.String(),
		FreshnessState: row.FreshnessState.String(),
	}
}

func ticketPullRequestAssociation(row *genent.TicketPullRequest) domain.Association {
	return domain.Association{
		From:            ticketObject(row.Edges.Ticket).Ref(),
		To:              pullRequestObject(row.Edges.PullRequest).Ref(),
		AssociationType: ontology.AssocImplementedBy,
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func ticketDocumentAssociation(row *genent.TicketDocument) domain.Association {
	return domain.Association{
		From:            ticketObject(row.Edges.Ticket).Ref(),
		To:              documentObject(row.Edges.Document).Ref(),
		AssociationType: ontology.AssocDocuments,
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func ticketMessageAssociation(row *genent.TicketMessage) domain.Association {
	return domain.Association{
		From:            ticketObject(row.Edges.Ticket).Ref(),
		To:              messageObject(row.Edges.Message).Ref(),
		AssociationType: ontology.AssocDiscussedIn,
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func ticketAssignmentAssociation(row *genent.TicketAssignment) domain.Association {
	return domain.Association{
		From:            ticketObject(row.Edges.Ticket).Ref(),
		To:              personObject(row.Edges.Person).Ref(),
		AssociationType: domain.AssociationType(row.AssignmentKind.String()),
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func pullRequestAuthorshipAssociation(row *genent.PullRequestAuthorship) domain.Association {
	return domain.Association{
		From:            pullRequestObject(row.Edges.PullRequest).Ref(),
		To:              personObject(row.Edges.Person).Ref(),
		AssociationType: domain.AssociationType(row.AuthorshipKind.String()),
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func pullRequestReviewAssociation(row *genent.PullRequestReview) domain.Association {
	return domain.Association{
		From:            pullRequestObject(row.Edges.PullRequest).Ref(),
		To:              personObject(row.Edges.Person).Ref(),
		AssociationType: domain.AssociationType(row.ReviewKind.String()),
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func documentLinkAssociation(row *genent.DocumentLink) domain.Association {
	return domain.Association{
		From:            documentObject(row.Edges.FromDocument).Ref(),
		To:              documentObject(row.Edges.ToDocument).Ref(),
		AssociationType: domain.AssociationType(row.DocumentLinkKind.String()),
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}

func associationMetadata(
	sourceSystem string,
	sourceInstance string,
	sourceURL string,
	sourceVersion string,
	confidence float64,
	visibility string,
	freshnessState string,
	observedAt time.Time,
	sourceUpdatedAt time.Time,
	evidenceCount int,
	evidenceRow *genent.Evidence,
) domain.AssociationMetadata {
	metadata := domain.AssociationMetadata{
		Source:          sourceSystem,
		SourceInstance:  sourceInstance,
		SourceURL:       sourceURL,
		MapperVersion:   sourceVersion,
		Confidence:      confidence,
		Visibility:      visibility,
		FreshnessState:  freshnessState,
		EvidenceCount:   evidenceCount,
		ObservedAt:      observedAt,
		SourceUpdatedAt: sourceUpdatedAt,
	}
	if evidenceRow != nil {
		metadata.EvidenceKey = evidenceRow.Key
		metadata.EvidenceClaimKind = evidenceRow.ClaimKind.String()
		metadata.EvidenceRelationshipKind = evidenceRow.RelationshipKind
		metadata.EvidenceProofState = evidenceRow.ProofState.String()
		metadata.EvidenceSource = evidenceRow.SourceSystem
		metadata.EvidenceSourceInstance = evidenceRow.SourceInstance
		metadata.EvidenceLocatorKind = evidenceRow.LocatorKind
		if metadata.Source == "" {
			metadata.Source = evidenceRow.SourceSystem
		}
		if metadata.SourceInstance == "" {
			metadata.SourceInstance = evidenceRow.SourceInstance
		}
		if metadata.SourceURL == "" {
			metadata.SourceURL = evidenceRow.SourceURL
		}
		if metadata.MapperVersion == "" {
			metadata.MapperVersion = evidenceRow.SourceVersion
		}
		if metadata.Confidence == 0 {
			metadata.Confidence = evidenceRow.Confidence
		}
		if metadata.Visibility == "" {
			metadata.Visibility = evidenceRow.Visibility.String()
		}
		if metadata.FreshnessState == "" {
			metadata.FreshnessState = evidenceRow.FreshnessState.String()
		}
		if metadata.ObservedAt.IsZero() {
			metadata.ObservedAt = evidenceRow.ObservedAt
		}
		if metadata.SourceUpdatedAt.IsZero() {
			metadata.SourceUpdatedAt = evidenceRow.SourceUpdatedAt
		}
	}
	return metadata
}

func associationTypeSet(types []domain.AssociationType) map[domain.AssociationType]bool {
	if len(types) == 0 {
		return nil
	}
	out := make(map[domain.AssociationType]bool, len(types))
	for _, typ := range types {
		out[typ] = true
	}
	return out
}

func anyAssociationTypeAllowed(allowed map[domain.AssociationType]bool, types ...domain.AssociationType) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, typ := range types {
		if allowed[typ] {
			return true
		}
	}
	return false
}

func assignmentAssociationTypes() []domain.AssociationType {
	return []domain.AssociationType{
		domain.AssociationType(ticketassignment.AssignmentKindAssignee.String()),
		domain.AssociationType(ticketassignment.AssignmentKindReporter.String()),
		domain.AssociationType(ticketassignment.AssignmentKindOwner.String()),
	}
}

func pullRequestAuthorshipAssociationTypes() []domain.AssociationType {
	return []domain.AssociationType{
		domain.AssociationType(pullrequestauthorship.AuthorshipKindAuthor.String()),
		domain.AssociationType(pullrequestauthorship.AuthorshipKindCreator.String()),
	}
}

func pullRequestReviewAssociationTypes() []domain.AssociationType {
	return []domain.AssociationType{
		domain.AssociationType(pullrequestreview.ReviewKindReviewer.String()),
		domain.AssociationType(pullrequestreview.ReviewKindApprover.String()),
		domain.AssociationType(pullrequestreview.ReviewKindCommenter.String()),
		domain.AssociationType(pullrequestreview.ReviewKindRequestedReviewer.String()),
	}
}

func documentLinkAssociationTypes() []domain.AssociationType {
	return []domain.AssociationType{domain.AssociationType(documentlink.DocumentLinkKindLinksTo.String())}
}

func expansionCandidateLimit(limit int) int {
	if limit <= 0 {
		return limit
	}
	candidateLimit := limit * 10
	if candidateLimit < 50 {
		candidateLimit = 50
	}
	if candidateLimit > 500 {
		candidateLimit = 500
	}
	return candidateLimit
}

func objectAllowed(filter domain.ExpandReadFilter, object domain.Object) bool {
	if filter.ObjectAllowed == nil {
		return true
	}
	return filter.ObjectAllowed(object)
}

func associationAllowed(filter domain.ExpandReadFilter, association domain.Association) bool {
	if filter.AssociationAllowed == nil {
		return true
	}
	return filter.AssociationAllowed(association)
}

func objectRefKey(ref domain.ObjectRef) string {
	return string(ref.ObjectType) + "\x00" + ref.Key
}

func associationKey(association domain.Association) string {
	if strings.TrimSpace(association.Key) != "" {
		return strings.TrimSpace(association.Key)
	}
	return string(association.From.ObjectType) + ":" + association.From.Key +
		"|" + string(association.AssociationType) +
		"|" + string(association.To.ObjectType) + ":" + association.To.Key
}

func rankedAssociationLess(left rankedAssociation, right rankedAssociation) bool {
	if left.rankScore != right.rankScore {
		return left.rankScore > right.rankScore
	}
	if !left.lastActivityAt.Equal(right.lastActivityAt) {
		return left.lastActivityAt.After(right.lastActivityAt)
	}
	if !left.updatedAt.Equal(right.updatedAt) {
		return left.updatedAt.After(right.updatedAt)
	}
	return associationKey(left.association) < associationKey(right.association)
}
