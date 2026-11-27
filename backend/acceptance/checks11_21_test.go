package acceptance

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/alerts"
)

// Check 11: search by name, mobile, tracking number and bank returns results grouped by customer.
func TestAcceptance11(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000011")
	c := e.newCase(access.RoleEmployee, cust, "100000")
	e.moveStage(access.RoleEmployee, c, "logged_in", "SRCH11")
	for _, q := range []string{"Ravi", "9100000011", "SRCH11", "HDFC"} {
		var groups []struct {
			Customer struct {
				ID int64 `json:"id"`
			} `json:"customer"`
			Cases []map[string]any `json:"cases"`
		}
		e.json(access.RoleEmployee, "GET", "/api/search?q="+q, nil, http.StatusOK, &groups)
		if len(groups) != 1 || groups[0].Customer.ID != cust || len(groups[0].Cases) != 1 {
			t.Fatalf("search %q = %+v, want one group for the customer with one case", q, groups)
		}
	}
}

// Check 12: filters by stage, bank, date range and assigned employee work together.
func TestAcceptance12(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000012"), "100000")
	today := time.Now().In(time.FixedZone("IST", 5*3600+1800)).Format("2006-01-02")
	var ids []int64
	count := func(q string) int {
		var list []struct {
			ID int64 `json:"id"`
		}
		e.json(access.RoleAdmin, "GET", "/api/cases?"+q, nil, http.StatusOK, &list)
		ids = ids[:0]
		for _, x := range list {
			ids = append(ids, x.ID)
		}
		return len(list)
	}
	if n := count(fmt.Sprintf("stage_id=%d&bank_id=%d&assignee_id=%d&from=%s&to=%s", e.stage["received"], e.bank, e.ids[access.RoleEmployee], today, today)); n != 1 || ids[0] != c {
		t.Fatalf("combined filters returned %d (%v), want the one case", n, ids)
	}
	if n := count(fmt.Sprintf("assignee_id=%d", e.ids[access.RoleAdmin])); n != 0 {
		t.Fatalf("assignee filter for another person returned %d, want 0", n)
	}
	if n := count("from=2000-01-01&to=2000-01-02"); n != 0 {
		t.Fatalf("old date range returned %d, want 0", n)
	}
}

// Check 13: dashboard counts match the counting rules in spec §9.
func TestAcceptance13(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000013")
	c := e.newCase(access.RoleEmployee, cust, "1500000")
	e.moveStage(access.RoleEmployee, c, "logged_in", "DASH13")
	e.moveStage(access.RoleEmployee, c, "under_sanction", "")
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/stage", c), map[string]any{
		"stage_id": e.stage["sanctioned"],
		"sanction": map[string]any{"amount": "1500000", "rate_percent": "9.25", "interest_type": "floating", "tenure_months": 240},
	}, http.StatusOK, nil)
	var d struct {
		NewCustomersToday int `json:"new_customers_today"`
		NewCasesToday     int `json:"new_cases_today"`
		Sanctions         struct {
			Count  int    `json:"count"`
			Amount string `json:"amount"`
		} `json:"sanctions"`
	}
	e.json(access.RoleEmployee, "GET", "/api/reports/dashboard", nil, http.StatusOK, &d)
	if d.NewCustomersToday != 1 || d.NewCasesToday != 1 {
		t.Fatalf("today counts = %d customers, %d cases; want 1 and 1", d.NewCustomersToday, d.NewCasesToday)
	}
	if d.Sanctions.Count != 1 || d.Sanctions.Amount != "1500000.00" {
		t.Fatalf("sanctions = %+v, want 1 case, 1500000.00", d.Sanctions)
	}
}

// Check 14: CSV export opens in Excel (BOM, header) and matches the on-screen report.
func TestAcceptance14(t *testing.T) {
	e := newEnv(t)
	e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000014"), "250000")
	resp := e.call(e.clients[access.RoleEmployee], "GET", "/api/reports/export.csv", nil)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("CSV status %d or missing BOM", resp.StatusCode)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatalf("CSV parse: %v", err)
	}
	var screen struct {
		Rows [][]string `json:"rows"`
	}
	e.json(access.RoleEmployee, "GET", "/api/reports/rows", nil, http.StatusOK, &screen)
	if len(records)-1 != len(screen.Rows) {
		t.Fatalf("CSV rows = %d, screen rows = %d", len(records)-1, len(screen.Rows))
	}
	for i := range screen.Rows {
		if strings.Join(records[i+1], "|") != strings.Join(screen.Rows[i], "|") {
			t.Fatalf("row %d differs: CSV %v, screen %v", i, records[i+1], screen.Rows[i])
		}
	}
}

// Check 15: document upload accepts only PDF, JPG and PNG up to 10 MB, and keeps only the latest version.
func TestAcceptance15(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000015"), "100000")
	resp := e.upload(access.RoleEmployee, c, "PAN", "pan.pdf", pdf(200))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PDF upload = %d, want 201", resp.StatusCode)
	}
	resp = e.upload(access.RoleEmployee, c, "Form", "form.pdf", []byte("<html>not a pdf</html>"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("HTML named PDF = %d, want 400", resp.StatusCode)
	}
	resp = e.upload(access.RoleEmployee, c, "PAN", "pan-v2.pdf", pdf(300))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second version = %d, want 201", resp.StatusCode)
	}
	if n := e.count(`SELECT count(*) FROM documents WHERE case_id=$1`, c); n != 1 {
		t.Fatalf("documents under one label = %d, want 1 (latest only)", n)
	}
}

