-- Stage 1 identity, tenancy, audit, and outbox.
-- Applied by the superuser migration connection. The application role is created
-- here only when it does not already exist, so production can provision the role
-- and password out of band before migrations run.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS postgis;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lokasolve_app') THEN
    CREATE ROLE lokasolve_app LOGIN PASSWORD 'lokasolve_app'
      NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
  END IF;
END
$$;

ALTER ROLE lokasolve_app SET statement_timeout = '15s';
ALTER ROLE lokasolve_app SET idle_in_transaction_session_timeout = '15s';

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE,
  phone_e164 TEXT UNIQUE,
  password_hash TEXT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'locked', 'pending_deletion', 'deleted')),
  email_verified_at TIMESTAMPTZ,
  phone_verified_at TIMESTAMPTZ,
  failed_login_attempts INT NOT NULL DEFAULT 0,
  locked_until TIMESTAMPTZ,
  is_demo BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_email_lowercase CHECK (email IS NULL OR email = lower(email)),
  CONSTRAINT users_has_login CHECK (email IS NOT NULL OR phone_e164 IS NOT NULL)
);

CREATE TABLE user_profiles (
  user_id UUID PRIMARY KEY REFERENCES users (id),
  display_name TEXT NOT NULL,
  preferred_language TEXT NOT NULL DEFAULT 'en',
  notification_preferences JSONB NOT NULL DEFAULT '{"in_app": true, "email": true, "sms": false, "whatsapp": false}'::jsonb,
  privacy_preferences JSONB NOT NULL DEFAULT '{"public_identity": false}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_contacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id),
  channel TEXT NOT NULL CHECK (channel IN ('email', 'sms', 'whatsapp')),
  value TEXT NOT NULL,
  verified_at TIMESTAMPTZ,
  opted_in_at TIMESTAMPTZ,
  opted_out_at TIMESTAMPTZ,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (channel, value)
);

CREATE TABLE roles (
  id UUID PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  is_system BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE permissions (
  id UUID PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL
);

CREATE TABLE role_permissions (
  role_id UUID NOT NULL REFERENCES roles (id),
  permission_id UUID NOT NULL REFERENCES permissions (id),
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  org_type TEXT NOT NULL CHECK (org_type IN (
    'municipality', 'utility', 'business', 'ngo', 'institution', 'service_provider', 'other'
  )),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
  is_demo BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE organization_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations (id),
  user_id UUID NOT NULL REFERENCES users (id),
  member_role TEXT NOT NULL CHECK (member_role IN ('owner', 'admin', 'member', 'field_worker')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'removed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, user_id)
);

CREATE TABLE departments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations (id),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, name)
);

CREATE TABLE teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations (id),
  department_id UUID REFERENCES departments (id),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, name)
);

CREATE TABLE user_role_assignments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id),
  role_id UUID NOT NULL REFERENCES roles (id),
  organization_id UUID REFERENCES organizations (id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_role_assignments_global_uq
  ON user_role_assignments (user_id, role_id)
  WHERE organization_id IS NULL;

CREATE UNIQUE INDEX user_role_assignments_org_uq
  ON user_role_assignments (user_id, role_id, organization_id)
  WHERE organization_id IS NOT NULL;

CREATE INDEX user_role_assignments_user_idx ON user_role_assignments (user_id);

CREATE TABLE locations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id),
  label TEXT,
  address_text TEXT,
  locality_text TEXT,
  latitude DOUBLE PRECISION,
  longitude DOUBLE PRECISION,
  geom geography(Point, 4326),
  source TEXT NOT NULL CHECK (source IN ('user_gps', 'user_pin', 'user_address', 'user_locality')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT locations_lat_lng_pair CHECK (
    (latitude IS NULL AND longitude IS NULL)
    OR (latitude IS NOT NULL AND longitude IS NOT NULL)
  ),
  CONSTRAINT locations_lat_range CHECK (latitude IS NULL OR (latitude >= -90 AND latitude <= 90)),
  CONSTRAINT locations_lng_range CHECK (longitude IS NULL OR (longitude >= -180 AND longitude <= 180))
);

