package apihttp

import (
	"net/http"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/officers"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

// Bank officers (round 15 tasks 6 and 7, spec 2026-11-26).

func officerJSON(o officers.Officer) map[string]any {
	return map[string]any{"id": o.ID, "bank_id": o.BankID, "bank_name": o.BankName, "team": o.Team,
		"team_name": o.TeamName, "name": o.Name, "phone": o.Phone, "email": o.Email}
}

func (h *handlers) listOfficers(w http.ResponseWriter, r *http.Request) {
	list, err := h.d.Officers.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, o := range list {
		out = append(out, officerJSON(o))
	}
	writeJSON(w, http.StatusOK, out)
}

type officerIn struct {
	BankID int64  `json:"bank_id"`
	Team   string `json:"team"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Email  string `json:"email"`
}

func (h *handlers) addOfficer(w http.ResponseWriter, r *http.Request) {
	var in officerIn
	if !decode(w, r, &in) {
		return
	}
	o, err := h.d.Officers.Add(r.Context(), actor(r), officers.NewOfficer{BankID: in.BankID, Team: in.Team, Name: in.Name, Phone: in.Phone, Email: in.Email})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, officerJSON(o))
}

func (h *handlers) updateOfficer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in officerIn
	if !decode(w, r, &in) {
		return
	}
	o, err := h.d.Officers.Update(r.Context(), actor(r), id, in.Name, in.Phone, in.Email)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, officerJSON(o))
}

func (h *handlers) removeOfficer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	if err := h.d.Officers.Remove(r.Context(), actor(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) caseOfficers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	list, err := h.d.Cases.Officers(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	hist, err := h.d.Cases.OfficerHistory(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	teams := make([]map[string]any, 0, len(list))
	for _, co := range list {
		var o any
		if co.Officer != nil {
			o = map[string]any{"id": co.Officer.ID, "name": co.Officer.Name, "phone": co.Officer.Phone, "email": co.Officer.Email, "removed": co.Officer.Removed}
		}
		teams = append(teams, map[string]any{"team": co.Team, "team_name": co.TeamName, "officer": o})
	}
	events := make([]map[string]any, 0, len(hist))
	for _, e := range hist {
		events = append(events, map[string]any{"at": e.At.Format(time.RFC3339), "team": e.Team, "team_name": e.TeamName,
			"officer_name": e.OfficerName, "changed_by": e.ChangedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"officers": teams, "history": events})
}

func (h *handlers) setCaseOfficer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		badID(w)
		return
	}
	var in struct {
		Team      string `json:"team"`
		OfficerID int64  `json:"officer_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.d.Cases.SetOfficer(r.Context(), actor(r), id, in.Team, in.OfficerID)
	if err != nil {
		writeError(w, err)
		return
	}
	h.writeCaseWithPayments(w, r, c)
}

// teamName is the stage JSON's officer_team_name.
func teamName(code string) string { return workflows.TeamName(code) }
