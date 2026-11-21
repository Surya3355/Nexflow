package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// PurgeResult counts what a purge removed.
type PurgeResult struct {
	ArchiveRows int
	Documents   int
}

// FileRemover deletes a stored file by its key. Deleting a missing file must
// succeed, so a purge can be retried safely.
type FileRemover interface {
	Delete(key string) error
}

type nopRemover struct{}

func (nopRemover) Delete(string) error { return nil }

// SetFileRemover sets the store used to delete files during purge.
func (s *Service) SetFileRemover(r FileRemover) { s.files = r }

// istZone is India Standard Time. Document dates are compared on IST days (spec §9).
var istZone = time.FixedZone("IST", 5*60*60+30*60)

// PurgeExpired removes archive rows and documents whose purge date has passed,
// along with their stored files. Audit rows are kept (spec §4.6). Files are
// deleted before their rows; if a file delete fails, the rows are kept and the
// next run retries.
func (s *Service) PurgeExpired(ctx context.Context, now time.Time) (PurgeResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PurgeResult{}, fmt.Errorf("purge: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var res PurgeResult

	rows, err := tx.Query(ctx, `SELECT id, snapshot::text FROM archive WHERE purge_after < $1 FOR UPDATE`, now)
	if err != nil {
		return PurgeResult{}, fmt.Errorf("purge: archive: %w", err)
	}
	type archived struct {
		id       int64
		snapshot string
	}
	var expired []archived
	for rows.Next() {
		var a archived
		if err := rows.Scan(&a.id, &a.snapshot); err != nil {
			rows.Close()
			return PurgeResult{}, fmt.Errorf("purge: scan archive: %w", err)
		}
		expired = append(expired, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return PurgeResult{}, fmt.Errorf("purge: archive rows: %w", err)
	}
	for _, a := range expired {
		keys, err := snapshotDocumentKeys([]byte(a.snapshot))
		if err != nil {
			return PurgeResult{}, err
		}
		for _, key := range keys {
			if err := s.files.Delete(key); err != nil {
				return PurgeResult{}, fmt.Errorf("purge: delete file %s: %w", key, err)
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM archive WHERE id = $1`, a.id); err != nil {
			return PurgeResult{}, fmt.Errorf("purge: delete archive row: %w", err)
		}
		res.ArchiveRows++
	}

	// Live documents of dropped or disbursed cases, past their purge date.
	istDay := istDate(now)
	docs, err := tx.Query(ctx, `SELECT id, storage_key FROM documents WHERE purge_after < $1::date FOR UPDATE`, istDay)
	if err != nil {
		return PurgeResult{}, fmt.Errorf("purge: documents: %w", err)
	}
	type doc struct {
		id  int64
		key string
	}
	var expiredDocs []doc
	for docs.Next() {
		var d doc
		if err := docs.Scan(&d.id, &d.key); err != nil {
			docs.Close()
			return PurgeResult{}, fmt.Errorf("purge: scan document: %w", err)
		}
		expiredDocs = append(expiredDocs, d)
	}
	docs.Close()
	if err := docs.Err(); err != nil {
		return PurgeResult{}, fmt.Errorf("purge: document rows: %w", err)
	}
	for _, d := range expiredDocs {
		if err := s.files.Delete(d.key); err != nil {
			return PurgeResult{}, fmt.Errorf("purge: delete file %s: %w", d.key, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, d.id); err != nil {
			return PurgeResult{}, fmt.Errorf("purge: delete document row: %w", err)
		}
		res.Documents++
	}

	if err := tx.Commit(ctx); err != nil {
		return PurgeResult{}, fmt.Errorf("purge: commit: %w", err)
	}
	return res, nil
}

// snapshotDocumentKeys returns the storage keys of the documents in an archive snapshot.
func snapshotDocumentKeys(snapshot []byte) ([]string, error) {
	var snap struct {
		Children struct {
			Documents []struct {
				StorageKey string `json:"storage_key"`
			} `json:"documents"`
		} `json:"children"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return nil, fmt.Errorf("purge: snapshot: %w", err)
	}
	keys := make([]string, 0, len(snap.Children.Documents))
	for _, d := range snap.Children.Documents {
		keys = append(keys, d.StorageKey)
	}
	return keys, nil
}

func istDate(t time.Time) time.Time {
	local := t.In(istZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
