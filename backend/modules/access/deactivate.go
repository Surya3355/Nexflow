package access

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access/accessdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
)

// Errors for switching users off and changing roles (spec 2026-11-27-hardening §4.2).
var (
	ErrTakeoverRequired = errors.New("access: choose who takes over this person's open work")
	ErrTakeoverInvalid  = errors.New("access: the person taking over must be another active user")
	ErrLastAdmin        = errors.New("access: the last active admin can't be switched off or changed to another role")
	ErrSelfDeactivate   = errors.New("access: you can't switch yourself off")
)

// OpenWork is what a user still has open.
type OpenWork struct{ Leads, Cases, Queries int }

func (w OpenWork) total() int { return w.Leads + w.Cases + w.Queries }

// Reassigner moves all of one user's open leads, cases and bank queries to
// another user, inside the transaction carried by ctx (history.WithTx). The
// handover module provides it, since access can't import cases or leads.
type Reassigner interface {
	ReassignAllTx(ctx context.Context, actorID, from, to int64) (leads, cases, queries int, err error)
}

// SetReassigner sets the handover used by Deactivate.
func (s *Service) SetReassigner(r Reassigner) { s.reassigner = r }

// OpenWork counts a user's open leads, active cases and unresolved bank queries. Admin only.
func (s *Service) OpenWork(ctx context.Context, actor User, userID int64) (OpenWork, error) {
	if !Can(actor.Role, ActionManageUsers) {
		return OpenWork{}, ErrForbidden
	}
	if _, err := s.q.GetUserByID(ctx, userID); errors.Is(err, pgx.ErrNoRows) {
		return OpenWork{}, ErrUserNotFound
	} else if err != nil {
		return OpenWork{}, fmt.Errorf("access: load user: %w", err)
	}
	return countOpenWork(ctx, s.q, userID)
}

func countOpenWork(ctx context.Context, q *accessdb.Queries, userID int64) (OpenWork, error) {
	r, err := q.CountOpenWork(ctx, userID)
	if err != nil {
		return OpenWork{}, fmt.Errorf("access: count open work: %w", err)
	}
	return OpenWork{Leads: int(r.Leads), Cases: int(r.Cases), Queries: int(r.Queries)}, nil
}

// Deactivate switches a user off. When they have open work, takeoverID must
// name another active user, who gets all of it in the same transaction. The
// user's sessions end at once (every request reloads the user). Admin only.
func (s *Service) Deactivate(ctx context.Context, actor User, userID int64, takeoverID *int64) error {
	if !Can(actor.Role, ActionManageUsers) {
		return ErrForbidden
	}
	if userID == actor.ID {
		return ErrSelfDeactivate
	}
	return s.inTx(ctx, func(txCtx context.Context, q *accessdb.Queries) error {
		target, err := lockUser(txCtx, q, userID)
		if err != nil {
			return err
		}
		if !target.IsActive {
			return nil
		}
		if Role(target.Role) == RoleAdmin {
			if err := requireOtherAdmin(txCtx, q); err != nil {
				return err
			}
		}
		work, err := countOpenWork(txCtx, q, userID)
		if err != nil {
			return err
		}
		details := map[string]any{"takeover_user_id": nil, "leads": 0, "cases": 0, "queries": 0}
		if work.total() > 0 {
			if takeoverID == nil {
				return ErrTakeoverRequired
			}
			if err := checkTakeover(txCtx, q, userID, *takeoverID); err != nil {
				return err
			}
			if s.reassigner == nil {
				return fmt.Errorf("access: no handover configured")
			}
			leads, cases, queries, err := s.reassigner.ReassignAllTx(txCtx, actor.ID, userID, *takeoverID)
			if err != nil {
				return fmt.Errorf("access: handover: %w", err)
			}
			details = map[string]any{"takeover_user_id": *takeoverID, "leads": leads, "cases": cases, "queries": queries}
		}
		if err := q.SetUserActive(txCtx, accessdb.SetUserActiveParams{ID: userID, IsActive: false, UpdatedBy: actorRef(actor.ID)}); err != nil {
			return fmt.Errorf("access: switch off: %w", err)
		}
		return s.audit(txCtx, actor, "user.deactivated", userID, details)
	})
}

