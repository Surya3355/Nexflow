package apihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/bankqueries"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/officers"
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
	Leads     *leads.Service
	Officers  *officers.Service
	Queries   *bankqueries.Service
	// AllowedOrigins are other sites allowed to send changing requests, such
	// as the Vite dev server. The app's own address is always allowed.
	AllowedOrigins []string
	// Health checks the database for /healthz; nil means always healthy.
	Health func(context.Context) error
}

// healthHandler answers 200 "ok" when check passes within 2 seconds, else 503.
func healthHandler(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if check != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := check(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("database unavailable"))
				return
			}
		}
		_, _ = w.Write([]byte("ok"))
	}
}

// NewRouter returns the full API under /api. Login and the download link are
// public; everything else needs a signed-in, active user.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(RequireSameOrigin(d.AllowedOrigins))
	r.Use(d.Auth.SessionManager().LoadAndSave)
	d.Auth.Mount(r)

	r.Get("/api/documents/{token}", documents.DownloadHandler(d.Documents, d.Linker))
	r.Get("/healthz", healthHandler(d.Health))

	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireUser)

		h := &handlers{d: d}

		r.Get("/api/users", h.listUsers)
		r.Get("/api/config/banks", h.listBanks)
		r.Get("/api/config/projects", h.listProjects)
		r.Get("/api/config/projects/{id}", h.getProject)
		r.Get("/api/config/banks/{id}/stages", h.stagesForBank)
		r.Get("/api/config/stages", h.standardStages)
		r.Get("/api/config/dropdowns/{category}", h.dropdown)

		r.Get("/api/admin/users", h.listTeam)
		r.Post("/api/admin/users", h.createUser)
		r.Post("/api/admin/users/{id}/password", h.resetPassword)
		r.Get("/api/admin/users/{id}/open-work", h.userOpenWork)
		r.Post("/api/admin/users/{id}/deactivate", h.deactivateUser)
		r.Post("/api/admin/users/{id}/reactivate", h.reactivateUser)
		r.Put("/api/admin/users/{id}/role", h.changeUserRole)
		r.Post("/api/admin/banks", h.createBank)
		r.Put("/api/admin/banks/{id}", h.renameBank)
		r.Patch("/api/admin/banks/{id}", h.updateBankSettings)
		r.Get("/api/admin/stage-lists", h.stageLists)
		r.Post("/api/admin/projects", h.createProject)
		r.Put("/api/admin/projects/{id}", h.saveProject)
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

		r.Post("/api/leads", h.createLead)
		r.Get("/api/leads", h.listLeads)
		r.Get("/api/leads/{id}", h.getLead)
		r.Post("/api/leads/{id}/contacts", h.logLeadContact)
		r.Post("/api/leads/{id}/assign", h.assignLead)
		r.Post("/api/leads/{id}/builder-contact", h.setLeadBuilderContact)
		r.Post("/api/leads/{id}/drop", h.dropLead)
		r.Post("/api/leads/{id}/bring-back", h.bringBackLead)
		r.Post("/api/leads/{id}/case", h.leadToCase)
		r.Post("/api/customers/{id}/leads", h.addCustomerLead)

		r.Post("/api/cases", h.createCase)
		r.Get("/api/cases", h.listCases)
		r.Get("/api/cases/attention", h.caseAttention)
		r.Get("/api/cases/{id}", h.getCase)
		r.Get("/api/cases/{id}/history", h.caseHistory)
		r.Get("/api/customers/{id}", h.getCustomer)
		r.Put("/api/cases/{id}", h.updateCase)
		r.Post("/api/cases/{id}/stage", h.changeStage)
		r.Post("/api/cases/{id}/payments", h.addPayment)
		r.Patch("/api/cases/{id}/payments/{paymentID}", h.updatePayment)
		r.Delete("/api/cases/{id}/payments/{paymentID}", h.deletePayment)
		r.Post("/api/cases/{id}/assign", h.assignCase)
		r.Post("/api/cases/{id}/project", h.changeCaseProject)
		r.Post("/api/cases/{id}/builder-contact", h.setCaseBuilderContact)
		r.Post("/api/cases/{id}/unit", h.setCaseUnit)
		r.Post("/api/cases/{id}/drop", h.dropCase)
		r.Delete("/api/cases/{id}", h.deleteCase)
		r.Get("/api/cases/{id}/documents", h.listDocuments)
		r.Post("/api/cases/{id}/documents", h.uploadDocument)
		r.Get("/api/cases/{id}/documents/{docID}/link", h.documentLink)

		r.Get("/api/officers", h.listOfficers)
		r.Post("/api/officers", h.addOfficer)
		r.Put("/api/officers/{id}", h.updateOfficer)
		r.Delete("/api/officers/{id}", h.removeOfficer)
		r.Get("/api/cases/{id}/officers", h.caseOfficers)
		r.Post("/api/cases/{id}/officers", h.setCaseOfficer)
		r.Get("/api/cases/{id}/queries", h.caseQueries)
		r.Post("/api/cases/{id}/queries", h.addQuery)
		r.Put("/api/queries/{id}", h.editQuery)
		r.Delete("/api/queries/{id}", h.removeQuery)
		r.Post("/api/queries/{id}/send", h.sendQueryProof)
		r.Post("/api/queries/{id}/accept", h.acceptQuery)
		r.Post("/api/queries/{id}/reject", h.rejectQuery)
		r.Post("/api/queries/{id}/remove-proof", h.removeQueryProof)

		r.Get("/api/search", h.search)

		r.Get("/api/reports/dashboard", h.dashboard)
		r.Get("/api/reports/rows", h.reportRows)
		r.Get("/api/reports/export.csv", h.exportCSV)
		r.Get("/api/reports/disbursements", h.disbursementRows)
		r.Get("/api/reports/disbursements.csv", h.exportDisbursementsCSV)
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
