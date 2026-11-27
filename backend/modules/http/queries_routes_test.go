package apihttp

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// sendProof posts the multipart proof form: a note and, if name is set, a small PDF.
func sendProof(t *testing.T, e *env, c *http.Client, queryID float64, note, name string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("note", note)
	if name != "" {
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), 100)...))
	}
	mw.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/queries/%d/send", e.srv.URL, int64(queryID)), &buf)
	req.Header.Set("Origin", e.srv.URL)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func istToday() string {
	return time.Now().In(time.FixedZone("IST", 5*60*60+30*60)).Format("2006-01-02")
}

func TestQueryRoutes(t *testing.T) {
	e := newEnv(t)
	emp := e.clients[access.RoleEmployee]
	listPath := fmt.Sprintf("/api/cases/%d/queries", e.ids["case"])

	bad := e.do(emp, "POST", listPath, map[string]any{"body": " ", "raised_on": istToday(), "assignee_id": e.ids["employee"]})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank body = %d, want 400", bad.StatusCode)
	}
	var q map[string]any
	resp := e.do(emp, "POST", listPath, map[string]any{"body": "Builder NOC", "raised_on": istToday(), "due_on": istToday(), "assignee_id": e.ids["employee"]})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add = %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &q)
	if q["number"] != float64(1) || q["status"] != "open" || q["due_on"] != istToday() || q["assignee"].(map[string]any)["name"] == "" || q["proof"] != nil {
		t.Fatalf("query = %v", q)
	}
	id := q["id"].(float64)

	var list struct {
		Queries   []map[string]any `json:"queries"`
		OpenCount int              `json:"open_count"`
	}
	decodeJSON(t, e.do(emp, "GET", listPath, nil), &list)
	if list.OpenCount != 1 || len(list.Queries) != 1 {
		t.Fatalf("list = %+v", list)
	}

	move := func() int {
		r := e.do(emp, "POST", fmt.Sprintf("/api/cases/%d/stage", e.ids["case"]), map[string]any{
			"stage_id": e.ids["sanctioned"], "sanction": map[string]any{"amount": "100000", "rate_percent": "8.5"}})
		r.Body.Close()
		return r.StatusCode
	}
	if s := move(); s != http.StatusConflict {
		t.Fatalf("forward move with open query = %d, want 409", s)
	}
	acc := e.do(emp, "POST", fmt.Sprintf("/api/queries/%d/accept", int64(id)), nil)
	acc.Body.Close()
	if acc.StatusCode != http.StatusConflict {
		t.Fatalf("accept open query = %d, want 409", acc.StatusCode)
	}
	empty := sendProof(t, e, emp, id, "", "")
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("send without proof = %d, want 400", empty.StatusCode)
	}
	resp = sendProof(t, e, emp, id, "Emailed to DOVR", "noc.pdf")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send = %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &q)
	proof := q["proof"].(map[string]any)
	doc := proof["document"].(map[string]any)
	if q["status"] != "sent" || proof["note"] != "Emailed to DOVR" || doc["label"] != "Query 1 proof" || doc["file_name"] != "noc.pdf" {
		t.Fatalf("after send = %v", q)
	}
	resp = e.do(emp, "POST", fmt.Sprintf("/api/queries/%d/accept", int64(id)), nil)
	decodeJSON(t, resp, &q)
	if resp.StatusCode != http.StatusOK || q["status"] != "resolved" {
		t.Fatalf("accept = %d %v", resp.StatusCode, q)
	}
	if s := move(); s != http.StatusOK {
		t.Fatalf("forward move after resolve = %d, want 200", s)
	}
	var rows []map[string]any
	decodeJSON(t, e.do(emp, "GET", "/api/cases", nil), &rows)
	if len(rows) == 0 || rows[0]["open_queries"] != float64(0) {
		t.Fatalf("case rows open_queries = %v", rows)
	}
}

func TestQueryRejectAndRemoveRoutes(t *testing.T) {
	e := newEnv(t)
	emp := e.clients[access.RoleEmployee]
	listPath := fmt.Sprintf("/api/cases/%d/queries", e.ids["case"])
	var q map[string]any
	decodeJSON(t, e.do(emp, "POST", listPath, map[string]any{"body": "PAN", "raised_on": istToday(), "assignee_id": e.ids["employee"]}), &q)
	id := int64(q["id"].(float64))

	resp := sendProof(t, e, emp, float64(id), "on portal", "")
	resp.Body.Close()
	noReason := e.do(emp, "POST", fmt.Sprintf("/api/queries/%d/reject", id), map[string]any{"reason": ""})
	noReason.Body.Close()
	if noReason.StatusCode != http.StatusBadRequest {
		t.Fatalf("reject without reason = %d", noReason.StatusCode)
	}
	resp = e.do(emp, "POST", fmt.Sprintf("/api/queries/%d/reject", id), map[string]any{"reason": "Blurred"})
	decodeJSON(t, resp, &q)
	if resp.StatusCode != http.StatusOK || q["status"] != "open" {
		t.Fatalf("reject = %d %v", resp.StatusCode, q)
	}
	resp = sendProof(t, e, emp, float64(id), "again", "")
	resp.Body.Close()
	resp = e.do(emp, "POST", fmt.Sprintf("/api/queries/%d/remove-proof", id), nil)
	decodeJSON(t, resp, &q)
	if resp.StatusCode != http.StatusOK || q["status"] != "open" {
		t.Fatalf("remove proof = %d %v", resp.StatusCode, q)
	}
	resp = e.do(emp, "PUT", fmt.Sprintf("/api/queries/%d", id), map[string]any{"body": "PAN copy", "raised_on": istToday(), "assignee_id": e.ids["employee"]})
	decodeJSON(t, resp, &q)
	if resp.StatusCode != http.StatusOK || q["body"] != "PAN copy" || q["due_on"] != nil {
		t.Fatalf("edit = %d %v", resp.StatusCode, q)
	}
	del := e.do(emp, "DELETE", fmt.Sprintf("/api/queries/%d", id), nil)
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", del.StatusCode)
	}
	var list struct {
		Queries   []map[string]any `json:"queries"`
		OpenCount int              `json:"open_count"`
		Events    []map[string]any `json:"events"`
	}
	decodeJSON(t, e.do(emp, "GET", listPath, nil), &list)
	if len(list.Queries) != 0 || list.OpenCount != 0 {
		t.Fatalf("after delete = %+v", list)
	}
	last := list.Events[len(list.Events)-1]
	if last["kind"] != "removed" || last["body"] != "PAN copy" || last["by"] == "" {
		t.Fatalf("events tail = %v", last)
	}
	gone := e.do(emp, "DELETE", fmt.Sprintf("/api/queries/%d", id), nil)
	gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("delete again = %d, want 404", gone.StatusCode)
	}
}

func TestQueriesOfMissingCase(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "GET", "/api/cases/99999/queries", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing case = %d, want 404", resp.StatusCode)
	}
}
