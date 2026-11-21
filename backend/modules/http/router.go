package apihttp

import (
	"net/http"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/reports"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/go-chi/chi/v5"
)

// Deps are the services the API calls.
type Deps struct {
	Auth      *access.Handler
	Access    *access.Service
	Workflows *workflows.Service
	Customers *customers.Service
	Cases     *cases.Service
	Documents *documents.Service
	Linker    *documents.Linker
	Archive   *archive.Service
	Reports   *reports.Service
}

// NewRouter returns the full API under /api. Login and the download link are
// public; everything else needs a signed-in, active user.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(d.Auth.SessionManager().LoadAndSave)
	d.Auth.Mount(r)

	r.Get("/api/documents/{token}", documents.DownloadHandler(d.Documents, d.Linker))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireUser)

		h := &handlers{d: d}

		r.Get("/api/users", h.listUsers)
		r.Get("/api/config/banks", h.listBanks)
		r.Get("/api/config/projects", h.listProjects)
		r.Get("/api/config/banks/{id}/stages", h.stagesForBank)
		r.Get("/api/config/stages", h.standardStages)
		r.Get("/api/config/dropdowns/{category}", h.dropdown)

		r.Get("/api/admin/users", h.listTeam)
		r.Post("/api/admin/users", h.createUser)
		r.Post("/api/admin/users/{id}/password", h.resetPassword)
		r.Post("/api/admin/banks", h.createBank)
		r.Put("/api/admin/banks/{id}", h.renameBank)
		r.Post("/api/admin/projects", h.createProject)
		r.Put("/api/admin/projects/{id}", h.renameProject)
		r.Post("/api/admin/stages", h.addStage)
		r.Patch("/api/admin/stages/{id}", h.setStageRequiresDocument)
		r.Post("/api/admin/dropdowns", h.addDropdownValue)
		r.Get("/api/admin/archive", h.listArchive)
		r.Post("/api/admin/archive/{id}/restore", h.restoreArchive)

		r.Post("/api/customers", h.createCustomer)
		r.Put("/api/customers/{id}", h.updateCustomer)
		r.Post("/api/customers/{id}/drop", h.dropCustomer)
		r.Post("/api/customers/{id}/reactivate", h.reactivateCustomer)
		r.Delete("/api/customers/{id}", h.deleteCustomer)

		r.Post("/api/cases", h.createCase)
		r.Get("/api/cases", h.listCases)
		r.Get("/api/cases/attention", h.caseAttention)
		r.Get("/api/cases/{id}", h.getCase)
		r.Get("/api/cases/{id}/history", h.caseHistory)
		r.Get("/api/customers/{id}", h.getCustomer)
		r.Put("/api/cases/{id}", h.updateCase)
		r.Post("/api/cases/{id}/stage", h.changeStage)
		r.Post("/api/cases/{id}/assign", h.assignCase)
		r.Post("/api/cases/{id}/project", h.changeCaseProject)
		r.Post("/api/cases/{id}/drop", h.dropCase)
		r.Delete("/api/cases/{id}", h.deleteCase)
		r.Get("/api/cases/{id}/documents", h.listDocuments)
		r.Post("/api/cases/{id}/documents", h.uploadDocument)
		r.Get("/api/cases/{id}/documents/{docID}/link", h.documentLink)

		r.Get("/api/search", h.search)

		r.Get("/api/reports/dashboard", h.dashboard)
		r.Get("/api/reports/rows", h.reportRows)
		r.Get("/api/reports/export.csv", h.exportCSV)
	})
	return r
}

// indiaTime is the zone every date the app sends is in (spec: dates follow India time).
var indiaTime = time.FixedZone("IST", 5*60*60+30*60)

// parseDay reads a YYYY-MM-DD query value as the start of that India day. Empty means no value.
func parseDay(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, indiaTime)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
