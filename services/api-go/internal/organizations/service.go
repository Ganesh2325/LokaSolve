package organizations

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/validate"
	"lokasolve/services/api-go/internal/views"
)

var errNotFound = httpx.New(404, "not_found", "That record was not found.")
var errForbidden = httpx.New(403, "forbidden", "You do not have access to that action.")

type Service struct {
	DB *database.DB
}

type CreateInput struct {
	Name      string
	Type      string
	RequestID string
}

type MemberInput struct {
	Email      string
	MemberRole string
	RequestID  string
}

func (s *Service) Create(ctx context.Context, principal authn.Principal, in CreateInput) (views.Organization, error) {
	fields := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 {
		fields["name"] = "Enter an organization name between 2 and 120 characters."
	}
	fields["org_type"] = validate.OrgType(in.Type)
	if invalid(fields) {
		return views.Organization{}, httpx.Validation(fields)
	}
	var org views.Organization
	flags := append(identity.AccessFlags(principal), "app.org_create", "app.org_bootstrap")
	err := s.DB.WithFlags(ctx, &principal.UserID, flags, func(tx pgx.Tx) error {
		var enabled bool
		if err := tx.QueryRow(ctx, `SELECT enabled FROM feature_flags WHERE key = 'org_self_service'`).Scan(&enabled); err != nil {
			return err
		}
		if !enabled && !principal.Has("admin:roles_grant") {
			return errForbidden
		}
		orgID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO organizations (id, name, org_type, is_demo) VALUES ($1, $2, $3, $4)
		`, orgID, name, in.Type, principal.IsDemo); err != nil {
			return err
		}
		org.ID = orgID.String()
		org.Status = "active"
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, member_role) VALUES ($1, $2, 'owner')
		`, orgID, principal.UserID); err != nil {
			return err
		}
		if err := identity.AssignRole(ctx, tx, principal.UserID, "org_admin", &orgID); err != nil {
			return err
		}
		org.Name = name
		org.Type = in.Type
		org.IsDemo = principal.IsDemo
		org.MemberRole = "owner"
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "organization.created", ResourceType: "organization",
			ResourceID: &orgID, TenantID: &orgID, RequestID: in.RequestID,
			Metadata: map[string]any{"org_type": in.Type},
		}); err != nil {
			return err
		}
		return events.Enqueue(ctx, tx, "organization", orgID, "organization.created", map[string]any{
			"organization_id": org.ID, "org_type": in.Type, "request_id": in.RequestID,
		})
	})
	return org, err
}

func (s *Service) Mine(ctx context.Context, principal authn.Principal) ([]views.Organization, error) {
	var orgs []views.Organization
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id::text, o.name, o.org_type, o.status, o.is_demo, m.member_role
			FROM organizations o
			JOIN organization_members m ON m.organization_id = o.id
			WHERE m.user_id = $1 AND m.status = 'active'
			ORDER BY o.name
		`, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var org views.Organization
			if err := rows.Scan(&org.ID, &org.Name, &org.Type, &org.Status, &org.IsDemo, &org.MemberRole); err != nil {
				return err
			}
			orgs = append(orgs, org)
		}
		return rows.Err()
	})
	if orgs == nil {
		orgs = []views.Organization{}
	}
	return orgs, err
}

func (s *Service) Get(ctx context.Context, principal authn.Principal, orgID uuid.UUID) (views.Organization, error) {
	if !principal.CanReadOrg(orgID) {
		return views.Organization{}, errNotFound
	}
	var org views.Organization
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id::text, name, org_type, status, is_demo FROM organizations WHERE id = $1
		`, orgID).Scan(&org.ID, &org.Name, &org.Type, &org.Status, &org.IsDemo)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound
		}
		return err
	})
	org.MemberRole = principal.MemberRole[orgID]
	return org, err
}

