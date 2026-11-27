package apihttp

import (
	"io"
	"net/http"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/bankqueries"
	"github.com/Surya3355/nexflow/backend/modules/documents"
)

// Bank queries on a case (round 15 tasks 8 and 9, spec 2026-11-26 bank queries).

func queryEventJSON(e bankqueries.Event) map[string]any {
	out := map[string]any{"query_id": e.QueryID, "number": e.Number, "body": e.Body, "kind": e.Kind,
		"note": e.Note, "at": e.At, "by": e.By, "document_id": nil, "file_deleted": e.FileDeleted}
	if e.DocumentID != nil {
		out["document_id"] = *e.DocumentID
	}
	return out
}

func queryJSON(q bankqueries.Query) map[string]any {
	out := map[string]any{"id": q.ID, "number": q.Number, "body": q.Body, "raised_on": q.RaisedOn.Format("2006-01-02"),
		"due_on": nil, "assignee": map[string]any{"id": q.AssigneeID, "name": q.AssigneeName}, "status": q.Status, "proof": nil}
	if q.DueOn != nil {
		out["due_on"] = q.DueOn.Format("2006-01-02")
	}
	if p := q.Proof; p != nil {
		proof := map[string]any{"note": p.Note, "document": nil, "at": p.At, "by": p.By, "file_deleted": p.FileDeleted, "file_name": p.FileName}
		if p.DocumentID != nil {
			proof["document"] = map[string]any{"id": *p.DocumentID, "label": p.DocumentLabel, "file_name": p.FileName}
		}
		out["proof"] = proof
	}
	events := make([]map[string]any, 0, len(q.Events))
	for _, e := range q.Events {
		events = append(events, queryEventJSON(e))
	}
	out["events"] = events
	return out
}

func (h *handlers) caseQueries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	list, events, err := h.d.Queries.Load(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	qs := make([]map[string]any, 0, len(list))
	open := 0
	for _, q := range list {
		qs = append(qs, queryJSON(q))
		if q.Status != bankqueries.StatusResolved {
			open++
		}
	}
	evs := make([]map[string]any, 0, len(events))
	for _, e := range events {
		evs = append(evs, queryEventJSON(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"queries": qs, "open_count": open, "events": evs})
}

type queryIn struct {
	Body       string  `json:"body"`
	RaisedOn   string  `json:"raised_on"`
	DueOn      *string `json:"due_on"`
	AssigneeID int64   `json:"assignee_id"`
}

// newQuery reads the dates of a query form; it writes a 400 and returns false when one is bad.
func (in queryIn) newQuery(w http.ResponseWriter) (bankqueries.NewQuery, bool) {
	raised, err := time.Parse("2006-01-02", in.RaisedOn)
	if err != nil {
		badInput(w, "enter the date the bank raised the query")
		return bankqueries.NewQuery{}, false
	}
	nq := bankqueries.NewQuery{Body: in.Body, RaisedOn: raised, AssigneeID: in.AssigneeID}
	if in.DueOn != nil && *in.DueOn != "" {
		due, err := time.Parse("2006-01-02", *in.DueOn)
		if err != nil {
			badInput(w, "the due date is not a valid date")
			return bankqueries.NewQuery{}, false
		}
		nq.DueOn = &due
	}
	return nq, true
}

func (h *handlers) addQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in queryIn
	if !decode(w, r, &in) {
		return
	}
	nq, ok := in.newQuery(w)
	if !ok {
		return
	}
	q, err := h.d.Queries.Add(r.Context(), actor(r), id, nq)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, queryJSON(q))
}

func (h *handlers) editQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in queryIn
	if !decode(w, r, &in) {
		return
	}
	nq, ok := in.newQuery(w)
	if !ok {
		return
	}
	q, err := h.d.Queries.Edit(r.Context(), actor(r), id, nq)
	writeQuery(w, q, err)
}

func writeQuery(w http.ResponseWriter, q bankqueries.Query, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, queryJSON(q))
}

// sendQueryProof reads a multipart form: "note" and an optional "file".
func (h *handlers) sendQueryProof(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if !parseUpload(w, r) {
		return
	}
	var file *documents.UploadedFile
	if f, header, err := r.FormFile("file"); err == nil {
		defer f.Close()
		content, err := io.ReadAll(io.LimitReader(f, documents.MaxFileSize+1))
		if err != nil {
			badInput(w, "could not read the file")
			return
		}
		file = &documents.UploadedFile{Name: header.Filename, Content: content}
	}
	q, err := h.d.Queries.Send(r.Context(), actor(r), id, r.FormValue("note"), file)
	writeQuery(w, q, err)
}

func (h *handlers) acceptQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	q, err := h.d.Queries.Accept(r.Context(), actor(r), id)
	writeQuery(w, q, err)
}

func (h *handlers) rejectQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	q, err := h.d.Queries.Reject(r.Context(), actor(r), id, in.Reason)
	writeQuery(w, q, err)
}

func (h *handlers) removeQueryProof(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	q, err := h.d.Queries.RemoveProof(r.Context(), actor(r), id)
	writeQuery(w, q, err)
}

func (h *handlers) removeQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if _, err := h.d.Queries.Remove(r.Context(), actor(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
