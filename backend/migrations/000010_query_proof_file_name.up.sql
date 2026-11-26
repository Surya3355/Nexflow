-- The proof file's name, kept on the "sent" event so the history can say "File deleted"
-- after the document is purged or removed (bank queries review, 2026-11-26).
ALTER TABLE case_query_events ADD COLUMN file_name text CHECK (file_name IS NULL OR char_length(file_name) <= 255);
UPDATE case_query_events e SET file_name = d.file_name FROM documents d WHERE d.id = e.document_id;
