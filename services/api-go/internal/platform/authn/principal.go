package authn

import "github.com/google/uuid"

type Principal struct {
	UserID      uuid.UUID
	SessionID   uuid.UUID
	Status      string
	IsDemo      bool
	Locked      bool
	GlobalRoles map[string]struct{}
	Permissions map[string]struct{}
	MemberRole  map[uuid.UUID]string
}

func (p Principal) Has(permission string) bool {
	_, ok := p.Permissions[permission]
	return ok
}

func (p Principal) HasRole(role string) bool {
	_, ok := p.GlobalRoles[role]
	return ok
}

func (p Principal) CanReadOrg(id uuid.UUID) bool {
	if p.Has("admin:orgs_read") || p.Has("admin:roles_grant") {
		return true
	}
	_, ok := p.MemberRole[id]
	return ok
}

func (p Principal) CanManageOrg(id uuid.UUID) bool {
	if p.Has("admin:roles_grant") {
		return true
	}
	role := p.MemberRole[id]
	return role == "owner" || role == "admin"
}
