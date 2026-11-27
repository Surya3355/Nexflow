package apihttp

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestOfficerRoutes(t *testing.T) {
	e := newEnv(t)
	emp := e.clients[access.RoleEmployee]

	// Add, list, edit, remove: any staff member.
	var o map[string]any
	resp := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "Sita", "phone": "9876543210"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add = %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &o)
	if o["team_name"] != "Officer" || o["bank_name"] != "HDFC" || o["phone"] != "9876543210" {
		t.Fatalf("officer = %v", o)
	}
	sita := o["id"].(float64)
	dup := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "S", "phone": "9876543210"})
	dup.Body.Close()
	if dup.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate phone = %d, want 409", dup.StatusCode)
	}
	wrong := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "dovr", "name": "S", "phone": "9876543211"})
	wrong.Body.Close()
	if wrong.StatusCode != http.StatusBadRequest {
		t.Fatalf("team not on bank = %d, want 400", wrong.StatusCode)
	}
	resp = e.do(emp, "PUT", fmt.Sprintf("/api/officers/%d", int64(sita)), map[string]any{"name": "Sita R", "phone": "9876543210"})
	decodeJSON(t, resp, &o)
	if resp.StatusCode != http.StatusOK || o["name"] != "Sita R" {
		t.Fatalf("edit = %d %v", resp.StatusCode, o)
	}
	var list []map[string]any
	decodeJSON(t, e.do(emp, "GET", "/api/officers", nil), &list)
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}

	// Stage JSON carries the team.
	var stages []map[string]any
	decodeJSON(t, e.do(emp, "GET", fmt.Sprintf("/api/config/banks/%d/stages", e.ids["bank"]), nil), &stages)
	if stages[0]["officer_team"] != "officer" || stages[0]["officer_team_name"] != "Officer" || stages[1]["officer_team"] != "" {
		t.Fatalf("stages = %v", stages[:2])
	}

	// Pick on the case, then read it back with history.
	casePath := fmt.Sprintf("/api/cases/%d/officers", e.ids["case"])
	resp = e.do(emp, "POST", casePath, map[string]any{"team": "officer", "officer_id": sita})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pick = %d", resp.StatusCode)
	}
	var got struct {
		Officers []map[string]any `json:"officers"`
		History  []map[string]any `json:"history"`
	}
	decodeJSON(t, e.do(emp, "GET", casePath, nil), &got)
	if len(got.Officers) != 1 || got.Officers[0]["team_name"] != "Officer" || got.Officers[0]["officer"].(map[string]any)["name"] != "Sita R" {
		t.Fatalf("case officers = %v", got.Officers)
	}
	if len(got.History) != 1 || got.History[0]["officer_name"] != "Sita R" || got.History[0]["team_name"] != "Officer" || got.History[0]["at"] == nil {
		t.Fatalf("history = %v", got.History)
	}

	// A stage move with an officer of a team the stage does not have is refused.
	bad := e.do(emp, "POST", fmt.Sprintf("/api/cases/%d/stage", e.ids["case"]), map[string]any{
		"stage_id": e.ids["sanctioned"], "officer_id": sita, "sanction": map[string]any{"amount": "100000"}})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("officer at stage without team = %d, want 400", bad.StatusCode)
	}

	// Add case with an officer.
	resp = e.do(emp, "POST", "/api/cases", map[string]any{"customer_id": e.ids["customer"], "bank_id": e.ids["bank"],
		"project_id": naID(t, e), "loan_amount": "100000", "assignee_id": e.ids["employee"], "officers": map[string]any{"officer": sita}})
	var c map[string]any
	decodeJSON(t, resp, &c)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with officer = %d %v", resp.StatusCode, c)
	}
	decodeJSON(t, e.do(emp, "GET", fmt.Sprintf("/api/cases/%d/officers", int64(c["id"].(float64))), nil), &got)
	if got.Officers[0]["officer"] == nil {
		t.Fatalf("new case officers = %v", got.Officers)
	}

	rm := e.do(emp, "DELETE", fmt.Sprintf("/api/officers/%d", int64(sita)), nil)
	rm.Body.Close()
	if rm.StatusCode != http.StatusNoContent {
		t.Fatalf("remove = %d", rm.StatusCode)
	}
	again := e.do(emp, "DELETE", fmt.Sprintf("/api/officers/%d", int64(sita)), nil)
	again.Body.Close()
	if again.StatusCode != http.StatusNotFound {
		t.Fatalf("remove twice = %d, want 404", again.StatusCode)
	}
	decodeJSON(t, e.do(emp, "GET", casePath, nil), &got)
	if got.Officers[0]["officer"].(map[string]any)["removed"] != true {
		t.Fatalf("removed officer on case = %v", got.Officers)
	}
}

func TestOfficerEmailOnlyRoutes(t *testing.T) {
	e := newEnv(t)
	emp := e.clients[access.RoleEmployee]
	var o map[string]any
	resp := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "Mail", "email": "mail@sbi.example"})
	decodeJSON(t, resp, &o)
	if resp.StatusCode != http.StatusCreated || o["email"] != "mail@sbi.example" || o["phone"] != "" {
		t.Fatalf("add email only = %d %v", resp.StatusCode, o)
	}
	none := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "None"})
	var msg map[string]string
	decodeJSON(t, none, &msg)
	if none.StatusCode != http.StatusBadRequest || msg["error"] != "officers: enter a mobile number or an email" {
		t.Fatalf("neither = %d %v", none.StatusCode, msg)
	}
	dup := e.do(emp, "POST", "/api/officers", map[string]any{"bank_id": e.ids["bank"], "team": "officer", "name": "Dup", "email": "MAIL@sbi.example"})
	dup.Body.Close()
	if dup.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate email = %d, want 409", dup.StatusCode)
	}
	id := o["id"].(float64)
	resp = e.do(emp, "PUT", fmt.Sprintf("/api/officers/%d", int64(id)), map[string]any{"name": "Mail", "phone": "9876543210", "email": "mail@sbi.example"})
	decodeJSON(t, resp, &o)
	if resp.StatusCode != http.StatusOK || o["phone"] != "9876543210" {
		t.Fatalf("edit adds mobile = %d %v", resp.StatusCode, o)
	}
	pick := e.do(emp, "POST", fmt.Sprintf("/api/cases/%d/officers", e.ids["case"]), map[string]any{"team": "officer", "officer_id": id})
	pick.Body.Close()
	var got struct {
		Officers []map[string]any `json:"officers"`
	}
	decodeJSON(t, e.do(emp, "GET", fmt.Sprintf("/api/cases/%d/officers", e.ids["case"]), nil), &got)
	if got.Officers[0]["officer"].(map[string]any)["email"] != "mail@sbi.example" {
		t.Fatalf("case officer = %v", got.Officers)
	}
}