CREATE INDEX locations_user_idx ON locations (user_id);
CREATE INDEX locations_geom_gix ON locations USING GIST (geom);

CREATE TABLE sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id),
  refresh_token_hash TEXT NOT NULL UNIQUE,
  user_agent TEXT,
  ip_address TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  replaced_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE otp_challenges (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  destination TEXT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('sms')),
  code_hash TEXT NOT NULL,
  purpose TEXT NOT NULL CHECK (purpose IN ('login', 'verify_phone')),
  user_id UUID REFERENCES users (id),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  attempts INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX otp_challenges_open_idx
  ON otp_challenges (destination, purpose, created_at DESC)
  WHERE consumed_at IS NULL;

CREATE TABLE idempotency_keys (
  key TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  user_id UUID,
  response_status INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key, method, path)
);

CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID,
  actor_role TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id UUID,
  tenant_id UUID,
  request_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id);

CREATE TABLE outbox_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type TEXT NOT NULL,
  aggregate_id UUID NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  attempts INT NOT NULL DEFAULT 0,
  last_error TEXT
);

CREATE INDEX outbox_unpublished_idx
  ON outbox_events (occurred_at)
  WHERE published_at IS NULL;

CREATE TABLE privacy_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id),
  request_type TEXT NOT NULL CHECK (request_type IN ('deletion', 'export', 'correction')),
  status TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'in_review', 'completed', 'rejected')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX privacy_requests_user_idx ON privacy_requests (user_id, created_at DESC);

