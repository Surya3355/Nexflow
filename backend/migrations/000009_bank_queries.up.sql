-- Bank queries on a case, resolved with proof (spec 2026-11-26 bank queries §5.1).
CREATE TABLE case_queries (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id     bigint NOT NULL REFERENCES cases(id),
  number      integer NOT NULL CHECK (number > 0),
  body        text NOT NULL CHECK (btrim(body) <> '' AND char_length(body) <= 500),
  raised_on   date NOT NULL,
  due_on      date CHECK (due_on IS NULL OR due_on >= raised_on),
  assignee_id bigint NOT NULL REFERENCES users(id),
  status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'sent', 'resolved')),
  removed_at  timestamptz,
  removed_by  bigint REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  bigint NOT NULL REFERENCES users(id),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  bigint NOT NULL REFERENCES users(id),
  UNIQUE (case_id, number)
);
CREATE INDEX case_queries_unresolved_idx ON case_queries (case_id) WHERE removed_at IS NULL AND status <> 'resolved';

-- Every step of a query, append-only. A purged proof document leaves the event with no file.
CREATE TABLE case_query_events (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  query_id    bigint NOT NULL REFERENCES case_queries(id),
  kind        text NOT NULL CHECK (kind IN ('raised', 'edited', 'sent', 'accepted', 'rejected', 'proof_removed', 'removed')),
  note        text CHECK (note IS NULL OR char_length(note) <= 500),
  document_id bigint REFERENCES documents(id) ON DELETE SET NULL,
  at          timestamptz NOT NULL DEFAULT now(),
  by_user     bigint NOT NULL REFERENCES users(id)
);
CREATE INDEX case_query_events_query_idx ON case_query_events (query_id, at, id);

ALTER TABLE alert_log DROP CONSTRAINT alert_log_kind_check;
ALTER TABLE alert_log ADD CONSTRAINT alert_log_kind_check
  CHECK (kind IN ('assigned', 'follow_up_due', 'follow_up_overdue', 'query_due', 'query_overdue'));
