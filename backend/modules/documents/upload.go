package documents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/documents/documentsdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors for uploads.
var (
	ErrCaseNotFound  = errors.New("documents: case not found")
	ErrCaseDropped   = errors.New("documents: a dropped case takes no new documents")
	ErrLabelRequired = errors.New("documents: a label is required")
)

// UploadedFile is a file as received from the client.
type UploadedFile struct {
	Name    string
	Content []byte
}

// Document is one file attached to a case: the latest version under its label.
type Document struct {
	ID          int64
	CaseID      int64
	StageID     int64
	Label       string
	FileName    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
	UploadedAt  time.Time
	UploadedBy  int64
}

// Service handles uploads and downloads. Files live in Store, not in the database.
type Service struct {
	pool  *pgxpool.Pool
	q     *documentsdb.Queries
	store Store
	rec   history.Recorder
}

// NewService returns a documents service on pool using store for file bytes.
func NewService(pool *pgxpool.Pool, store Store, rec history.Recorder) *Service {
	return &Service{pool: pool, q: documentsdb.New(pool), store: store, rec: rec}
}

// Upload attaches a file to a case under a label. The file is tagged with the
// case's current stage. A new upload with the same label replaces the file
// and removes the old stored object (latest version only, spec §4.4).
func (s *Service) Upload(ctx context.Context, actor access.User, caseID int64, label string, file UploadedFile) (Document, error) {
	if !access.Can(actor.Role, access.ActionUploadDocument) {
		return Document{}, access.ErrForbidden
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return Document{}, ErrLabelRequired
	}
	contentType, err := ValidateFile(file.Name, file.Content)
	if err != nil {
		return Document{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Document{}, fmt.Errorf("documents: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	c, err := q.LockCaseForDocument(txCtx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrCaseNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("documents: lock case: %w", err)
	}
	if c.Status == "dropped" {
		return Document{}, ErrCaseDropped
	}

	key, err := newKey(caseID)
	if err != nil {
		return Document{}, err
	}
	if err := s.store.Put(key, file.Content); err != nil {
		return Document{}, fmt.Errorf("documents: store: %w", err)
	}

	var purge pgtype.Date
	if c.Status == "completed" {
		purge = pgtype.Date{Time: istDate(time.Now()).AddDate(1, 0, 0), Valid: true}
	}

	var doc Document
	doc.CaseID, doc.StageID, doc.Label = caseID, c.CurrentStageID, label
	doc.FileName, doc.ContentType, doc.SizeBytes = file.Name, contentType, int64(len(file.Content))
	doc.StorageKey, doc.UploadedBy = key, actor.ID

	existing, err := q.FindDocumentByLabel(txCtx, documentsdb.FindDocumentByLabelParams{CaseID: caseID, Label: label})
	var oldKey string
	switch {
	case err == nil:
		oldKey = existing.StorageKey
		if err := q.ReplaceDocument(txCtx, documentsdb.ReplaceDocumentParams{
			ID: existing.ID, StageID: c.CurrentStageID, FileName: file.Name, ContentType: contentType,
			SizeBytes: doc.SizeBytes, StorageKey: key, UploadedBy: actor.ID,
		}); err != nil {
			return Document{}, fmt.Errorf("documents: replace: %w", err)
		}
		doc.ID = existing.ID
	case errors.Is(err, pgx.ErrNoRows):
		row, err := q.InsertDocument(txCtx, documentsdb.InsertDocumentParams{
			CaseID: caseID, StageID: c.CurrentStageID, Label: label, FileName: file.Name,
			ContentType: contentType, SizeBytes: doc.SizeBytes, StorageKey: key,
			PurgeAfter: purge, UploadedBy: actor.ID,
		})
		if err != nil {
			return Document{}, fmt.Errorf("documents: insert: %w", err)
		}
		doc.ID, doc.UploadedAt = row.ID, row.UploadedAt.Time
	default:
		return Document{}, fmt.Errorf("documents: find label: %w", err)
	}

	actorID, id := actor.ID, doc.ID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "document.uploaded", EntityType: "document", EntityID: &id,
		Details: map[string]any{"case_id": caseID, "label": label, "stage_id": c.CurrentStageID},
	}); err != nil {
		return Document{}, fmt.Errorf("documents: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		_ = s.store.Delete(key)
		return Document{}, fmt.Errorf("documents: commit: %w", err)
	}
	// The old object goes only after the new row is committed.
	if oldKey != "" {
		if err := s.store.Delete(oldKey); err != nil {
			return doc, fmt.Errorf("documents: saved, but the old file could not be removed (%s): %w", oldKey, err)
		}
	}
	return doc, nil
}

func newKey(caseID int64) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("documents: key: %w", err)
	}
	return fmt.Sprintf("cases/%d/%s", caseID, hex.EncodeToString(b)), nil
}

func istDate(t time.Time) time.Time {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	local := t.In(ist)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// Download loads a document and its file bytes. Callers must check the link first.
func (s *Service) Download(ctx context.Context, docID int64) (Document, []byte, error) {
	row, err := s.q.GetDocument(ctx, docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, nil, ErrNotFoundDoc
	}
	if err != nil {
		return Document{}, nil, fmt.Errorf("documents: read: %w", err)
	}
	content, err := s.store.Get(row.StorageKey)
	if err != nil {
		return Document{}, nil, fmt.Errorf("documents: file: %w", err)
	}
	return Document{
		ID: row.ID, CaseID: row.CaseID, StageID: row.StageID, Label: row.Label,
		FileName: row.FileName, ContentType: row.ContentType, SizeBytes: row.SizeBytes,
		StorageKey: row.StorageKey, UploadedAt: row.UploadedAt.Time, UploadedBy: row.UploadedBy,
	}, content, nil
}

// ErrNotFoundDoc is returned when no document has the ID.
var ErrNotFoundDoc = errors.New("documents: document not found")

// ListForCase returns the latest document under each label of a case.
func (s *Service) ListForCase(ctx context.Context, caseID int64) ([]Document, error) {
	rows, err := s.q.ListCaseDocuments(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("documents: list: %w", err)
	}
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, Document{
			ID: r.ID, CaseID: r.CaseID, StageID: r.StageID, Label: r.Label, FileName: r.FileName,
			ContentType: r.ContentType, SizeBytes: r.SizeBytes, StorageKey: r.StorageKey,
			UploadedAt: r.UploadedAt.Time, UploadedBy: r.UploadedBy,
		})
	}
	return out, nil
}
