package apihttp

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// expect sends a request and checks the status and, when given, part of the message.
func (e *env) expect(role access.Role, method, path string, body any, status int, msg string) {
	e.t.Helper()
	resp := e.do(e.clients[role], method, path, body)
	got := readAll(e.t, resp)
	if resp.StatusCode != status || (msg != "" && !strings.Contains(got, msg)) {
		e.t.Errorf("%s %s %v: %d %s; want %d containing %q", method, path, body, resp.StatusCode, strings.TrimSpace(got), status, msg)
	}
}

func TestBankNameRules(t *testing.T) {
	e := newEnv(t)
	a := access.RoleAdmin
	e.expect(a, "POST", "/api/admin/banks", map[string]string{"name": "QA-HDFC"}, http.StatusCreated, "")
	e.expect(a, "POST", "/api/admin/banks", map[string]string{"name": "QA-HDFC"}, http.StatusConflict, "already")
	e.expect(a, "POST", "/api/admin/banks", map[string]string{"name": "qa-hdfc"}, http.StatusConflict, "already")
	e.expect(a, "POST", "/api/admin/banks", map[string]string{"name": "   "}, http.StatusBadRequest, "")
	e.expect(a, "POST", "/api/admin/banks", map[string]string{"name": strings.Repeat("b", 101)}, http.StatusBadRequest, "100 characters")
	// Rename follows the same rules; "HDFC" already exists in the test data.
	e.expect(a, "PUT", fmt.Sprintf("/api/admin/banks/%d", e.ids["bank"]), map[string]string{"name": "qa-HDFC"}, http.StatusConflict, "already")
	e.expect(a, "PUT", fmt.Sprintf("/api/admin/banks/%d", e.ids["bank"]), map[string]string{"name": strings.Repeat("b", 101)}, http.StatusBadRequest, "100 characters")
}

func TestEmailAndNameRules(t *testing.T) {
	e := newEnv(t)
	a, s := access.RoleAdmin, access.RoleEmployee
	long := strings.Repeat("n", 151)
	e.expect(s, "POST", "/api/customers", map[string]string{"name": "QA Ravi", "mobile": "9811111111", "email": "nope"}, http.StatusBadRequest, "valid email")
	e.expect(s, "POST", "/api/customers", map[string]string{"name": long, "mobile": "9811111112"}, http.StatusBadRequest, "150 characters")
	e.expect(s, "PUT", fmt.Sprintf("/api/customers/%d", e.ids["customer"]), map[string]string{"name": "Base Customer", "mobile": "9000000000", "email": "a@b"}, http.StatusBadRequest, "valid email")
	e.expect(a, "POST", "/api/admin/users", map[string]string{"username": "qa-x", "full_name": "QA X", "role": "employee", "password": password, "email": "nope"}, http.StatusBadRequest, "valid email")
	e.expect(a, "POST", "/api/admin/users", map[string]string{"username": "qa-y", "full_name": "  ", "role": "employee", "password": password}, http.StatusBadRequest, "name is required")
	e.expect(a, "POST", "/api/admin/users", map[string]string{"username": "qa-z", "full_name": long, "role": "employee", "password": password}, http.StatusBadRequest, "150 characters")

	var na int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT id FROM projects WHERE is_system`), &na)
	lead := map[string]any{"name": "QA Lead", "mobile": "9822222222", "email": "nope", "project_id": na, "assignee_id": e.ids["employee"]}
	e.expect(s, "POST", "/api/leads", lead, http.StatusBadRequest, "valid email")
	lead["email"], lead["name"] = "", long
	e.expect(s, "POST", "/api/leads", lead, http.StatusBadRequest, "150 characters")

	e.expect(s, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": long, "phone": "9833333333"}, http.StatusBadRequest, "")
	e.expect(s, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "QA Off", "email": "a@b"}, http.StatusBadRequest, "valid email")

	project := projectBody("QA-Proj "+fresh(), "9844444444")
	project["contacts"].([]map[string]any)[0]["email"] = "nope"
	e.expect(a, "POST", "/api/admin/projects", project, http.StatusBadRequest, "valid email")
	project = projectBody(long, "9844444445")
	e.expect(a, "POST", "/api/admin/projects", project, http.StatusBadRequest, "150 characters")
	project = projectBody("QA-Proj "+fresh(), "9844444446")
	project["address"] = strings.Repeat("a", 2001)
	e.expect(a, "POST", "/api/admin/projects", project, http.StatusBadRequest, "2000 characters")
}

func TestHugeLoanAmountNot500(t *testing.T) {
	e := newEnv(t)
	var na int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT id FROM projects WHERE is_system`), &na)
	body := map[string]any{"customer_id": freshCustomer(t, e), "bank_id": e.ids["bank"], "project_id": na,
		"loan_amount": "999999999999999999", "assignee_id": e.ids["employee"]}
	e.expect(access.RoleEmployee, "POST", "/api/cases", body, http.StatusBadRequest, "")
	e.expect(access.RoleEmployee, "PUT", fmt.Sprintf("/api/cases/%d", e.ids["case"]), map[string]any{"loan_amount": "100.555"}, http.StatusBadRequest, "")
}

