package cases

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// fakeNotifier records alerts queued by Assign.
type fakeNotifier struct{ assigned []int64 }

func (n *fakeNotifier) QueueAssigned(_ context.Context, caseID, recipientID int64) error {
	n.assigned = append(n.assigned, recipientID)
	return nil
}

func seedUser(t *testing.T, f fixture, name string, active bool) access.User {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role, is_active)
		 VALUES ($1, $1, 'x', 'employee', $2) RETURNING id`, name, active), &id)
	return access.User{ID: id, Username: name, Role: access.RoleEmployee, IsActive: active}
}

func TestAnyStaffCanReassignAtAnyStage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("ASG1"), nil, nil); err != nil {
		t.Fatalf("logged_in: %v", err)
	}
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "disbursed"), nil, nil, nil); err != nil {
		t.Fatalf("disbursed: %v", err)
	}
	other := seedUser(t, f, "colleague", true)
	got, err := f.svc.Assign(ctx, f.actor, c.ID, other.ID)
	if err != nil {
		t.Fatalf("employee reassign at Disbursed: %v", err)
	}
	if got.AssigneeID != other.ID {
		t.Fatalf("assignee = %d, want %d", got.AssigneeID, other.ID)
	}
}

func TestReassignmentKeepsHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	other := seedUser(t, f, "colleague", true)
	if _, err := f.svc.Assign(ctx, f.actor, c.ID, other.ID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM case_assignment_events WHERE case_id=$1`, c.ID); n != 2 {
		t.Fatalf("assignment events = %d, want 2", n)
	}
	var from, to int64
	mustScan(t, f.pool.QueryRow(ctx,
		`SELECT from_user_id, to_user_id FROM case_assignment_events WHERE case_id=$1 ORDER BY id DESC LIMIT 1`, c.ID), &from, &to)
	if from != f.actor.ID || to != other.ID {
		t.Fatalf("history from=%d to=%d, want from=%d to=%d", from, to, f.actor.ID, other.ID)
	}
}

func TestCannotAssignToInactiveUser(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	gone := seedUser(t, f, "left", false)
	_, err := f.svc.Assign(context.Background(), f.actor, c.ID, gone.ID)
	if !errors.Is(err, ErrAssigneeInactive) {
		t.Fatalf("err = %v, want ErrAssigneeInactive", err)
	}
}

func TestAssignQueuesAssignedAlert(t *testing.T) {
	f := newFixture(t)
	n := &fakeNotifier{}
	f.svc.SetNotifier(n)
	c := newCase(t, f)
	other := seedUser(t, f, "colleague", true)
	if _, err := f.svc.Assign(context.Background(), f.actor, c.ID, other.ID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(n.assigned) != 1 || n.assigned[0] != other.ID {
		t.Fatalf("queued alerts = %v, want [%d]", n.assigned, other.ID)
	}
}
