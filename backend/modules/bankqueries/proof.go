package bankqueries

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/bankqueries/bankqueriesdb"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/jackc/pgx/v5"
)

// step changes one query inside a transaction: it locks the query (case
// first), lets fn act on it, and returns the query as it now is.
func (s *Service) step(ctx context.Context, actor access.User, id int64,
	fn func(ctx context.Context, q *bankqueriesdb.Queries, tx pgx.Tx, row bankqueriesdb.LockQueryRow) error) (Query, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Query{}, access.ErrForbidden
	}
	var caseID int64
	err := s.inTx(ctx, func(ctx context.Context, q *bankqueriesdb.Queries, tx pgx.Tx) error {
		row, err := lockQuery(ctx, q, id)
		if err != nil {
			return err
		}
		caseID = row.CaseID
		if row.Status == StatusResolved {
			return ErrQueryResolved
		}
		return fn(ctx, q, tx, row)
	})
	if err != nil {
		return Query{}, err
	}
	return s.readBack(ctx, caseID, id)
}

func setStatus(ctx context.Context, q *bankqueriesdb.Queries, id int64, status string, by int64) error {
	if err := q.SetQueryStatus(ctx, bankqueriesdb.SetQueryStatusParams{ID: id, Status: status, UpdatedBy: by}); err != nil {
		return fmt.Errorf("bankqueries: status: %w", err)
	}
	return nil
}

// Send records the proof given for an open query (a note, a file, or both)
// and marks it sent to the bank. A file becomes a case document labelled
// "Query <n> proof"; it is removed again if anything fails.
func (s *Service) Send(ctx context.Context, actor access.User, id int64, note string, file *documents.UploadedFile) (Query, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxText {
		return Query{}, ErrNoteTooLong
	}
	if note == "" && file == nil {
		return Query{}, ErrProofRequired
	}
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Query{}, access.ErrForbidden
	}
	var stored documents.Stored
	if file != nil {
		caseID, err := s.q.QueryCaseID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return Query{}, ErrQueryNotFound
		}
		if err != nil {
			return Query{}, fmt.Errorf("bankqueries: find: %w", err)
		}
		if stored, err = s.docs.StoreFile(caseID, *file); err != nil {
			return Query{}, err
		}
	}
	var caseID int64
	err := s.inTx(ctx, func(ctx context.Context, q *bankqueriesdb.Queries, tx pgx.Tx) error {
		row, err := lockQuery(ctx, q, id)
		if err != nil {
			return err
		}
		caseID = row.CaseID
		switch row.Status {
		case StatusResolved:
			return ErrQueryResolved
		case StatusSent:
			return ErrAlreadySent
		}
		var docID *int64
		var proofFile *eventFile
		if file != nil {
			doc, err := s.docs.InsertTx(ctx, tx, actor, row.CaseID, fmt.Sprintf("Query %d proof", row.Number), stored)
			if err != nil {
				return err
			}
			docID = &doc.ID
			proofFile = &eventFile{docID: doc.ID, name: doc.FileName}
		}
		if err := setStatus(ctx, q, id, StatusSent, actor.ID); err != nil {
			return err
		}
		if err := event(ctx, q, id, "sent", note, proofFile, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.sent", id, map[string]any{"case_id": row.CaseID, "note": note, "document_id": docID})
	})
	if err != nil {
		// Nothing was saved: the stored file belongs to no document.
		s.docs.DiscardFile(stored.Key)
		return Query{}, err
	}
	// Saved: from here on the file is the proof, whatever happens to the read-back.
	return s.readBack(ctx, caseID, id)
}

// Accept marks a query sent to the bank as resolved: the bank accepted the proof.
func (s *Service) Accept(ctx context.Context, actor access.User, id int64) (Query, error) {
	return s.step(ctx, actor, id, func(ctx context.Context, q *bankqueriesdb.Queries, _ pgx.Tx, row bankqueriesdb.LockQueryRow) error {
		if row.Status != StatusSent {
			return ErrNotSent
		}
		if err := setStatus(ctx, q, id, StatusResolved, actor.ID); err != nil {
			return err
		}
		if err := event(ctx, q, id, "accepted", "", nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.accepted", id, map[string]any{"case_id": row.CaseID})
	})
}

// Reject reopens a query whose proof the bank did not accept. The proof sent
// stays in the query's history and in Documents.
func (s *Service) Reject(ctx context.Context, actor access.User, id int64, reason string) (Query, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Query{}, ErrReasonRequired
	}
	if utf8.RuneCountInString(reason) > maxText {
		return Query{}, ErrNoteTooLong
	}
	return s.step(ctx, actor, id, func(ctx context.Context, q *bankqueriesdb.Queries, _ pgx.Tx, row bankqueriesdb.LockQueryRow) error {
		if row.Status != StatusSent {
			return ErrNotSent
		}
		if err := setStatus(ctx, q, id, StatusOpen, actor.ID); err != nil {
			return err
		}
		if err := event(ctx, q, id, "rejected", reason, nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.rejected", id, map[string]any{"case_id": row.CaseID, "reason": reason})
	})
}

// RemoveProof takes back wrong proof: the proof file (if any) is deleted and
// the query is open again.
func (s *Service) RemoveProof(ctx context.Context, actor access.User, id int64) (Query, error) {
	var oldKey string
	got, err := s.step(ctx, actor, id, func(ctx context.Context, q *bankqueriesdb.Queries, tx pgx.Tx, row bankqueriesdb.LockQueryRow) error {
		if row.Status != StatusSent {
			return ErrNotSent
		}
		doc, err := q.LatestSentDocument(ctx, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("bankqueries: proof: %w", err)
		}
		if doc.Valid {
			key, err := s.docs.DeleteTx(ctx, tx, actor, doc.Int64)
			if err != nil && !errors.Is(err, documents.ErrNotFoundDoc) {
				return err
			}
			oldKey = key
		}
		if err := setStatus(ctx, q, id, StatusOpen, actor.ID); err != nil {
			return err
		}
		if err := event(ctx, q, id, "proof_removed", "", nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.proof_removed", id, map[string]any{"case_id": row.CaseID})
	})
	if err != nil {
		return Query{}, err
	}
	// The file goes only after the row is gone for good.
	s.docs.DiscardFile(oldKey)
	return got, nil
}

// Remove takes away a query added by mistake. It stays in the case history;
// any proof file stays in Documents.
func (s *Service) Remove(ctx context.Context, actor access.User, id int64) (Query, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Query{}, access.ErrForbidden
	}
	var removed Query
	err := s.inTx(ctx, func(ctx context.Context, q *bankqueriesdb.Queries, _ pgx.Tx) error {
		row, err := lockQuery(ctx, q, id)
		if err != nil {
			return err
		}
		if row.Status == StatusResolved {
			return ErrQueryResolved
		}
		removed = Query{ID: id, CaseID: row.CaseID, Number: int(row.Number), Status: row.Status}
		if err := q.RemoveQuery(ctx, bankqueriesdb.RemoveQueryParams{ID: id, RemovedBy: pgInt8(actor.ID)}); err != nil {
			return fmt.Errorf("bankqueries: remove: %w", err)
		}
		if err := event(ctx, q, id, "removed", "", nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.removed", id, map[string]any{"case_id": row.CaseID})
	})
	return removed, err
}
