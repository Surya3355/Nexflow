package apihttp

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/reports"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/go-chi/chi/v5"
)

type handlers struct{ d Deps }

// actor returns the signed-in user. RequireUser guarantees it is present.
func actor(r *http.Request) access.User {
	u, _ := access.UserFromContext(r.Context())
	return u
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func badID(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
}

func badInput(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

// ---- configuration --------------------------------------------------------

func (h *handlers) listBanks(w http.ResponseWriter, r *http.Request) {
	banks, err := h.d.Workflows.ListBanks(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(banks))
	for _, b := range banks {
		out = append(out, map[string]any{"id": b.ID, "name": b.Name, "has_own_stages": b.HasOwnStages,
			"part_payments": b.PartPayments, "stage_list": b.StageList})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) stagesForBank(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	stages, err := h.d.Workflows.StagesFor(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stageList(stages))
}

func stageList(stages []workflows.Stage) []map[string]any {
	out := make([]map[string]any, 0, len(stages))
	for _, s := range stages {
		out = append(out, map[string]any{
			"id": s.ID, "code": s.Code, "name": s.Name, "sort_order": s.SortOrder,
			"requires_document": s.RequiresDocument, "role": s.Role,
		})
	}
	return out
}

func (h *handlers) dropdown(w http.ResponseWriter, r *http.Request) {
	vals, err := h.d.Workflows.Dropdown(r.Context(), chi.URLParam(r, "category"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(vals))
	for _, v := range vals {
		out = append(out, map[string]any{"id": v.ID, "value": v.Value})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- admin ----------------------------------------------------------------

func (h *handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := h.d.Access.CreateUser(r.Context(), actor(r), access.NewUser{
		Username: in.Username, FullName: in.FullName, Role: access.Role(in.Role), Password: in.Password, Email: in.Email,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": u.ID, "username": u.Username, "role": string(u.Role)})
}

// listTeam is the admin Team tab: every user with role and open case count.
func (h *handlers) listTeam(w http.ResponseWriter, r *http.Request) {
	team, err := h.d.Access.ListTeam(r.Context(), actor(r))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(team))
	for _, m := range team {
		out = append(out, map[string]any{
			"id": m.ID, "username": m.Username, "full_name": m.FullName, "role": string(m.Role),
			"email": m.Email, "is_active": m.IsActive, "open_cases": m.OpenCases,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// resetPassword lets an admin set a new password for a user.
func (h *handlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.d.Access.ResetPassword(r.Context(), actor(r), id, in.Password); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) createBank(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	b, err := h.d.Workflows.CreateBank(r.Context(), actor(r), strings.TrimSpace(in.Name))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": b.ID, "name": b.Name})
}

// renameBank changes a bank's name (admin only).
// updateBankSettings changes a bank's stage list and/or part payments. Admin only.
func (h *handlers) updateBankSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		StageList    *string `json:"stage_list"`
		PartPayments *bool   `json:"part_payments"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.StageList != nil {
		if err := h.d.Workflows.SetBankStageList(r.Context(), actor(r), id, *in.StageList); err != nil {
			writeError(w, err)
			return
		}
	}
	if in.PartPayments != nil {
		if err := h.d.Workflows.SetBankPartPayments(r.Context(), actor(r), id, *in.PartPayments); err != nil {
			writeError(w, err)
			return
		}
	}
	if in.StageList == nil && in.PartPayments == nil && !access.Can(actor(r).Role, access.ActionManageConfig) {
		writeError(w, access.ErrForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stageLists returns every stage list for the admin Stages screen. Admin only.
func (h *handlers) stageLists(w http.ResponseWriter, r *http.Request) {
	if !access.Can(actor(r).Role, access.ActionManageConfig) {
		writeError(w, access.ErrForbidden)
		return
	}
	lists, err := h.d.Workflows.StageLists(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(lists))
	for _, l := range lists {
		var bank any
		if l.BankID != nil {
			bank = *l.BankID
		}
		out = append(out, map[string]any{"bank_id": bank, "name": l.Name, "stages": stageList(l.Stages)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) renameBank(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	b, err := h.d.Workflows.RenameBank(r.Context(), actor(r), id, in.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": b.ID, "name": b.Name})
}

func (h *handlers) addStage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BankID           *int64 `json:"bank_id"`
		Code             string `json:"code"`
		Name             string `json:"name"`
		SortOrder        int    `json:"sort_order"`
		RequiresDocument bool   `json:"requires_document"`
	}
	if !decode(w, r, &in) {
		return
	}
	st, err := h.d.Workflows.AddStage(r.Context(), actor(r), in.BankID, in.Code, in.Name, in.SortOrder, in.RequiresDocument)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, stageList([]workflows.Stage{st})[0])
}

func (h *handlers) setStageRequiresDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		RequiresDocument bool `json:"requires_document"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.d.Workflows.SetStageRequiresDocument(r.Context(), actor(r), id, in.RequiresDocument); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) addDropdownValue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Category  string `json:"category"`
		Value     string `json:"value"`
		SortOrder int    `json:"sort_order"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.d.Workflows.AddDropdownValue(r.Context(), actor(r), in.Category, in.Value, in.SortOrder)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": v.ID, "category": v.Category, "value": v.Value})
}

func (h *handlers) listArchive(w http.ResponseWriter, r *http.Request) {
	if !access.Can(actor(r).Role, access.ActionRestoreArchive) {
		writeError(w, access.ErrForbidden)
		return
	}
	items, err := h.d.Archive.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id": it.ID, "entity_type": it.EntityType, "entity_id": it.EntityID,
			"archived_at": it.ArchivedAt, "purge_after": it.PurgeAfter, "restored": it.Restored,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) restoreArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		ReasonID int64  `json:"reason_id"`
		Remark   string `json:"remark"`
	}
	if !decode(w, r, &in) {
		return
	}
	ref, err := h.d.Archive.Restore(r.Context(), actor(r), id, archive.DropInput{ReasonID: in.ReasonID, Remark: in.Remark})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entity_type": ref.EntityType, "entity_id": ref.EntityID})
}

// ---- customers ------------------------------------------------------------

type customerIn struct {
	Name   string `json:"name"`
	Mobile string `json:"mobile"`
	Email  string `json:"email"`
}

func customerJSON(c customers.Customer) map[string]any {
	return map[string]any{"id": c.ID, "name": c.Name, "mobile": c.Mobile, "email": c.Email, "status": c.Status}
}

func (h *handlers) createCustomer(w http.ResponseWriter, r *http.Request) {
	var in customerIn
	if !decode(w, r, &in) {
		return
	}
	res, err := h.d.Customers.Create(r.Context(), actor(r), customers.NewCustomer{Name: in.Name, Mobile: in.Mobile, Email: in.Email})
	if err != nil {
		writeError(w, err)
		return
	}
	if res.Existing != nil {
		// Warning, not an error: the mobile belongs to this customer already.
		writeJSON(w, http.StatusOK, map[string]any{"existing": customerJSON(*res.Existing)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"customer": customerJSON(*res.Customer)})
}

func (h *handlers) updateCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in customerIn
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Customers.Update(r.Context(), actor(r), id, customers.CustomerFields{Name: in.Name, Mobile: in.Mobile, Email: in.Email})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customerJSON(c))
}

type dropIn struct {
	ReasonID int64  `json:"reason_id"`
	Remark   string `json:"remark"`
}

// reactivateCustomer brings a dropped customer back to active (all staff).
func (h *handlers) reactivateCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	c, err := h.d.Customers.Reactivate(r.Context(), actor(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customerJSON(c))
}

func (h *handlers) dropCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in dropIn
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Customers.Drop(r.Context(), actor(r), id, customers.DropInput{ReasonID: in.ReasonID, Remark: in.Remark})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customerJSON(c))
}

func (h *handlers) deleteCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if err := h.d.Archive.DeleteCustomer(r.Context(), actor(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- cases ----------------------------------------------------------------

func caseJSON(c cases.Case) map[string]any {
	out := map[string]any{
		"id": c.ID, "customer_id": c.CustomerID, "bank_id": c.BankID, "project_id": c.ProjectID, "loan_amount": c.LoanAmount,
		"current_stage_id": c.CurrentStageID, "assignee_id": c.AssigneeID, "status": c.Status,
		"created_at": c.CreatedAt,
	}
	if c.TrackingNumber != nil {
		out["tracking_number"] = *c.TrackingNumber
	}
	if c.FollowUpDate != nil {
		out["follow_up_date"] = c.FollowUpDate.Format("2006-01-02")
	}
	out["lead_id"], out["unit_number"] = nil, nil
	if c.LeadID != nil {
		out["lead_id"] = *c.LeadID
	}
	if c.UnitNumber != nil {
		out["unit_number"] = *c.UnitNumber
	}
	out["project_type"] = nil
	if c.ProjectType != nil {
		out["project_type"] = *c.ProjectType
	}
	out["builder_contact_id"], out["builder_contact"] = nil, nil
	if c.BuilderContactID != nil {
		out["builder_contact_id"] = *c.BuilderContactID
	}
	if b := c.BuilderContact; b != nil {
		out["builder_contact"] = builderContactJSON(b.ID, b.Name, b.Phone, b.Email, b.Label, b.Removed)
	}
	return out
}

// builderContactJSON is the contact a lead or case points to (spec 2026-11-24 §4.3).
func builderContactJSON(id int64, name, phone, email, label string, removed bool) map[string]any {
	return map[string]any{"id": id, "name": name, "phone": phone, "email": email, "label": label, "removed": removed}
}

// setCaseBuilderContact points a case at another contact of its project (all staff).
func (h *handlers) setCaseBuilderContact(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.d.Cases.SetBuilderContact(r.Context(), actor(r), id, in.ContactID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}

func (h *handlers) createCase(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CustomerID     int64  `json:"customer_id"`
		BankID         int64  `json:"bank_id"`
		ProjectID      int64  `json:"project_id"`
		LoanAmount     string `json:"loan_amount"`
		AssigneeID     int64  `json:"assignee_id"`
		BringBack      bool   `json:"bring_back"`
		UnitNumber     string `json:"unit_number"`
		StartAt        string `json:"start_at"`
		TrackingNumber string `json:"tracking_number"`
		BuilderContact int64  `json:"builder_contact_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.Create(r.Context(), actor(r), cases.NewCase{
		CustomerID: in.CustomerID, BankID: in.BankID, ProjectID: in.ProjectID, LoanAmount: in.LoanAmount, AssigneeID: in.AssigneeID,
		BringBack: in.BringBack, UnitNumber: in.UnitNumber, StartAt: in.StartAt, TrackingNumber: in.TrackingNumber,
		BuilderContactID: in.BuilderContact,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, caseJSON(c))
}

func (h *handlers) listCases(w http.ResponseWriter, r *http.Request) {
	f, err := caseFilter(r)
	if err != nil {
		badInput(w, "invalid filter")
		return
	}
	list, err := h.d.Cases.ListRows(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, caseRowJSON(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// caseAttention lists cases that need attention (UI redesign spec §6).
// Optional assignee_id narrows it to one person.
func (h *handlers) caseAttention(w http.ResponseWriter, r *http.Request) {
	var assignee *int64
	if v := r.URL.Query().Get("assignee_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badInput(w, "invalid assignee")
			return
		}
		assignee = &n
	}
	items, err := h.d.Cases.Attention(r.Context(), assignee, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := caseRowJSON(it.CaseRow)
		row["reason"] = it.Reason
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

// caseRowJSON is caseJSON plus the names and stage progress of the case list.
func caseRowJSON(c cases.CaseRow) map[string]any {
	out := caseJSON(c.Case)
	out["customer_name"] = c.CustomerName
	out["customer_mobile"] = c.CustomerMobile
	out["bank_name"] = c.BankName
	out["project_name"] = c.ProjectName
	out["stage_name"] = c.StageName
	out["stage_code"] = c.StageCode
	out["stage_order"] = c.StageOrder
	out["stage_count"] = c.StageCount
	out["assignee_name"] = c.AssigneeName
	out["stage_since"] = c.StageSince
	out["needs_document"] = c.NeedsDocument
	return out
}

// caseFilter reads the list filters from the query string.
func caseFilter(r *http.Request) (cases.ListFilter, error) {
	var f cases.ListFilter
	q := r.URL.Query()
	parseID := func(name string) (*int64, error) {
		v := q.Get(name)
		if v == "" {
			return nil, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		return &n, nil
	}
	var err error
	if f.StageID, err = parseID("stage_id"); err != nil {
		return f, err
	}
	if f.BankID, err = parseID("bank_id"); err != nil {
		return f, err
	}
	if f.ProjectID, err = parseID("project_id"); err != nil {
		return f, err
	}
	if f.AssigneeID, err = parseID("assignee_id"); err != nil {
		return f, err
	}
	if f.From, err = parseDay(q.Get("from")); err != nil {
		return f, err
	}
	if f.To, err = parseDay(q.Get("to")); err != nil {
		return f, err
	}
	if f.To != nil {
		next := f.To.AddDate(0, 0, 1)
		f.To = &next
	}
	return f, nil
}

func (h *handlers) getCase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	c, err := h.d.Cases.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeCaseWithPayments(w, r, c)
}

// writeCaseWithPayments sends the case with its payments and totals (spec 2026-11-25 §5.4).
func (h *handlers) writeCaseWithPayments(w http.ResponseWriter, r *http.Request, c cases.Case) {
	sum, err := h.d.Cases.Payments(r.Context(), c.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := caseJSON(c)
	pays := make([]map[string]any, 0, len(sum.Payments))
	for _, p := range sum.Payments {
		pays = append(pays, map[string]any{"id": p.ID, "amount": p.Amount, "paid_on": p.PaidOn.Format("2006-01-02"), "kind": p.Kind})
	}
	out["part_payments"] = c.PartPayments
	out["payments"], out["total_paid"], out["remaining"] = pays, sum.TotalPaid, sum.Remaining
	out["fully_paid"], out["over_paid"] = sum.FullyPaid, sum.OverPaid
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) addPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in paymentIn
	if !decode(w, r, &in) {
		return
	}
	p, ok := toPayment(w, in)
	if !ok {
		return
	}
	c, err := h.d.Cases.AddPayment(r.Context(), actor(r), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeCaseWithPayments(w, r, c)
}

func (h *handlers) updatePayment(w http.ResponseWriter, r *http.Request) {
	id, ok1 := pathID(r, "id")
	pid, ok2 := pathID(r, "paymentID")
	if !ok1 || !ok2 {
		badID(w)
		return
	}
	var in paymentIn
	if !decode(w, r, &in) {
		return
	}
	p, ok := toPayment(w, in)
	if !ok {
		return
	}
	c, err := h.d.Cases.UpdatePayment(r.Context(), actor(r), id, pid, p)
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeCaseWithPayments(w, r, c)
}

func (h *handlers) deletePayment(w http.ResponseWriter, r *http.Request) {
	id, ok1 := pathID(r, "id")
	pid, ok2 := pathID(r, "paymentID")
	if !ok1 || !ok2 {
		badID(w)
		return
	}
	c, err := h.d.Cases.DeletePayment(r.Context(), actor(r), id, pid)
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeCaseWithPayments(w, r, c)
}

func (h *handlers) updateCase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		LoanAmount   string `json:"loan_amount"`
		FollowUpDate string `json:"follow_up_date"`
	}
	if !decode(w, r, &in) {
		return
	}
	follow, err := parseDay(in.FollowUpDate)
	if err != nil {
		badInput(w, "invalid follow-up date")
		return
	}
	c, err := h.d.Cases.Update(r.Context(), actor(r), id, cases.CaseFields{LoanAmount: in.LoanAmount, FollowUpDate: follow})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}

type sanctionIn struct {
	Amount       string `json:"amount"`
	RatePercent  string `json:"rate_percent"`
	InterestType string `json:"interest_type"`
	TenureMonths int    `json:"tenure_months"`
	Date         string `json:"date"`
}

type paymentIn struct {
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Kind   string `json:"kind"`
}

// toPayment parses a payment body; ok is false after a 400 was written.
func toPayment(w http.ResponseWriter, in paymentIn) (cases.PaymentInput, bool) {
	d, err := parseDay(in.Date)
	if err != nil {
		badInput(w, "invalid payment date")
		return cases.PaymentInput{}, false
	}
	p := cases.PaymentInput{Amount: in.Amount, Kind: in.Kind}
	if d != nil {
		p.Date = *d
	}
	return p, true
}

func (h *handlers) changeStage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		StageID        int64           `json:"stage_id"`
		TrackingNumber *string         `json:"tracking_number"`
		Sanction       *sanctionIn     `json:"sanction"`
		Payment        *paymentIn      `json:"payment"`
		Remark         string          `json:"remark"`
	}
	if !decode(w, r, &in) {
		return
	}
	var sanction *cases.SanctionDetails
	if in.Sanction != nil {
		d, err := parseDay(in.Sanction.Date)
		if err != nil {
			badInput(w, "invalid sanction date")
			return
		}
		sd := cases.SanctionDetails{Amount: in.Sanction.Amount, RatePercent: in.Sanction.RatePercent,
			InterestType: in.Sanction.InterestType, TenureMonths: in.Sanction.TenureMonths}
		if d != nil {
			sd.Date = *d
		}
		sanction = &sd
	}
	m := cases.StageMove{ToStageID: in.StageID, TrackingNumber: in.TrackingNumber, Sanction: sanction, Remark: in.Remark}
	if in.Payment != nil {
		p, ok := toPayment(w, *in.Payment)
		if !ok {
			return
		}
		m.Payment = &p
	}
	c, err := h.d.Cases.ChangeStage(r.Context(), actor(r), id, m)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := caseJSON(c)
	// A missing document is a warning only (round 12); the move already happened.
	if st, err := h.d.Documents.RequirementStatus(r.Context(), id); err == nil && st.Warning != "" {
		resp["warning"] = st.Warning
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) assignCase(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.d.Cases.Assign(r.Context(), actor(r), id, in.AssigneeID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}

// changeCaseProject moves a case to another project (all staff).
func (h *handlers) changeCaseProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		ProjectID int64 `json:"project_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.ChangeProject(r.Context(), actor(r), id, in.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}

func (h *handlers) dropCase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in dropIn
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.Drop(r.Context(), actor(r), id, cases.DropInput{ReasonID: in.ReasonID, Remark: in.Remark})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, caseJSON(c))
}

func (h *handlers) deleteCase(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if err := h.d.Archive.DeleteCase(r.Context(), actor(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) search(w http.ResponseWriter, r *http.Request) {
	groups, err := h.d.Cases.Search(r.Context(), cases.SearchQuery{Text: r.URL.Query().Get("q")})
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		cs := make([]map[string]any, 0, len(g.Cases))
		for _, c := range g.Cases {
			cs = append(cs, caseJSON(c))
		}
		all, err := h.d.Leads.ForCustomer(r.Context(), g.Customer.ID, time.Now())
		if err != nil {
			writeError(w, err)
			return
		}
		open := make([]leads.Row, 0, len(all))
		for _, l := range all {
			if l.Status == leads.StatusOpen {
				open = append(open, l)
			}
		}
		out = append(out, map[string]any{
			"customer": map[string]any{"id": g.Customer.ID, "name": g.Customer.Name, "mobile": g.Customer.Mobile, "status": g.Customer.Status},
			"cases":    cs,
			"leads":    leadRowsJSON(open),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- documents ------------------------------------------------------------

func documentJSON(d documents.Document) map[string]any {
	return map[string]any{
		"id": d.ID, "label": d.Label, "file_name": d.FileName, "content_type": d.ContentType,
		"size_bytes": d.SizeBytes, "stage_id": d.StageID, "uploaded_at": d.UploadedAt,
	}
}

func (h *handlers) listDocuments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	docs, err := h.d.Documents.ListForCase(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		out = append(out, documentJSON(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) uploadDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, documents.MaxFileSize+64*1024)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "the file is larger than 10 MB"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		badInput(w, "choose a file")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, documents.MaxFileSize+1))
	if err != nil {
		badInput(w, "could not read the file")
		return
	}
	doc, err := h.d.Documents.Upload(r.Context(), actor(r), id, r.FormValue("label"),
		documents.UploadedFile{Name: header.Filename, Content: content})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, documentJSON(doc))
}

func (h *handlers) documentLink(w http.ResponseWriter, r *http.Request) {
	caseID, ok := pathID(r, "id")
	docID, ok2 := pathID(r, "docID")
	if !ok || !ok2 {
		badID(w)
		return
	}
	docs, err := h.d.Documents.ListForCase(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	found := false
	for _, d := range docs {
		if d.ID == docID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, documents.ErrNotFoundDoc)
		return
	}
	token := h.d.Linker.Sign(docID, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"url": "/api/documents/" + token, "expires_in_seconds": int(documents.LinkTTL.Seconds())})
}

// ---- reports --------------------------------------------------------------

// reportPeriod parses from, to, bank_id and project_id; ok is false after a 400.
func reportPeriod(w http.ResponseWriter, r *http.Request, now time.Time) (reports.Period, bool) {
	from, err1 := parseDay(r.URL.Query().Get("from"))
	to, err2 := parseDay(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		badInput(w, "invalid period")
		return reports.Period{}, false
	}
	p := reports.Period{From: now.AddDate(0, -1, 0), To: now.AddDate(0, 0, 1)}
	if from != nil {
		p.From = *from
	}
	if to != nil {
		p.To = to.AddDate(0, 0, 1)
	}
	if v := r.URL.Query().Get("bank_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badInput(w, "invalid bank")
			return reports.Period{}, false
		}
		p.BankID = &n
	}
	if v := r.URL.Query().Get("project_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badInput(w, "invalid project")
			return reports.Period{}, false
		}
		p.ProjectID = &n
	}
	return p, true
}

func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	p, ok := reportPeriod(w, r, now)
	if !ok {
		return
	}
	d, err := h.d.Reports.Dashboard(r.Context(), now, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"new_customers_today": d.NewCustomersToday,
		"new_cases_today":     d.NewCasesToday,
		"new_customers":       d.NewCustomers,
		"new_cases":           d.NewCases,
		"stage_moves_today":   d.StageMovesToday,
		"cases_by_stage":      stageTotalsJSON(d.CasesByStage),
		"sanctions":           stageCountJSON(d.Sanctions),
		"disbursements":       stageCountJSON(d.Disbursements),
	})
}

func (h *handlers) disbursementRowsFor(w http.ResponseWriter, r *http.Request) ([]reports.DisbursementRow, bool) {
	p, ok := reportPeriod(w, r, time.Now())
	if !ok {
		return nil, false
	}
	rows, err := h.d.Reports.DisbursementRows(r.Context(), p)
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	return rows, true
}

func (h *handlers) disbursementRows(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.disbursementRowsFor(w, r)
	if !ok {
		return
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Fields())
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": reports.DisbursementHeader, "rows": out})
}

func (h *handlers) exportDisbursementsCSV(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.disbursementRowsFor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nexflow-disbursements.csv"`)
	_ = reports.ExportDisbursementsCSV(w, rows)
}

func stageTotalsJSON(ts []reports.StageTotal) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{"bank_id": t.BankID, "bank_name": t.BankName, "stage_id": t.StageID,
			"stage_code": t.StageCode, "stage_name": t.StageName, "sort_order": t.SortOrder, "total": t.Total})
	}
	return out
}

func stageCountJSON(s reports.StageCount) map[string]any {
	return map[string]any{"count": s.Count, "amount": s.Amount, "dropped": s.Dropped}
}

// reportRowsFor parses the filter and loads the rows. A bad filter is reported
// as a 400 before any database work.
func (h *handlers) reportRowsFor(w http.ResponseWriter, r *http.Request) ([]reports.Row, bool) {
	f, err := caseFilter(r)
	if err != nil {
		badInput(w, "invalid filter")
		return nil, false
	}
	rows, err := h.d.Reports.Rows(r.Context(), actor(r), reports.RowFilter{
		StageID: f.StageID, BankID: f.BankID, ProjectID: f.ProjectID, AssigneeID: f.AssigneeID, From: f.From, To: f.To,
	})
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	return rows, true
}

func (h *handlers) reportRows(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.reportRowsFor(w, r)
	if !ok {
		return
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Fields())
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": reports.Header, "rows": out})
}

func (h *handlers) exportCSV(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.reportRowsFor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nexflow-report.csv"`)
	_ = reports.ExportCSV(w, rows)
}

func (h *handlers) caseHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	events, err := h.d.Cases.History(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		out = append(out, map[string]any{
			"id": e.ID, "reached_at": e.ReachedAt, "stage_code": e.StageCode,
			"stage_name": e.StageName, "changed_by": e.ChangedBy, "remark": e.Remark,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) getCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	c, err := h.d.Customers.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	list, err := h.d.Cases.ListRows(r.Context(), cases.ListFilter{CustomerID: &id})
	if err != nil {
		writeError(w, err)
		return
	}
	cs := make([]map[string]any, 0, len(list))
	for _, x := range list {
		cs = append(cs, caseRowJSON(x))
	}
	ls, err := h.d.Leads.ForCustomer(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	resp := customerJSON(c)
	resp["cases"] = cs
	resp["leads"] = leadRowsJSON(ls)
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.d.Access.ListActiveUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{"id": u.ID, "full_name": u.FullName, "username": u.Username})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) standardStages(w http.ResponseWriter, r *http.Request) {
	stages, err := h.d.Workflows.StandardStages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stageList(stages))
}

// projectJSON is a project as the API presents it.
func projectJSON(p workflows.Project) map[string]any {
	out := map[string]any{
		"id": p.ID, "name": p.Name, "is_system": p.IsSystem, "open_cases": p.OpenCases, "total_cases": p.TotalCases,
		"type": nil, "address": p.Address, "active_contacts": p.ActiveContacts,
	}
	if p.Type != nil {
		out["type"] = *p.Type
	}
	if p.Contacts != nil {
		cs := make([]map[string]any, 0, len(p.Contacts))
		for _, c := range p.Contacts {
			cs = append(cs, map[string]any{"id": c.ID, "name": c.Name, "phone": c.Phone, "email": c.Email, "label": c.Label})
		}
		out["contacts"] = cs
	}
	return out
}

// projectIn is the whole project form (spec 2026-11-24 §4.2).
type projectIn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Address  string `json:"address"`
	Contacts []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Phone string `json:"phone"`
		Email string `json:"email"`
		Label string `json:"label"`
	} `json:"contacts"`
}

func (in projectIn) input() workflows.ProjectInput {
	out := workflows.ProjectInput{Name: in.Name, Type: in.Type, Address: in.Address}
	for _, c := range in.Contacts {
		out.Contacts = append(out.Contacts, workflows.ContactInput{ID: c.ID, Name: c.Name, Phone: c.Phone, Email: c.Email, Label: c.Label})
	}
	return out
}

// listProjects returns every project, NA first (all signed-in staff).
func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.d.Workflows.ListProjects(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectJSON(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// getProject returns one project with its active contacts (all signed-in staff).
func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	p, err := h.d.Workflows.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectJSON(p))
}

// createProject adds a complete project with its contacts (admin only).
func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	var in projectIn
	if !decode(w, r, &in) {
		return
	}
	p, err := h.d.Workflows.CreateProject(r.Context(), actor(r), in.input())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectJSON(p))
}

// saveProject replaces a project's details and contacts (admin only; NA is locked).
func (h *handlers) saveProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in projectIn
	if !decode(w, r, &in) {
		return
	}
	p, err := h.d.Workflows.SaveProject(r.Context(), actor(r), id, in.input())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectJSON(p))
}