func (s *Service) Members(ctx context.Context, principal authn.Principal, orgID uuid.UUID) ([]views.Member, error) {
	if !principal.CanReadOrg(orgID) {
		return nil, errNotFound
	}
	var members []views.Member
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT user_id::text, COALESCE(email, ''), display_name, member_role, status
			FROM app_org_member_directory($1)
		`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var member views.Member
			if err := rows.Scan(&member.UserID, &member.Email, &member.DisplayName, &member.MemberRole, &member.Status); err != nil {
				return err
			}
			members = append(members, member)
		}
		return rows.Err()
	})
	if members == nil {
		members = []views.Member{}
	}
	return members, err
}

func (s *Service) AddMember(ctx context.Context, principal authn.Principal, orgID uuid.UUID, in MemberInput) error {
	if !principal.CanReadOrg(orgID) {
		return errNotFound
	}
	if !principal.CanManageOrg(orgID) {
		return errForbidden
	}
	email, emailErr := validate.Email(in.Email)
	role := in.MemberRole
	fields := map[string]string{"email": emailErr}
	switch role {
	case "admin", "member", "field_worker":
	case "owner":
		if principal.MemberRole[orgID] != "owner" && !principal.Has("admin:roles_grant") {
			fields["member_role"] = "Only an owner can add another owner."
		}
	default:
		fields["member_role"] = "Choose owner, admin, member, or field_worker."
	}
	if invalid(fields) {
		return httpx.Validation(fields)
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var userID *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT app_user_id_by_email($1)`, email).Scan(&userID)
		if err != nil {
			return err
		}
		if userID == nil {
			return httpx.New(404, "user_not_found", "No account uses that email yet.")
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, member_role)
			VALUES ($1, $2, $3)
			ON CONFLICT (organization_id, user_id) DO UPDATE
			SET member_role = EXCLUDED.member_role, status = 'active'
			WHERE organization_members.status = 'removed'
		`, orgID, *userID, role)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(409, "already_member", "That person is already a member.")
		}
		if err := identity.AssignRole(ctx, tx, *userID, roleCode(role), &orgID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "organization.member_added", ResourceType: "organization_member",
			ResourceID: userID, TenantID: &orgID, RequestID: in.RequestID,
			Metadata: map[string]any{"member_role": role},
		}); err != nil {
			return err
		}
		return events.Enqueue(ctx, tx, "organization", orgID, "organization.member_added", map[string]any{
			"organization_id": orgID.String(), "user_id": userID.String(), "member_role": role, "request_id": in.RequestID,
		})
	})
}

func (s *Service) RemoveMember(ctx context.Context, principal authn.Principal, orgID, memberID uuid.UUID, requestID string) error {
	if !principal.CanReadOrg(orgID) {
		return errNotFound
	}
	if !principal.CanManageOrg(orgID) {
		return errForbidden
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(ctx, `
			SELECT member_role FROM organization_members
			WHERE organization_id = $1 AND user_id = $2 AND status = 'active'
		`, orgID, memberID).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if role == "owner" {
			var remaining int
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM organization_members
				WHERE organization_id = $1 AND member_role = 'owner' AND status = 'active' AND user_id <> $2
			`, orgID, memberID).Scan(&remaining); err != nil {
				return err
			}
			if remaining == 0 {
				return httpx.New(409, "last_owner", "An organization needs at least one owner.")
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE organization_members SET status = 'removed' WHERE organization_id = $1 AND user_id = $2
		`, orgID, memberID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_role_assignments WHERE user_id = $1 AND organization_id = $2
		`, memberID, orgID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "organization.member_removed", ResourceType: "organization_member",
			ResourceID: &memberID, TenantID: &orgID, RequestID: requestID, Metadata: map[string]any{"member_role": role},
		})
	})
}

func (s *Service) CreateDepartment(ctx context.Context, principal authn.Principal, orgID uuid.UUID, name, requestID string) (views.Department, error) {
	if err := s.manage(principal, orgID); err != nil {
		return views.Department{}, err
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 80 {
		return views.Department{}, httpx.Validation(map[string]string{"name": "Enter a department name between 2 and 80 characters."})
	}
	var department views.Department
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO departments (organization_id, name) VALUES ($1, $2) RETURNING id::text, name
		`, orgID, name).Scan(&department.ID, &department.Name)
		if err != nil {
			return err
		}
		departmentID, err := uuid.Parse(department.ID)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "organization.department_created", ResourceType: "department",
			ResourceID: &departmentID, TenantID: &orgID, RequestID: requestID,
		})
	})
	return department, err
}

func (s *Service) Departments(ctx context.Context, principal authn.Principal, orgID uuid.UUID) ([]views.Department, error) {
	if !principal.CanReadOrg(orgID) {
		return nil, errNotFound
	}
	var departments []views.Department
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, name FROM departments WHERE organization_id = $1 ORDER BY name`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var department views.Department
			if err := rows.Scan(&department.ID, &department.Name); err != nil {
				return err
			}
			departments = append(departments, department)
		}
		return rows.Err()
	})
	if departments == nil {
		departments = []views.Department{}
	}
	return departments, err
}

func (s *Service) CreateTeam(ctx context.Context, principal authn.Principal, orgID uuid.UUID, name string, departmentID *uuid.UUID, requestID string) (views.Team, error) {
	if err := s.manage(principal, orgID); err != nil {
		return views.Team{}, err
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 80 {
		return views.Team{}, httpx.Validation(map[string]string{"name": "Enter a team name between 2 and 80 characters."})
	}
	var team views.Team
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		if departmentID != nil {
			var owner uuid.UUID
			err := tx.QueryRow(ctx, `SELECT organization_id FROM departments WHERE id = $1`, *departmentID).Scan(&owner)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != orgID) {
				return httpx.Validation(map[string]string{"department_id": "That department is not in this organization."})
			}
			if err != nil {
				return err
			}
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO teams (organization_id, department_id, name) VALUES ($1, $2, $3)
			RETURNING id::text, name, department_id::text
		`, orgID, departmentID, name).Scan(&team.ID, &team.Name, &team.DepartmentID)
		if err != nil {
			return err
		}
		teamID, err := uuid.Parse(team.ID)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "organization.team_created", ResourceType: "team",
			ResourceID: &teamID, TenantID: &orgID, RequestID: requestID,
		})
	})
	return team, err
}

func (s *Service) manage(principal authn.Principal, orgID uuid.UUID) error {
	if !principal.CanReadOrg(orgID) {
		return errNotFound
	}
	if !principal.CanManageOrg(orgID) {
		return errForbidden
	}
	return nil
}

func roleCode(memberRole string) string {
	switch memberRole {
	case "owner", "admin":
		return "org_admin"
	case "field_worker":
		return "field_worker"
	default:
		return "org_member"
	}
}

func invalid(fields map[string]string) bool {
	for _, value := range fields {
		if value != "" {
			return true
		}
	}
	return false
}