// Check 16: documents are not reachable without a valid short-lived link.
func TestAcceptance16(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000016"), "100000")
	resp := e.upload(access.RoleEmployee, c, "PAN", "pan.pdf", pdf(200))
	resp.Body.Close()
	var docID int64
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT id FROM documents WHERE case_id=$1`, c), &docID)

	bare := e.call(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/documents/%d", docID), nil)
	bare.Body.Close()
	if bare.StatusCode != http.StatusNotFound {
		t.Fatalf("file without a link = %d, want 404", bare.StatusCode)
	}
	var link struct {
		URL string `json:"url"`
	}
	e.json(access.RoleEmployee, "GET", fmt.Sprintf("/api/cases/%d/documents/%d/link", c, docID), nil, http.StatusOK, &link)
	ok := e.call(e.clients[access.RoleEmployee], "GET", link.URL, nil)
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("valid link = %d, want 200", ok.StatusCode)
	}
	tampered := e.call(e.clients[access.RoleEmployee], "GET", link.URL+"x", nil)
	tampered.Body.Close()
	if tampered.StatusCode != http.StatusNotFound {
		t.Fatalf("tampered link = %d, want 404", tampered.StatusCode)
	}
}

// Check 17: assignment alert goes to the new assignee; a follow-up due alert is queued on the due date.
func TestAcceptance17(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000017"), "100000")
	colleague := e.seedUser("colleague17", "colleague17@example.com", access.RoleEmployee)
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/assign", c), map[string]int64{"assignee_id": colleague}, http.StatusOK, nil)

	sender := &recordingSender{}
	if _, err := e.queue.Dispatch(ctx, sender, 50); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !sender.sentTo("colleague17@example.com", "assigned to you") {
		t.Fatalf("assignment email not sent to the new assignee: %v", sender.to)
	}

	due := time.Date(2026, 11, 17, 10, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
	if _, err := e.pool.Exec(ctx, `UPDATE cases SET follow_up_date = $1 WHERE id = $2`, due.Format("2006-01-02"), c); err != nil {
		t.Fatalf("set follow-up: %v", err)
	}
	if _, err := e.queue.RunFollowUps(ctx, due); err != nil {
		t.Fatalf("RunFollowUps: %v", err)
	}
	if n := e.count(`SELECT count(*) FROM alert_log WHERE case_id=$1 AND kind=$2`, c, alerts.KindFollowUpDue); n != 1 {
		t.Fatalf("due alerts on the due date = %d, want 1", n)
	}
}

type recordingSender struct {
	to []string
}

func (r *recordingSender) Send(_ context.Context, to, subject, _ string) error {
	r.to = append(r.to, to+"|"+subject)
	return nil
}

func (r *recordingSender) sentTo(addr, subjectPart string) bool {
	for _, s := range r.to {
		if strings.HasPrefix(s, addr+"|") && strings.Contains(s, subjectPart) {
			return true
		}
	}
	return false
}

// Check 18: only admin can change roles, banks, stages and dropdowns; the server refuses each other role.
func TestAcceptance18(t *testing.T) {
	e := newEnv(t)
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		e.json(role, "POST", "/api/admin/banks", map[string]string{"name": "X-" + string(role)}, http.StatusForbidden, nil)
		e.json(role, "POST", "/api/admin/dropdowns", map[string]any{"category": "drop_reason", "value": "Y-" + string(role), "sort_order": 9}, http.StatusForbidden, nil)
		e.json(role, "POST", "/api/admin/users", map[string]string{"username": "z" + string(role), "full_name": "Z", "role": "employee", "password": password}, http.StatusForbidden, nil)
		e.json(role, "PATCH", fmt.Sprintf("/api/admin/stages/%d", e.stage["sanctioned"]), map[string]bool{"requires_document": true}, http.StatusForbidden, nil)
	}
	e.json(access.RoleAdmin, "POST", "/api/admin/banks", map[string]string{"name": "ICICI"}, http.StatusCreated, nil)
}

// Check 19: the daily backup runs and retains 30 days (infra/backup retention test).
func TestAcceptance19(t *testing.T) {
	script, err := filepath.Abs("../../infra/backup/test_retention.sh")
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	out, err := exec.Command("bash", script).CombinedOutput()
	if err != nil {
		t.Fatalf("retention test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("retention test did not pass: %s", out)
	}
}

// Check 20: any staff member can assign or reassign at any stage; each change is in assignment history.
func TestAcceptance20(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleManager, e.newCustomer(access.RoleManager, "9100000020"), "100000")
	e.moveStage(access.RoleEmployee, c, "logged_in", "ASG20")
	e.moveStage(access.RoleEmployee, c, "disbursed", "")
	colleague := e.seedUser("colleague20", "colleague20@example.com", access.RoleEmployee)
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/assign", c), map[string]int64{"assignee_id": colleague}, http.StatusOK, nil)
	if n := e.count(`SELECT count(*) FROM case_assignment_events WHERE case_id=$1 AND to_user_id=$2 AND from_user_id IS NOT NULL`, c, colleague); n != 1 {
		t.Fatalf("reassignment history rows = %d, want 1", n)
	}
}

// Check 21: each document shows the stage it was uploaded at; uploads are optional; no checklist.
func TestAcceptance21(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000021"), "100000")
	e.moveStage(access.RoleEmployee, c, "logged_in", "STG21")
	resp := e.upload(access.RoleEmployee, c, "Salary slip", "slip.png", append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 80)...))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload = %d, want 201", resp.StatusCode)
	}
	var stageID int64
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT stage_id FROM documents WHERE case_id=$1`, c), &stageID)
	if stageID != e.stage["logged_in"] {
		t.Fatalf("document stage = %d, want Logged in (%d)", stageID, e.stage["logged_in"])
	}
	e.moveStage(access.RoleEmployee, c, "under_sanction", "")
}
