package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func sampleRows() []Row {
	return []Row{
		{CaseID: 1, CustomerName: "Ravi Kumar", Mobile: "9876543210", Bank: "HDFC", Stage: "Logged in",
			Status: "active", Assignee: "Surya", LoanAmount: "2500000.00", Tracking: "APP1", CreatedOn: "2026-11-12"},
		{CaseID: 2, CustomerName: `Meena, "Priya" Iyer`, Mobile: "9123456780", Bank: "SBI", Stage: "Disbursed",
			Status: "completed", Assignee: "Asha", LoanAmount: "900000.50", CreatedOn: "2026-11-13", FollowUpOn: "2026-11-20"},
	}
}

func TestCSVStartsWithBOM(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportCSV(&buf, sampleRows()); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("CSV does not start with a UTF-8 BOM: % x", buf.Bytes()[:4])
	}
}

// Same rows, same values: what staff see on screen is what the file holds.
func TestCSVRowsMatchOnScreenRows(t *testing.T) {
	rows := sampleRows()
	var buf bytes.Buffer
	if err := ExportCSV(&buf, rows); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) != len(rows)+1 {
		t.Fatalf("records = %d, want header + %d rows", len(records), len(rows))
	}
	for i, r := range rows {
		got := records[i+1]
		want := r.Fields()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("row %d: CSV %v, screen %v", i, got, want)
		}
	}
}

func TestCSVEscapesCommasAndQuotes(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportCSV(&buf, sampleRows()); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := records[2][1]; got != `Meena, "Priya" Iyer` {
		t.Fatalf("customer name = %q, want it intact", got)
	}
}

// Spec §6: every role can view and export reports.
func TestAllStaffCanExport(t *testing.T) {
	f := newFixture(t)
	for _, role := range []access.Role{access.RoleAdmin, access.RoleManager, access.RoleEmployee} {
		u := access.User{Role: role, IsActive: true}
		if _, err := NewService(f.pool).Rows(context.Background(), u, RowFilter{}); err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
	}
	if _, err := NewService(f.pool).Rows(context.Background(), access.User{Role: access.Role("owner")}, RowFilter{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unknown role err = %v, want ErrForbidden", err)
	}
}

// A typed name such as =HYPERLINK(...) must not run as a formula in Excel.
func TestCSVNeutralisesFormulas(t *testing.T) {
	rows := []Row{{CaseID: 9, CustomerName: `=HYPERLINK("http://x","a")`, Mobile: "9000000000"}}
	var buf bytes.Buffer
	if err := ExportCSV(&buf, rows); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(buf.Bytes(), []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := records[1][1]; !strings.HasPrefix(got, "'=") {
		t.Fatalf("formula cell = %q, want a leading apostrophe", got)
	}
}

func TestNeutraliseTabCR(t *testing.T) {
	for in, want := range map[string]string{"\t=1+1": "'\t=1+1", "\r=1+1": "'\r=1+1", "=SUM(A1)": "'=SUM(A1)", "Ravi": "Ravi"} {
		if got := neutraliseFormula(in); got != want {
			t.Errorf("neutraliseFormula(%q) = %q, want %q", in, got, want)
		}
	}
}
