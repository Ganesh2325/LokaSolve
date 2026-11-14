-- Product tables keep user and organization UUIDs and drop the foreign keys.
-- Identity rows stay in place. Nothing in this migration deletes a report or an account.

DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.conname, src.relname AS table_name
    FROM pg_constraint c
    JOIN pg_class src ON src.oid = c.conrelid
    JOIN pg_class dst ON dst.oid = c.confrelid
    JOIN pg_namespace n ON n.oid = src.relnamespace
    WHERE c.contype = 'f'
      AND n.nspname = 'public'
      AND dst.relname IN ('users', 'organizations')
      AND src.relname NOT IN (
        'user_profiles', 'user_contacts', 'organization_members', 'departments', 'teams',
        'user_role_assignments', 'locations', 'sessions', 'otp_challenges', 'privacy_requests'
      )
  LOOP
    EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', r.table_name, r.conname);
  END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS identity_org_projection (
  id UUID PRIMARY KEY,
  org_type TEXT NOT NULL,
  status TEXT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_member_projection (
  organization_id UUID NOT NULL,
  user_id UUID NOT NULL,
  member_role TEXT NOT NULL,
  status TEXT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, user_id)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON identity_org_projection, identity_member_projection TO lokasolve_app;

CREATE OR REPLACE FUNCTION app_issue_in_jurisdiction(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1
    FROM issues i
    WHERE i.id = target
      AND i.locality_text IS NOT NULL
      AND (
        EXISTS (
          SELECT 1
          FROM organization_members m
          JOIN organizations o ON o.id = m.organization_id AND o.status = 'active'
            AND o.org_type IN ('municipality', 'utility', 'institution')
          JOIN organization_jurisdictions j ON j.organization_id = o.id
          WHERE m.user_id = app_user_id() AND m.status = 'active'
            AND lower(btrim(i.locality_text)) = lower(btrim(j.locality_text))
        )
        OR EXISTS (
          SELECT 1
          FROM identity_member_projection m
          JOIN identity_org_projection o ON o.id = m.organization_id AND o.status = 'active'
            AND o.org_type IN ('municipality', 'utility', 'institution')
          JOIN organization_jurisdictions j ON j.organization_id = o.id
          WHERE m.user_id = app_user_id() AND m.status = 'active'
            AND lower(btrim(i.locality_text)) = lower(btrim(j.locality_text))
        )
      )
  )
$$;

GRANT EXECUTE ON FUNCTION app_issue_in_jurisdiction(uuid) TO lokasolve_app;
