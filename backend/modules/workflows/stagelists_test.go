package workflows

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/jackc/pgx/v5/pgxpool"
)

var wantHDFC = []struct{ code, name, role string }{
	{"received", "Received", ""},
	{"link_sent", "Link sent", RoleStart},
	{"application_submitted", "Application submitted", RoleTracking},
	{"dovr", "DOVR", ""},
	{"appraisal", "Appraisal", ""},
	{"sanctioned", "Sanctioned", RoleSanction},
	{"upload_documents", "Upload documents", ""},
	{"under_fixing", "Under fixing", ""},
	{"fixed", "Fixed", ""},
	{"signing", "Signing", ""},
	{"disbursement", "Disbursement", RoleDisbursement},
}

func TestSetBankStageListCreatesHDFCStages(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, err := svc.CreateBank(ctx, admin, "HDFC")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetBankStageList(ctx, admin, b.ID, StageListHDFC); err != nil {
		t.Fatalf("SetBankStageList: %v", err)
	}
	stages, err := svc.StagesFor(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != len(wantHDFC) {
		t.Fatalf("stages = %d, want %d", len(stages), len(wantHDFC))
	}
	for i, w := range wantHDFC {
		if stages[i].Code != w.code || stages[i].Name != w.name || stages[i].Role != w.role {
			t.Errorf("stage %d = %s/%s/%q, want %s/%s/%q", i, stages[i].Code, stages[i].Name, stages[i].Role, w.code, w.name, w.role)
		}
	}
	on, err := svc.BankPartPayments(ctx, b.ID)
	if err != nil || !on {
		t.Fatalf("part payments = %v (%v), want true", on, err)
	}
	banks, _ := svc.ListBanks(ctx)
	if banks[0].StageList != StageListHDFC || !banks[0].PartPayments {
		t.Fatalf("bank = %+v", banks[0])
	}
}

func seedCase(t *testing.T, pool *pgxpool.Pool, bank, user int64, mobile, stageCode, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var cust, id int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('C', $1) RETURNING id`, mobile).Scan(&cust); err != nil {
		t.Fatal(err)
	}
	q := `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, project_id, status,
	        dropped_reason_id, dropped_at)
	      VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE bank_id IS NULL AND code = $3), $4,
	        (SELECT id FROM projects WHERE is_system), $5,
	        CASE WHEN $5 = 'dropped' THEN (SELECT id FROM dropdown_values WHERE category = 'drop_reason' LIMIT 1) END,
	        CASE WHEN $5 = 'dropped' THEN now() END)
	      RETURNING id`
	if err := pool.QueryRow(ctx, q, cust, bank, stageCode, user, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO case_stage_events (case_id, stage_id, reached_at, changed_by)
		SELECT $1, current_stage_id, now(), $2 FROM cases WHERE id = $1`, id, user); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSetBankStageListMovesCases(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, _ := svc.CreateBank(ctx, admin, "HDFC")
	moves := []struct{ from, status, to string }{
		{"received", "active", "link_sent"},
		{"logged_in", "active", "application_submitted"},
		{"under_sanction", "dropped", "dovr"},
		{"sanctioned", "active", "sanctioned"},
		{"documentation", "active", "upload_documents"},
		{"disbursed", "completed", "disbursement"},
	}
	ids := make([]int64, len(moves))
	for i, m := range moves {
		ids[i] = seedCase(t, pool, b.ID, admin.ID, "900000000"+string(rune('0'+i)), m.from, m.status)
	}
	var eventsBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM case_stage_events`).Scan(&eventsBefore)

	if err := svc.SetBankStageList(ctx, admin, b.ID, StageListHDFC); err != nil {
		t.Fatalf("SetBankStageList: %v", err)
	}
	for i, m := range moves {
		var code, status string
		if err := pool.QueryRow(ctx, `SELECT s.code, c.status FROM cases c JOIN stages s ON s.id = c.current_stage_id WHERE c.id = $1 AND s.bank_id = $2`, ids[i], b.ID).Scan(&code, &status); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if code != m.to || status != m.status {
			t.Errorf("case from %s = %s/%s, want %s/%s", m.from, code, status, m.to, m.status)
		}
	}
	var eventsAfter, oldStage int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM case_stage_events`).Scan(&eventsAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM case_stage_events e JOIN stages s ON s.id = e.stage_id WHERE s.bank_id IS NULL`).Scan(&oldStage)
	if eventsAfter != eventsBefore || oldStage != eventsBefore {
		t.Fatalf("events before %d after %d on old stages %d: history must be untouched", eventsBefore, eventsAfter, oldStage)
	}
}

func TestSetBankStageListAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	manager := actor(t, pool, "lead", access.RoleManager)
	ctx := context.Background()
	b, _ := svc.CreateBank(ctx, admin, "HDFC")
	if err := svc.SetBankStageList(ctx, manager, b.ID, StageListHDFC); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("manager err = %v, want ErrForbidden", err)
	}
	if err := svc.SetBankPartPayments(ctx, manager, b.ID, true); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("manager part payments err = %v, want ErrForbidden", err)
	}
}

func TestSetBankStageListTwiceRefused(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, _ := svc.CreateBank(ctx, admin, "HDFC")
	if err := svc.SetBankStageList(ctx, admin, b.ID, StageListHDFC); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetBankStageList(ctx, admin, b.ID, StageListHDFC); !errors.Is(err, ErrBankHasOwnStages) {
		t.Fatalf("second switch err = %v, want ErrBankHasOwnStages", err)
	}
	if err := svc.SetBankStageList(ctx, admin, b.ID, "standard"); !errors.Is(err, ErrInvalidStageList) {
		t.Fatalf("standard err = %v, want ErrInvalidStageList", err)
	}
	if err := svc.SetBankStageList(ctx, admin, 999999, StageListHDFC); !errors.Is(err, ErrBankNotFound) {
		t.Fatalf("missing bank err = %v, want ErrBankNotFound", err)
	}
}

func TestSetBankPartPayments(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, _ := svc.CreateBank(ctx, admin, "ICICI")
	if on, _ := svc.BankPartPayments(ctx, b.ID); on {
		t.Fatal("new bank should have part payments off")
	}
	if err := svc.SetBankPartPayments(ctx, admin, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := svc.BankPartPayments(ctx, b.ID); !on {
		t.Fatal("part payments not switched on")
	}
	banks, _ := svc.ListBanks(ctx)
	if banks[0].StageList != StageListStandard {
		t.Fatalf("stage list = %q, want standard", banks[0].StageList)
	}
}

func TestStageWithRole(t *testing.T) {
	stages := []Stage{{ID: 1, Code: "a"}, {ID: 2, Code: "b", Role: RoleSanction}}
	if s, ok := StageWithRole(stages, RoleSanction); !ok || s.ID != 2 {
		t.Fatalf("got %+v %v", s, ok)
	}
	if _, ok := StageWithRole(stages, RoleDisbursement); ok {
		t.Fatal("found a role that is not there")
	}
}

func TestStandardStagesCarryRoles(t *testing.T) {
	svc, _ := setup(t)
	std, err := svc.StandardStages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := StageWithRole(std, RoleTracking); !ok || s.Code != "logged_in" {
		t.Fatalf("tracking stage = %+v %v", s, ok)
	}
}

func TestHDFCStageListHasTeams(t *testing.T) {
	svc, pool := setup(t)
	admin := actor(t, pool, "root", access.RoleAdmin)
	ctx := context.Background()
	b, _ := svc.CreateBank(ctx, admin, "HDFC")
	if err := svc.SetBankStageList(ctx, admin, b.ID, StageListHDFC); err != nil {
		t.Fatal(err)
	}
	stages, _ := svc.StagesFor(ctx, b.ID)
	want := map[string]string{"link_sent": TeamTDEP, "dovr": TeamDOVR, "appraisal": TeamAppraisal, "under_fixing": TeamFixing}
	for _, st := range stages {
		if st.OfficerTeam != want[st.Code] {
			t.Errorf("team of %s = %q, want %q", st.Code, st.OfficerTeam, want[st.Code])
		}
	}
}

func TestStandardStagesHaveOfficerTeam(t *testing.T) {
	svc, _ := setup(t)
	std, _ := svc.StandardStages(context.Background())
	for _, st := range std {
		want := ""
		if st.Code == "received" {
			want = TeamOfficer
		}
		if st.OfficerTeam != want {
			t.Errorf("team of %s = %q, want %q", st.Code, st.OfficerTeam, want)
		}
	}
	if TeamName(TeamTDEP) != "TDEP" || TeamName(TeamAppraisal) != "Appraisal" || TeamName(TeamOfficer) != "Officer" {
		t.Fatal("team names")
	}
}
