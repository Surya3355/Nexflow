// Package apihttp is the JSON API. Each handler decodes the request, calls a
// module service, and writes the result. Authorization is enforced inside the
// services (access.Can), so these handlers never decide who may act.
package apihttp

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

// statusFor maps a service error to an HTTP status. Unknown errors are 500
// and their text is not shown to the client.
func statusFor(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, access.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, access.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, access.ErrUserNotFound),
		errors.Is(err, cases.ErrCaseNotFound),
		errors.Is(err, customers.ErrCustomerNotFound),
		errors.Is(err, archive.ErrNotFound),
		errors.Is(err, documents.ErrNotFoundDoc),
		errors.Is(err, workflows.ErrBankNotFound),
		errors.Is(err, workflows.ErrProjectNotFound),
		errors.Is(err, cases.ErrProjectNotFound),
		errors.Is(err, leads.ErrLeadNotFound),
		errors.Is(err, leads.ErrProjectNotFound):
		return http.StatusNotFound
	case errors.Is(err, access.ErrUsernameTaken),
		errors.Is(err, cases.ErrDuplicateTracking),
		errors.Is(err, customers.ErrMobileInUse),
		errors.Is(err, archive.ErrMobileInUse),
		errors.Is(err, archive.ErrDuplicateTracking),
		errors.Is(err, archive.ErrAlreadyRestored),
		errors.Is(err, archive.ErrCustomerArchived),
		errors.Is(err, archive.ErrCustomerHasCases),
		errors.Is(err, cases.ErrAlreadyDropped),
		errors.Is(err, customers.ErrAlreadyDropped),
		errors.Is(err, cases.ErrCaseDropped),
		errors.Is(err, cases.ErrAssigneeInactive),
		errors.Is(err, workflows.ErrBankNameTaken),
		errors.Is(err, workflows.ErrProjectNameTaken),
		errors.Is(err, workflows.ErrProjectLocked),
		errors.Is(err, cases.ErrCustomerDropped),
		errors.Is(err, customers.ErrNotDropped),
		errors.Is(err, leads.ErrLeadClosed),
		errors.Is(err, leads.ErrLeadExists),
		errors.Is(err, leads.ErrLeadHasCase),
		errors.Is(err, leads.ErrCustomerDropped),
		errors.Is(err, archive.ErrCustomerHasOpenLeads):
		return http.StatusConflict
	case errors.Is(err, documents.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, customers.ErrInvalidMobile),
		errors.Is(err, cases.ErrInvalidAmount),
		errors.Is(err, cases.ErrTrackingRequired),
		errors.Is(err, cases.ErrProjectRequired),
		errors.Is(err, cases.ErrInvalidTracking),
		errors.Is(err, cases.ErrStageNotForBank),
		errors.Is(err, cases.ErrNoStartStage),
		errors.Is(err, cases.ErrInvalidReason),
		errors.Is(err, cases.ErrRemarkRequired),
		errors.Is(err, customers.ErrInvalidReason),
		errors.Is(err, customers.ErrRemarkRequired),
		errors.Is(err, archive.ErrInvalidReason),
		errors.Is(err, archive.ErrRemarkRequired),
		errors.Is(err, archive.ErrReasonRequired),
		errors.Is(err, access.ErrInvalidUsername),
		errors.Is(err, access.ErrInvalidRole),
		errors.Is(err, access.ErrWeakPassword),
		errors.Is(err, documents.ErrEmpty),
		errors.Is(err, documents.ErrUnsupportedType),
		errors.Is(err, documents.ErrLabelRequired),
		errors.Is(err, workflows.ErrBankNameRequired),
		errors.Is(err, workflows.ErrProjectNameRequired),
		errors.Is(err, customers.ErrNameRequired),
		errors.Is(err, cases.ErrInvalidUnit),
		errors.Is(err, cases.ErrInvalidStart),
		errors.Is(err, cases.ErrBankRequired),
		errors.Is(err, leads.ErrInvalidResponse),
		errors.Is(err, leads.ErrRemarkRequired),
		errors.Is(err, leads.ErrProjectRequired),
		errors.Is(err, leads.ErrAssigneeInvalid),
		errors.Is(err, leads.ErrInvalidReason),
		errors.Is(err, leads.ErrDropRemarkRequired):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// userMessage gives the text a client may see for an error.
func userMessage(err error, status int) string {
	if status == http.StatusInternalServerError {
		return "something went wrong"
	}
	return err.Error()
}

func writeError(w http.ResponseWriter, err error) {
	status := statusFor(err)
	if status == http.StatusInternalServerError {
		log.Printf("apihttp: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": userMessage(err, status)})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads a JSON body of at most 1 MB.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return false
	}
	return true
}
