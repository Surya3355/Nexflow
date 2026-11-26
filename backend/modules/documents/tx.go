package documents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/documents/documentsdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Stored is a file already written to the store but not yet recorded in the
// database. The caller records it with InsertTx, or discards it on failure.
type Stored struct {
	Key, Name, ContentType string
	Size                   int64
}

// StoreFile checks a file and writes its bytes to the store.
func (s *Service) StoreFile(caseID int64, file UploadedFile) (Stored, error) {
	contentType, err := ValidateFile(file.Name, file.Content)
	if err != nil {
		return Stored{}, err
	}
	key, err := newKey(caseID)
	if err != nil {
		return Stored{}, err
	}
	if err := s.store.Put(key, file.Content); err != nil {
		return Stored{}, fmt.Errorf("documents: store: %w", err)
	}
	return Stored{Key: key, Name: file.Name, ContentType: contentType, Size: int64(len(file.Content))}, nil
}

// DiscardFile removes a stored file. Best effort: a leftover file is harmless.
func (s *Service) DiscardFile(key string) {
	if key != "" {
		_ = s.store.Delete(key)
	}
}

// InsertTx records a stored file on a case inside the caller's transaction,
// at the case's current stage. It never replaces a document: when baseLabel is
// taken it uses "baseLabel 2", "baseLabel 3", … The caller holds the case lock.
func (s *Service) InsertTx(ctx context.Context, tx pgx.Tx, actor access.User, caseID int64, baseLabel string, f Stored) (Document, error) {
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)
	c, err := q.CaseStageForDocument(txCtx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrCaseNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("documents: case: %w", err)
	}
	label := baseLabel
	for n := 2; ; n++ {
		taken, err := q.CaseLabelTaken(txCtx, documentsdb.CaseLabelTakenParams{CaseID: caseID, Label: label})
		if err != nil {
			return Document{}, fmt.Errorf("documents: label: %w", err)
		}
		if !taken {
			break
		}
		label = fmt.Sprintf("%s %d", baseLabel, n)
	}
	var purge pgtype.Date
	if c.Status == "completed" {
		purge = pgtype.Date{Time: istDate(time.Now()).AddDate(1, 0, 0), Valid: true}
	}
	row, err := q.InsertDocument(txCtx, documentsdb.InsertDocumentParams{
		CaseID: caseID, StageID: c.CurrentStageID, Label: label, FileName: f.Name,
		ContentType: f.ContentType, SizeBytes: f.Size, StorageKey: f.Key,
		PurgeAfter: purge, UploadedBy: actor.ID,
	})
	if err != nil {
		return Document{}, fmt.Errorf("documents: insert: %w", err)
	}
	actorID, id := actor.ID, row.ID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "document.uploaded", EntityType: "document", EntityID: &id,
		Details: map[string]any{"case_id": caseID, "label": label, "stage_id": c.CurrentStageID},
	}); err != nil {
		return Document{}, fmt.Errorf("documents: audit: %w", err)
	}
	return Document{
		ID: row.ID, CaseID: caseID, StageID: c.CurrentStageID, Label: label, FileName: f.Name,
		ContentType: f.ContentType, SizeBytes: f.Size, StorageKey: f.Key,
		UploadedAt: row.UploadedAt.Time, UploadedBy: actor.ID,
	}, nil
}

// DeleteTx removes a document row inside the caller's transaction and returns
// its storage key, so the caller can delete the file once the transaction commits.
func (s *Service) DeleteTx(ctx context.Context, tx pgx.Tx, actor access.User, docID int64) (string, error) {
	txCtx := history.WithTx(ctx, tx)
	row, err := s.q.WithTx(tx).DeleteDocument(txCtx, docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFoundDoc
	}
	if err != nil {
		return "", fmt.Errorf("documents: delete: %w", err)
	}
	actorID := actor.ID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "document.removed", EntityType: "document", EntityID: &docID,
		Details: map[string]any{"case_id": row.CaseID, "label": row.Label},
	}); err != nil {
		return "", fmt.Errorf("documents: audit: %w", err)
	}
	return row.StorageKey, nil
}
