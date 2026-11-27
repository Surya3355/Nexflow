package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/Surya3355/nexflow/backend/modules/reports/reportsdb"
)

// DisbursementRow is one case with a payment in the period (spec 2026-11-25 §5.6).
type DisbursementRow struct {
	CaseID       int64
	Customer     string
	Mobile       string
	Bank         string
	Project      string
	Tracking     string
	Assignee     string
	PaidInPeriod string
	TotalPaid    string
	LastPaidOn   string // YYYY-MM-DD
	Kind         string // "Part" or "Full"
	Sanctioned   string // sanctioned amount, else loan amount
	Remaining    string
	Status       string
	Dropped      bool
}

// DisbursementHeader is the column titles, in the same order as Fields.
var DisbursementHeader = []string{"Case ID", "Customer", "Mobile", "Bank", "Project", "Tracking no.", "Assigned to",
	"Paid in period", "Total paid", "Last payment", "Type", "Sanctioned", "Remaining", "Status"}

// Fields returns the row values in DisbursementHeader order.
func (r DisbursementRow) Fields() []string {
	return []string{strconv.FormatInt(r.CaseID, 10), r.Customer, r.Mobile, r.Bank, r.Project, r.Tracking, r.Assignee,
		r.PaidInPeriod, r.TotalPaid, r.LastPaidOn, r.Kind, r.Sanctioned, r.Remaining, r.Status}
}

// DisbursementRows returns one row per case with a payment in p, newest payment first.
func (s *Service) DisbursementRows(ctx context.Context, p Period) ([]DisbursementRow, error) {
	rows, err := s.q.DisbursementRows(ctx, reportsdb.DisbursementRowsParams{
		FromTs: ts(p.From), ToTs: ts(p.To), BankID: int64Null(p.BankID), ProjectID: int64Null(p.ProjectID),
	})
	if err != nil {
		return nil, fmt.Errorf("reports: disbursement rows: %w", err)
	}
	out := make([]DisbursementRow, 0, len(rows))
	for _, r := range rows {
		kind := "Part"
		if r.FullyPaid {
			kind = "Full"
		}
		out = append(out, DisbursementRow{CaseID: r.ID, Customer: r.CustomerName, Mobile: r.Mobile, Bank: r.BankName,
			Project: r.ProjectName, Tracking: r.TrackingNumber, Assignee: r.AssigneeName, PaidInPeriod: r.PaidInPeriod,
			TotalPaid: r.TotalPaid, LastPaidOn: r.LastPaidOn, Kind: kind, Sanctioned: r.Sanctioned,
			Remaining: r.Remaining, Status: r.Status, Dropped: r.Status == "dropped"})
	}
	return out, nil
}

// ExportDisbursementsCSV writes the disbursement rows as CSV for Excel, like ExportCSV.
func ExportDisbursementsCSV(w io.Writer, rows []DisbursementRow) error {
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(DisbursementHeader); err != nil {
		return err
	}
	for _, r := range rows {
		fields := r.Fields()
		for i, f := range fields {
			fields[i] = neutraliseFormula(f)
		}
		if err := cw.Write(fields); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