CREATE TABLE feature_flags (
  key TEXT PRIMARY KEY,
  enabled BOOLEAN NOT NULL DEFAULT false,
  description TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER user_profiles_set_updated_at BEFORE UPDATE ON user_profiles
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER user_contacts_set_updated_at BEFORE UPDATE ON user_contacts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER organizations_set_updated_at BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER organization_members_set_updated_at BEFORE UPDATE ON organization_members
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER departments_set_updated_at BEFORE UPDATE ON departments
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER teams_set_updated_at BEFORE UPDATE ON teams
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER privacy_requests_set_updated_at BEFORE UPDATE ON privacy_requests
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER feature_flags_set_updated_at BEFORE UPDATE ON feature_flags
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO roles (id, code, name, description) VALUES
  ('10000000-0000-4000-8000-000000000001', 'resident', 'Resident', 'Reports problems and manages their own account.'),
  ('10000000-0000-4000-8000-000000000002', 'provider', 'Service provider', 'Operates a service profile and receives eligible work.'),
  ('10000000-0000-4000-8000-000000000003', 'org_member', 'Organization member', 'Reads cases and work inside one organization.'),
  ('10000000-0000-4000-8000-000000000004', 'org_admin', 'Organization admin', 'Manages members and structure for one organization.'),
  ('10000000-0000-4000-8000-000000000005', 'field_worker', 'Field worker', 'Sees and updates work assigned to them.'),
  ('10000000-0000-4000-8000-000000000006', 'support_agent', 'Support agent', 'Reviews accounts, audit, and operational queues.'),
  ('10000000-0000-4000-8000-000000000007', 'super_admin', 'Super administrator', 'Manages platform configuration and exceptional access.')
ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (id, code, description) VALUES
  ('20000000-0000-4000-8000-000000000001', 'profile:read', 'Read the signed-in profile.'),
  ('20000000-0000-4000-8000-000000000002', 'profile:update', 'Update the signed-in profile.'),
  ('20000000-0000-4000-8000-000000000003', 'locations:manage', 'Manage the signed-in user saved places.'),
  ('20000000-0000-4000-8000-000000000004', 'org:create', 'Create an organization.'),
  ('20000000-0000-4000-8000-000000000005', 'org:read', 'Read one organization the user belongs to.'),
  ('20000000-0000-4000-8000-000000000006', 'org:manage_members', 'Manage members of one organization.'),
  ('20000000-0000-4000-8000-000000000007', 'org:manage_structure', 'Manage departments and teams of one organization.'),
  ('20000000-0000-4000-8000-000000000008', 'provider:profile', 'Manage a provider profile.'),
  ('20000000-0000-4000-8000-000000000009', 'issues:create', 'Create a report.'),
  ('20000000-0000-4000-8000-000000000010', 'issues:read_own', 'Read the signed-in user reports.'),
  ('20000000-0000-4000-8000-000000000011', 'assignments:read_own', 'Read work assigned to the signed-in user.'),
  ('20000000-0000-4000-8000-000000000012', 'admin:users_read', 'List platform users.'),
  ('20000000-0000-4000-8000-000000000013', 'admin:audit_read', 'Read the audit log.'),
  ('20000000-0000-4000-8000-000000000014', 'admin:orgs_read', 'Read organizations across tenants.'),
  ('20000000-0000-4000-8000-000000000015', 'admin:roles_grant', 'Grant platform roles.'),
  ('20000000-0000-4000-8000-000000000016', 'admin:taxonomy_manage', 'Manage taxonomy and workflow configuration.')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON (
  (r.code = 'resident' AND p.code IN (
    'profile:read', 'profile:update', 'locations:manage', 'org:create', 'issues:create', 'issues:read_own'
  ))
  OR (r.code = 'provider' AND p.code IN ('provider:profile', 'assignments:read_own'))
  OR (r.code = 'org_member' AND p.code IN ('org:read'))
  OR (r.code = 'org_admin' AND p.code IN ('org:read', 'org:manage_members', 'org:manage_structure'))
  OR (r.code = 'field_worker' AND p.code IN ('org:read', 'assignments:read_own'))
  OR (r.code = 'support_agent' AND p.code IN (
    'profile:read', 'admin:users_read', 'admin:audit_read', 'admin:orgs_read'
  ))
  OR (r.code = 'super_admin' AND p.code IN (
    'profile:read', 'profile:update', 'locations:manage', 'org:create', 'org:read',
    'org:manage_members', 'org:manage_structure', 'provider:profile', 'issues:create',
    'issues:read_own', 'assignments:read_own', 'admin:users_read', 'admin:audit_read',
    'admin:orgs_read', 'admin:roles_grant', 'admin:taxonomy_manage'
  ))
)
ON CONFLICT DO NOTHING;

INSERT INTO feature_flags (key, enabled, description) VALUES
  ('org_self_service', true, 'Signed-in users may create an organization and become its owner.'),
  ('otp_login', true, 'Phone OTP login can be used when a sender is configured.'),
  ('ai_intake', false, 'Model-backed intake stays off until a provider and evaluation set exist.')
ON CONFLICT (key) DO NOTHING;

-- Row-level security helpers. Custom GUCs are defense in depth for application
-- queries. A connection that can execute arbitrary SQL as lokasolve_app can set
-- the same GUCs; keep that password server-side and use parameterized queries.

CREATE OR REPLACE FUNCTION app_user_id()
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
  SELECT NULLIF(current_setting('app.user_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION app_flag(flag text)
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
  SELECT current_setting(flag, true) = 'on'
$$;

CREATE OR REPLACE FUNCTION app_is_org_member(org uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT org IS NOT NULL AND EXISTS (
    SELECT 1
    FROM organization_members m
    WHERE m.organization_id = org
      AND m.user_id = app_user_id()
      AND m.status = 'active'
  );
$$;

CREATE OR REPLACE FUNCTION app_is_org_manager(org uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT org IS NOT NULL AND EXISTS (
    SELECT 1
    FROM organization_members m
    WHERE m.organization_id = org
      AND m.user_id = app_user_id()
      AND m.member_role IN ('owner', 'admin')
      AND m.status = 'active'
  );
$$;

CREATE OR REPLACE FUNCTION app_user_id_by_email(p_email text)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT id FROM users WHERE email = lower(p_email) AND status <> 'deleted'
$$;

CREATE OR REPLACE FUNCTION app_org_member_directory(org uuid)
RETURNS TABLE (user_id uuid, email text, display_name text, member_role text, status text)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT u.id, u.email, p.display_name, m.member_role, m.status
  FROM organization_members m
  JOIN users u ON u.id = m.user_id
  JOIN user_profiles p ON p.user_id = u.id
  WHERE m.organization_id = org
    AND m.status = 'active'
    AND (
      app_flag('app.platform_admin')
      OR app_flag('app.platform_read')
      OR app_is_org_member(org)
    )
  ORDER BY p.display_name;
$$;

REVOKE ALL ON FUNCTION app_user_id() FROM PUBLIC;
REVOKE ALL ON FUNCTION app_org_member_directory(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_flag(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_is_org_member(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_is_org_manager(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_user_id_by_email(text) FROM PUBLIC;

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE user_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_profiles FORCE ROW LEVEL SECURITY;
ALTER TABLE user_contacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_contacts FORCE ROW LEVEL SECURITY;
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE organization_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_members FORCE ROW LEVEL SECURITY;
ALTER TABLE departments ENABLE ROW LEVEL SECURITY;
ALTER TABLE departments FORCE ROW LEVEL SECURITY;
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams FORCE ROW LEVEL SECURITY;
ALTER TABLE user_role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_role_assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
ALTER TABLE otp_challenges ENABLE ROW LEVEL SECURITY;
ALTER TABLE otp_challenges FORCE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;
ALTER TABLE privacy_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE privacy_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE feature_flags ENABLE ROW LEVEL SECURITY;
ALTER TABLE feature_flags FORCE ROW LEVEL SECURITY;

CREATE POLICY users_select ON users FOR SELECT
  USING (
    id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_flag('app.platform_read')
    OR app_flag('app.auth_lookup')
  );
CREATE POLICY users_insert ON users FOR INSERT
  WITH CHECK (app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));
CREATE POLICY users_update ON users FOR UPDATE
  USING (id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'))
  WITH CHECK (id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));

CREATE POLICY profiles_select ON user_profiles FOR SELECT
  USING (user_id = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_flag('app.auth_lookup'));
CREATE POLICY profiles_insert ON user_profiles FOR INSERT
  WITH CHECK (app_flag('app.auth_lookup') OR user_id = app_user_id() OR app_flag('app.platform_admin'));
CREATE POLICY profiles_update ON user_profiles FOR UPDATE
  USING (user_id = app_user_id() OR app_flag('app.platform_admin'))
  WITH CHECK (user_id = app_user_id() OR app_flag('app.platform_admin'));

CREATE POLICY contacts_all ON user_contacts FOR ALL
  USING (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin') OR app_flag('app.platform_read'))
  WITH CHECK (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));

CREATE POLICY organizations_select ON organizations FOR SELECT
  USING (app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_is_org_member(id));
CREATE POLICY organizations_insert ON organizations FOR INSERT
  WITH CHECK (app_flag('app.platform_admin') OR app_flag('app.org_create'));
CREATE POLICY organizations_update ON organizations FOR UPDATE
  USING (app_flag('app.platform_admin') OR app_is_org_manager(id))
  WITH CHECK (app_flag('app.platform_admin') OR app_is_org_manager(id));

CREATE POLICY members_select ON organization_members FOR SELECT
  USING (
    app_flag('app.platform_admin')
    OR app_flag('app.platform_read')
    OR user_id = app_user_id()
    OR app_is_org_member(organization_id)
  );
CREATE POLICY members_insert ON organization_members FOR INSERT
  WITH CHECK (
    app_flag('app.platform_admin')
    OR (app_flag('app.org_bootstrap') AND user_id = app_user_id())
    OR app_is_org_manager(organization_id)
  );
CREATE POLICY members_update ON organization_members FOR UPDATE
  USING (app_flag('app.platform_admin') OR app_is_org_manager(organization_id))
  WITH CHECK (app_flag('app.platform_admin') OR app_is_org_manager(organization_id));

CREATE POLICY departments_select ON departments FOR SELECT
  USING (app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_is_org_member(organization_id));
CREATE POLICY departments_write ON departments FOR INSERT
  WITH CHECK (app_flag('app.platform_admin') OR app_is_org_manager(organization_id));

CREATE POLICY teams_select ON teams FOR SELECT
  USING (app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_is_org_member(organization_id));
CREATE POLICY teams_write ON teams FOR INSERT
  WITH CHECK (app_flag('app.platform_admin') OR app_is_org_manager(organization_id));

CREATE POLICY roles_select ON user_role_assignments FOR SELECT
  USING (
    user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_flag('app.platform_read')
    OR app_flag('app.auth_lookup')
    OR (organization_id IS NOT NULL AND app_is_org_manager(organization_id))
  );
CREATE POLICY roles_insert ON user_role_assignments FOR INSERT
  WITH CHECK (
    app_flag('app.platform_admin')
    OR app_flag('app.auth_lookup')
    OR (organization_id IS NOT NULL AND app_is_org_manager(organization_id))
  );
CREATE POLICY roles_delete ON user_role_assignments FOR DELETE
  USING (
    app_flag('app.platform_admin')
    OR (organization_id IS NOT NULL AND app_is_org_manager(organization_id))
  );

CREATE POLICY locations_all ON locations FOR ALL
  USING (user_id = app_user_id() OR app_flag('app.platform_admin'))
  WITH CHECK (user_id = app_user_id() OR app_flag('app.platform_admin'));

CREATE POLICY sessions_select ON sessions FOR SELECT
  USING (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));
CREATE POLICY sessions_insert ON sessions FOR INSERT
  WITH CHECK (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));
CREATE POLICY sessions_update ON sessions FOR UPDATE
  USING (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'))
  WITH CHECK (user_id = app_user_id() OR app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));

CREATE POLICY otp_all ON otp_challenges FOR ALL
  USING (app_flag('app.auth_lookup') OR app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.auth_lookup') OR app_flag('app.platform_admin'));

CREATE POLICY idempotency_all ON idempotency_keys FOR ALL
  USING (app_flag('app.auth_lookup') OR app_flag('app.platform_admin') OR user_id = app_user_id())
  WITH CHECK (app_flag('app.auth_lookup') OR app_flag('app.platform_admin') OR user_id = app_user_id());

CREATE POLICY audit_select ON audit_logs FOR SELECT
  USING (app_flag('app.platform_admin') OR app_flag('app.platform_read'));
CREATE POLICY audit_insert ON audit_logs FOR INSERT
  WITH CHECK (
    app_flag('app.auth_lookup')
    OR app_flag('app.platform_admin')
    OR app_flag('app.worker')
    OR actor_user_id = app_user_id()
  );

CREATE POLICY outbox_all ON outbox_events FOR ALL
  USING (app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_flag('app.worker') OR app_flag('app.auth_lookup'))
  WITH CHECK (
    app_flag('app.auth_lookup')
    OR app_flag('app.platform_admin')
    OR app_flag('app.worker')
    OR app_user_id() IS NOT NULL
  );

CREATE POLICY privacy_select ON privacy_requests FOR SELECT
  USING (user_id = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read'));
CREATE POLICY privacy_insert ON privacy_requests FOR INSERT
  WITH CHECK (user_id = app_user_id() OR app_flag('app.platform_admin'));

CREATE POLICY flags_read ON feature_flags FOR SELECT USING (true);
CREATE POLICY flags_write ON feature_flags FOR UPDATE
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

GRANT USAGE ON SCHEMA public TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO lokasolve_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_user_id() TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_flag(text) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_is_org_member(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_is_org_manager(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_user_id_by_email(text) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_org_member_directory(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION set_updated_at() TO lokasolve_app;

DO $$
BEGIN
  IF to_regclass('public.schema_migrations') IS NOT NULL THEN
    REVOKE INSERT, UPDATE, DELETE ON schema_migrations FROM lokasolve_app;
  END IF;
END
$$;
