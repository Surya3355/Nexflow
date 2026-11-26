package cases

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

// addOfficer inserts a current officer of bank in team.
func addOfficer(t *testing.T, f fixture, bank int64, team, name, phone string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO bank_officers (bank_id, team, name, phone) VALUES ($1, $2, $3, $4) RETURNING id`, bank, team, name, phone), &id)
	return id
}

func officerOf(t *testing.T, f fixture, caseID int64, team string) *Officer {
	t.Helper()
	list, err := f.svc.Officers(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Officers: %v", err)
	}
	for _, o := range list {
		if o.Team == team {
			return o.Officer
		}
	}
	t.Fatalf("team %s not listed in %+v", team, list)
	return nil
}

func moveTo(t *testing.T, f fixture, c Case, bank int64, code string, officer int64) error {
	t.Helper()
	m := StageMove{ToStageID: bankStage(t, f, bank, code), OfficerID: officer}
	if code == "application_submitted" {
		tr := "APP123"
		m.TrackingNumber = &tr
	}
	_, err := f.svc.ChangeStage(context.Background(), f.actor, c.ID, m)
	return err
}

func TestMoveToDOVRPicksOfficer(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	ravi := addOfficer(t, f, bank, workflows.TeamDOVR, "Ravi", "9876543210")
	c := caseAtBank(t, f, bank, "500000")
	if err := moveTo(t, f, c, bank, "dovr", ravi); err != nil {
		t.Fatalf("move: %v", err)
	}
	o := officerOf(t, f, c.ID, workflows.TeamDOVR)
	if o == nil || o.ID != ravi || o.Name != "Ravi" || o.Phone != "9876543210" || o.Removed {
		t.Fatalf("DOVR officer = %+v", o)
	}
	hist, err := f.svc.OfficerHistory(context.Background(), c.ID)
	if err != nil || len(hist) != 1 || hist[0].TeamName != "DOVR" || hist[0].OfficerName != "Ravi" || hist[0].ChangedBy == "" {
		t.Fatalf("history = %+v %v", hist, err)
	}
}

func TestMoveWithoutOfficerKeepsCurrent(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	ravi := addOfficer(t, f, bank, workflows.TeamDOVR, "Ravi", "9876543210")
	c := caseAtBank(t, f, bank, "500000")
	if err := moveTo(t, f, c, bank, "dovr", ravi); err != nil {
		t.Fatal(err)
	}
	if err := moveTo(t, f, c, bank, "link_sent", 0); err != nil {
		t.Fatal(err)
	}
	if err := moveTo(t, f, c, bank, "dovr", 0); err != nil {
		t.Fatal(err)
	}
	if err := moveTo(t, f, c, bank, "dovr", ravi); err != nil {
		t.Fatal(err)
	}
	if o := officerOf(t, f, c.ID, workflows.TeamDOVR); o == nil || o.ID != ravi {
		t.Fatalf("officer = %+v", o)
	}
	if n := f.count(t, `SELECT count(*) FROM case_officers WHERE case_id = $1`, c.ID); n != 1 {
		t.Fatalf("picks = %d, want 1 (same officer again adds nothing)", n)
	}
}

func TestPickRefusesWrongOfficer(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	appr := addOfficer(t, f, bank, workflows.TeamAppraisal, "Appu", "9876543210")
	sbiOfficer := addOfficer(t, f, f.bankID, workflows.TeamOfficer, "Sita", "9876543211")
	gone := addOfficer(t, f, bank, workflows.TeamDOVR, "Gone", "9876543212")
	if _, err := f.pool.Exec(context.Background(), `UPDATE bank_officers SET removed_at = now() WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	c := caseAtBank(t, f, bank, "500000")
	for name, id := range map[string]int64{"other team": appr, "other bank": sbiOfficer, "removed": gone, "unknown": 99999} {
		if err := moveTo(t, f, c, bank, "dovr", id); !errors.Is(err, ErrOfficerInvalid) {
			t.Errorf("%s: err = %v, want ErrOfficerInvalid", name, err)
		}
		if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamDOVR, id); !errors.Is(err, ErrOfficerInvalid) {
			t.Errorf("SetOfficer %s: err = %v, want ErrOfficerInvalid", name, err)
		}
	}
	dovr := addOfficer(t, f, bank, workflows.TeamDOVR, "Ravi", "9876543213")
	if err := moveTo(t, f, c, bank, "appraisal", dovr); !errors.Is(err, ErrOfficerInvalid) {
		t.Errorf("DOVR officer at Appraisal: err = %v", err)
	}
	if err := moveTo(t, f, c, bank, "signing", dovr); !errors.Is(err, ErrOfficerInvalid) {
		t.Errorf("officer at stage without team: err = %v", err)
	}
}

