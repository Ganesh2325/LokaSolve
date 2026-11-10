package identity

import "lokasolve/services/api-go/internal/platform/authn"

func AccessFlags(principal authn.Principal) []string {
	var flags []string
	if principal.Has("admin:roles_grant") {
		flags = append(flags, "app.platform_admin")
	}
	if principal.Has("admin:orgs_read") || principal.Has("admin:users_read") || principal.Has("admin:audit_read") {
		flags = append(flags, "app.platform_read")
	}
	return flags
}
