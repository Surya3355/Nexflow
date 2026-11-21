package reports

import (
	"encoding/csv"
	"io"
	"strconv"
)

// Row is one case as the report shows it on screen and in the CSV file.
type Row struct {
	CaseID       int64
	CustomerName string
	Mobile       string
	Bank         string
	Project      string
	Stage        string
	Status       string
	Assignee     string
	LoanAmount   string
	Tracking     string
	CreatedOn    string
	FollowUpOn   string
}

// Header is the column titles, in the same order as Fields.
var Header = []string{"Case ID", "Customer", "Mobile", "Bank", "Project", "Stage", "Status",
	"Assignee", "Loan amount", "Tracking number", "Created on", "Follow-up on"}

// Fields returns the row values in Header order.
func (r Row) Fields() []string {
	return []string{
		strconv.FormatInt(r.CaseID, 10), r.CustomerName, r.Mobile, r.Bank, r.Project, r.Stage, r.Status,
		r.Assignee, r.LoanAmount, r.Tracking, r.CreatedOn, r.FollowUpOn,
	}
}

// ExportCSV writes rows as CSV for Excel: a UTF-8 byte-order mark first, so
// Indian names display correctly, and a header line (spec §9, plan Task 28).
func ExportCSV(w io.Writer, rows []Row) error {
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(Header); err != nil {
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

// neutraliseFormula prefixes a cell that starts with a formula character so
// spreadsheet software shows it as text instead of running it.
func neutraliseFormula(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@':
		return "'" + s
	}
	return s
}