func TestSetOfficer(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	a := addOfficer(t, f, bank, workflows.TeamTDEP, "Asha", "9876543210")
	b := addOfficer(t, f, bank, workflows.TeamTDEP, "Bala", "9876543211")
	c := caseAtBank(t, f, bank, "500000")
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamTDEP, a); err != nil {
		t.Fatal(err)
	}
	f.rec.entries = nil
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamTDEP, b); err != nil {
		t.Fatal(err)
	}
	if o := officerOf(t, f, c.ID, workflows.TeamTDEP); o == nil || o.ID != b {
		t.Fatalf("officer = %+v", o)
	}
	if len(f.rec.entries) != 1 || f.rec.entries[0].Action != "case.officer_changed" {
		t.Fatalf("audit = %+v", f.rec.entries)
	}
	d := f.rec.entries[0].Details.(map[string]any)
	if d["team"] != workflows.TeamTDEP || d["from"] != a || d["to"] != b {
		t.Fatalf("audit details = %+v", d)
	}
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamOfficer, b); !errors.Is(err, ErrTeamNotOnStages) {
		t.Fatalf("team not on list err = %v", err)
	}
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamTDEP, 0); !errors.Is(err, ErrOfficerInvalid) {
		t.Fatalf("officer 0 err = %v", err)
	}
	if _, err := f.svc.Drop(context.Background(), f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamTDEP, a); !errors.Is(err, ErrCaseDropped) {
		t.Fatalf("dropped err = %v", err)
	}
}

func TestOfficersListsReachedTeamsOnly(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "500000")
	if err := moveTo(t, f, c, bank, "dovr", 0); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.Officers(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Team != workflows.TeamTDEP || list[1].Team != workflows.TeamDOVR || list[1].TeamName != "DOVR" || list[1].Officer != nil {
		t.Fatalf("officers = %+v", list)
	}
}

func TestRemovedOfficerStillShownOnCase(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	a := addOfficer(t, f, bank, workflows.TeamTDEP, "Asha", "9876543210")
	c := caseAtBank(t, f, bank, "500000")
	if _, err := f.svc.SetOfficer(context.Background(), f.actor, c.ID, workflows.TeamTDEP, a); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE bank_officers SET removed_at = now() WHERE id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if o := officerOf(t, f, c.ID, workflows.TeamTDEP); o == nil || !o.Removed || o.Name != "Asha" {
		t.Fatalf("officer = %+v", o)
	}
}

func TestCreateWithOfficers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := hdfcBank(t, f)
	tdep := addOfficer(t, f, bank, workflows.TeamTDEP, "Asha", "9876543210")
	dovr := addOfficer(t, f, bank, workflows.TeamDOVR, "Ravi", "9876543211")
	sbi := addOfficer(t, f, f.bankID, workflows.TeamOfficer, "Sita", "9876543212")
	base := NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: bank, LoanAmount: "500000", AssigneeID: f.actor.ID}

	in := base
	in.Officers = map[string]int64{workflows.TeamTDEP: tdep}
	c, err := f.svc.Create(ctx, f.actor, in)
	if err != nil {
		t.Fatalf("HDFC with TDEP: %v", err)
	}
	if o := officerOf(t, f, c.ID, workflows.TeamTDEP); o == nil || o.ID != tdep {
		t.Fatalf("TDEP = %+v", o)
	}

	in = base
	in.Officers = map[string]int64{workflows.TeamDOVR: dovr}
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrTeamNotOnStages) {
		t.Fatalf("DOVR at creation err = %v, want ErrTeamNotOnStages", err)
	}
	in = base
	in.Officers = map[string]int64{workflows.TeamTDEP: sbi}
	if _, err := f.svc.Create(ctx, f.actor, in); !errors.Is(err, ErrOfficerInvalid) {
		t.Fatalf("SBI officer on HDFC err = %v, want ErrOfficerInvalid", err)
	}
	in = base
	in.Officers = map[string]int64{workflows.TeamTDEP: 0}
	if _, err := f.svc.Create(ctx, f.actor, in); err != nil {
		t.Fatalf("empty pick: %v", err)
	}

	// Standard bank starting at Logged in: the Officer team (on Received) is reached.
	std := NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "500000", AssigneeID: f.actor.ID,
		StartAt: "tracking", TrackingNumber: "SBI777", Officers: map[string]int64{workflows.TeamOfficer: sbi}}
	c2, err := f.svc.Create(ctx, f.actor, std)
	if err != nil {
		t.Fatalf("standard at Logged in: %v", err)
	}
	if o := officerOf(t, f, c2.ID, workflows.TeamOfficer); o == nil || o.ID != sbi {
		t.Fatalf("Officer = %+v", o)
	}
}

func TestCreateFromLeadWithOfficer(t *testing.T) {
	f := newFixture(t)
	leadID, _ := f.openLead(t)
	sbi := addOfficer(t, f, f.bankID, workflows.TeamOfficer, "Sita", "9876543212")
	c, err := f.svc.CreateFromLead(context.Background(), f.actor, leadID, FromLead{BankID: f.bankID, LoanAmount: "4500000",
		AssigneeID: f.actor.ID, Officers: map[string]int64{workflows.TeamOfficer: sbi}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if o := officerOf(t, f, c.ID, workflows.TeamOfficer); o == nil || o.ID != sbi {
		t.Fatalf("Officer = %+v", o)
	}
}

func TestOfficerPickNotBeforeItsMove(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	ravi := addOfficer(t, f, bank, workflows.TeamDOVR, "Ravi", "9876543210")
	c := caseAtBank(t, f, bank, "500000")
	if err := moveTo(t, f, c, bank, "dovr", ravi); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(*) FROM case_officers o
		WHERE o.case_id = $1 AND o.picked_at < (SELECT max(reached_at) FROM case_stage_events WHERE case_id = $1)`, c.ID); n != 0 {
		t.Fatal("officer pick is dated before the stage move it came with")
	}
}
