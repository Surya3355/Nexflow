package access

import (
	"context"
	"fmt"
)

// TeamMember is a user as the admin Team tab shows them.
type TeamMember struct {
	User
	Email     string
	OpenCases int // active cases assigned to the user
}

// ListTeam returns every user, including inactive ones. Admin only.
func (s *Service) ListTeam(ctx context.Context, actor User) ([]TeamMember, error) {
	if !Can(actor.Role, ActionManageUsers) {
		return nil, ErrForbidden
	}
	rows, err := s.q.ListTeam(ctx)
	if err != nil {
		return nil, fmt.Errorf("access: list team: %w", err)
	}
	out := make([]TeamMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeamMember{
			User:  User{ID: r.ID, Username: r.Username, FullName: r.FullName, Role: Role(r.Role), IsActive: r.IsActive},
			Email: r.Email, OpenCases: int(r.OpenCases),
		})
	}
	return out, nil
}