// Reactivate switches a user back on. Admin only.
func (s *Service) Reactivate(ctx context.Context, actor User, userID int64) error {
	if !Can(actor.Role, ActionManageUsers) {
		return ErrForbidden
	}
	return s.inTx(ctx, func(txCtx context.Context, q *accessdb.Queries) error {
		target, err := lockUser(txCtx, q, userID)
		if err != nil {
			return err
		}
		if target.IsActive {
			return nil
		}
		if err := q.SetUserActive(txCtx, accessdb.SetUserActiveParams{ID: userID, IsActive: true, UpdatedBy: actorRef(actor.ID)}); err != nil {
			return fmt.Errorf("access: switch on: %w", err)
		}
		return s.audit(txCtx, actor, "user.reactivated", userID, nil)
	})
}

// ChangeRole sets a user's role. The last active admin keeps the admin role. Admin only.
func (s *Service) ChangeRole(ctx context.Context, actor User, userID int64, role Role) error {
	if !Can(actor.Role, ActionManageUsers) {
		return ErrForbidden
	}
	if !role.Valid() {
		return ErrInvalidRole
	}
	return s.inTx(ctx, func(txCtx context.Context, q *accessdb.Queries) error {
		target, err := lockUser(txCtx, q, userID)
		if err != nil {
			return err
		}
		before := Role(target.Role)
		if before == role {
			return nil
		}
		if before == RoleAdmin && target.IsActive {
			if err := requireOtherAdmin(txCtx, q); err != nil {
				return err
			}
		}
		if err := q.SetUserRole(txCtx, accessdb.SetUserRoleParams{ID: userID, Role: string(role), UpdatedBy: actorRef(actor.ID)}); err != nil {
			return fmt.Errorf("access: set role: %w", err)
		}
		return s.audit(txCtx, actor, "user.role_changed", userID, map[string]any{"before": string(before), "after": string(role)})
	})
}

func lockUser(ctx context.Context, q *accessdb.Queries, id int64) (accessdb.LockUserRow, error) {
	row, err := q.LockUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrUserNotFound
	}
	if err != nil {
		return row, fmt.Errorf("access: lock user: %w", err)
	}
	return row, nil
}

// requireOtherAdmin refuses when the user being changed is the only active admin.
func requireOtherAdmin(ctx context.Context, q *accessdb.Queries) error {
	n, err := q.CountActiveAdminsLocked(ctx)
	if err != nil {
		return fmt.Errorf("access: count admins: %w", err)
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func checkTakeover(ctx context.Context, q *accessdb.Queries, from, to int64) error {
	if to == from {
		return ErrTakeoverInvalid
	}
	row, err := q.LockUser(ctx, to)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.IsActive) {
		return ErrTakeoverInvalid
	}
	if err != nil {
		return fmt.Errorf("access: check takeover: %w", err)
	}
	return nil
}

// inTx runs fn in one transaction, with the transaction in its context so
// audit entries and the handover join it.
func (s *Service) inTx(ctx context.Context, fn func(context.Context, *accessdb.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("access: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(history.WithTx(ctx, tx), s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("access: commit: %w", err)
	}
	return nil
}

func (s *Service) audit(ctx context.Context, actor User, action string, userID int64, details map[string]any) error {
	actorID, id := actor.ID, userID
	e := history.Entry{ActorUserID: &actorID, Action: action, EntityType: "user", EntityID: &id}
	if details != nil {
		e.Details = details
	}
	if err := s.rec.Record(ctx, e); err != nil {
		return fmt.Errorf("access: audit: %w", err)
	}
	return nil
}