func TestTextLimits(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("r", 2001)
	var other int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT id FROM dropdown_values WHERE category = 'drop_reason' AND value = 'Other'`), &other)
	drop := map[string]any{"reason_id": other, "remark": long}
	e.expect(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/drop", e.ids["case"]), drop, http.StatusBadRequest, "2000 characters")
	e.expect(access.RoleEmployee, "POST", fmt.Sprintf("/api/customers/%d/drop", e.ids["customer"]), drop, http.StatusBadRequest, "2000 characters")
	var na int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT id FROM projects WHERE is_system`), &na)
	var lead int64
	mustScan(t, e.pool.QueryRow(t.Context(), `INSERT INTO leads (customer_id, project_id, assignee_id) VALUES ($1, $2, $3) RETURNING id`,
		freshCustomer(t, e), na, e.ids["employee"]), &lead)
	e.expect(access.RoleEmployee, "POST", fmt.Sprintf("/api/leads/%d/contacts", lead), map[string]any{"response": "call_back", "remark": long}, http.StatusBadRequest, "2000 characters")
	e.expect(access.RoleEmployee, "POST", fmt.Sprintf("/api/leads/%d/drop", lead), drop, http.StatusBadRequest, "2000 characters")
	// Right at the limit is fine.
	e.expect(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/drop", freshCase(t, e)), map[string]any{"reason_id": other, "remark": strings.Repeat("r", 2000)}, http.StatusOK, "")
}

func uploadTo(t *testing.T, e *env, caseID int64) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("label", "QA doc")
	fw, _ := mw.CreateFormFile("file", "a.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4\n0000000000"))
	mw.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/cases/%d/documents", e.srv.URL, caseID), &buf)
	req.Header.Set("Origin", e.srv.URL)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.clients[access.RoleEmployee].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUploadMissingCase(t *testing.T) {
	e := newEnv(t)
	resp := uploadTo(t, e, 999999)
	if body := readAll(t, resp); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("upload to a missing case: %d %s, want 404", resp.StatusCode, body)
	}
}

func TestSendProofNotMultipart(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/queries/1/send", strings.NewReader("note=sent"))
	req.Header.Set("Origin", e.srv.URL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.clients[access.RoleEmployee].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if body := readAll(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "file upload") {
		t.Fatalf("form-encoded send: %d %s, want 400 asking for a file upload", resp.StatusCode, body)
	}
}

func TestUploadOver10MB(t *testing.T) {
	e := newEnv(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("label", "QA big")
	fw, _ := mw.CreateFormFile("file", "big.pdf")
	_, _ = fw.Write(append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), 11<<20)...))
	mw.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/cases/%d/documents", e.srv.URL, e.ids["case"]), &buf)
	req.Header.Set("Origin", e.srv.URL)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.clients[access.RoleEmployee].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if body := readAll(t, resp); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("11 MB upload: %d %s, want 413", resp.StatusCode, body)
	}
}
