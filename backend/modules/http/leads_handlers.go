package apihttp

import (
	"net/http"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/leads"
)

func dateOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func leadJSON(l leads.Lead) map[string]any {
	out := map[string]any{
		"id": l.ID, "customer_id": l.CustomerID, "project_id": l.ProjectID, "assignee_id": l.AssigneeID,
		"status": l.Status, "next_call_date": dateOrNil(l.NextCallDate), "created_at": l.CreatedAt,
		"converted_at": timeOrNil(l.ConvertedAt), "case_id": nil,
	}
	if l.CaseID != nil {
		out["case_id"] = *l.CaseID
	}
	out["builder_contact_id"] = nil
	if l.BuilderContactID != nil {
		out["builder_contact_id"] = *l.BuilderContactID
	}
	return out
}

func leadRowJSON(r leads.Row) map[string]any {
	out := leadJSON(r.Lead)
	out["customer_name"] = r.CustomerName
	out["customer_mobile"] = r.CustomerMobile
	out["customer_email"] = r.CustomerEmail
	out["project_name"] = r.ProjectName
	out["assignee_name"] = r.AssigneeName
	out["last_response"] = nil
	if r.LastResponse != "" {
		out["last_response"] = r.LastResponse
	}
	out["last_contact_at"] = timeOrNil(r.LastContactAt)
	out["call_due"] = r.CallDue
	out["overdue_days"] = r.OverdueDays
	out["builder_contact"] = nil
	if b := r.BuilderContact; b != nil {
		out["builder_contact"] = builderContactJSON(b.ID, b.Name, b.Phone, b.Email, b.Label, b.Removed)
	}
	return out
}

func leadRowsJSON(rows []leads.Row) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, leadRowJSON(r))
	}
	return out
}

// writeLead answers with the lead's full row, so screens refresh in one call.
func (h *handlers) writeLead(w http.ResponseWriter, r *http.Request, status int, id int64) {
	row, err := h.d.Leads.Get(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, leadRowJSON(row))
}

func (h *handlers) createLead(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		Mobile     string `json:"mobile"`
		Email      string `json:"email"`
		ProjectID  int64  `json:"project_id"`
		AssigneeID int64  `json:"assignee_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := h.d.Leads.Create(r.Context(), actor(r), leads.NewLead{
		Name: in.Name, Mobile: in.Mobile, Email: in.Email, ProjectID: in.ProjectID, AssigneeID: in.AssigneeID,
	}, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	if res.Existing != nil {
		writeJSON(w, http.StatusOK, map[string]any{"existing": customerJSON(*res.Existing)})
		return
	}
	row, err := h.d.Leads.Get(r.Context(), res.Lead.ID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"lead": leadRowJSON(row), "customer": customerJSON(*res.Customer)})
}

func (h *handlers) addCustomerLead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		ProjectID  int64 `json:"project_id"`
		AssigneeID int64 `json:"assignee_id"`
		BringBack  bool  `json:"bring_back"`
	}
	if !decode(w, r, &in) {
		return
	}
	l, err := h.d.Leads.AddForCustomer(r.Context(), actor(r), id, in.ProjectID, in.AssigneeID, in.BringBack, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusCreated, l.ID)
}

func (h *handlers) listLeads(w http.ResponseWriter, r *http.Request) {
	rows, err := h.d.Leads.ListOpen(r.Context(), time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, leadRowsJSON(rows))
}

func (h *handlers) getLead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	row, err := h.d.Leads.Get(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	contacts, err := h.d.Leads.Contacts(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	cs := make([]map[string]any, 0, len(contacts))
	for _, c := range contacts {
		cs = append(cs, map[string]any{
			"id": c.ID, "response": c.Response, "remark": c.Remark, "next_call_date": dateOrNil(c.NextCallDate),
			"created_at": c.CreatedAt, "created_by": c.CreatedBy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lead": leadRowJSON(row), "contacts": cs})
}

func (h *handlers) logLeadContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		Response     string  `json:"response"`
		Remark       string  `json:"remark"`
		NextCallDate *string `json:"next_call_date"`
	}
	if !decode(w, r, &in) {
		return
	}
	var next *time.Time
	if in.NextCallDate != nil && *in.NextCallDate != "" {
		d, err := time.Parse("2006-01-02", *in.NextCallDate)
		if err != nil {
			badInput(w, "next call date must be YYYY-MM-DD")
			return
		}
		next = &d
	}
	if _, err := h.d.Leads.LogContact(r.Context(), actor(r), id, leads.ContactIn{Response: in.Response, Remark: in.Remark, NextCallDate: next}); err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusCreated, id)
}

func (h *handlers) assignLead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		AssigneeID int64 `json:"assignee_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, err := h.d.Leads.Assign(r.Context(), actor(r), id, in.AssigneeID); err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusOK, id)
}

// setLeadBuilderContact points an open lead at another contact of its project (all staff).
func (h *handlers) setLeadBuilderContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		ContactID int64 `json:"contact_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, err := h.d.Leads.SetBuilderContact(r.Context(), actor(r), id, in.ContactID); err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusOK, id)
}

func (h *handlers) dropLead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in dropIn
	if !decode(w, r, &in) {
		return
	}
	if _, err := h.d.Leads.Drop(r.Context(), actor(r), id, in.ReasonID, in.Remark); err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusOK, id)
}

func (h *handlers) bringBackLead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if _, err := h.d.Leads.BringBack(r.Context(), actor(r), id, time.Now()); err != nil {
		writeError(w, err)
		return
	}
	h.writeLead(w, r, http.StatusOK, id)
}

func (h *handlers) leadToCase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		BankID         int64  `json:"bank_id"`
		LoanAmount     string `json:"loan_amount"`
		AssigneeID     int64  `json:"assignee_id"`
		UnitNumber     string `json:"unit_number"`
		StartAt        string `json:"start_at"`
		TrackingNumber string `json:"tracking_number"`
		BuilderContact int64  `json:"builder_contact_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.CreateFromLead(r.Context(), actor(r), id, cases.FromLead{
		BankID: in.BankID, LoanAmount: in.LoanAmount, AssigneeID: in.AssigneeID,
		UnitNumber: in.UnitNumber, StartAt: in.StartAt, TrackingNumber: in.TrackingNumber,
		BuilderContactID: in.BuilderContact,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, caseJSON(c))
}

func (h *handlers) setCaseUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		UnitNumber string `json:"unit_number"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.SetUnitNumber(r.Context(), actor(r), id, in.UnitNumber)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}
