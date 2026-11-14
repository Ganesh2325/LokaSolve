-- Issue extraction support. Legacy tables stay. Issue-owned rows are not deleted.
-- Authority, review, and assignment rows may reference an issue UUID that now
-- lives in the Issue Service, so those foreign keys are removed.

ALTER TABLE outbox_events
  ADD COLUMN IF NOT EXISTS schema_version INT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS outbox_idempotency_idx
  ON outbox_events (idempotency_key) WHERE idempotency_key IS NOT NULL;

ALTER TABLE issue_events DROP CONSTRAINT IF EXISTS issue_events_issue_id_fkey;
ALTER TABLE review_items DROP CONSTRAINT IF EXISTS review_items_issue_id_fkey;
ALTER TABLE assignments DROP CONSTRAINT IF EXISTS assignments_issue_id_fkey;
ALTER TABLE incident_links DROP CONSTRAINT IF EXISTS incident_links_issue_id_fkey;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_issue_id_fkey;
ALTER TABLE message_attempts DROP CONSTRAINT IF EXISTS message_attempts_issue_id_fkey;
ALTER TABLE digital_actions DROP CONSTRAINT IF EXISTS digital_actions_issue_id_fkey;
ALTER TABLE referrals DROP CONSTRAINT IF EXISTS referrals_issue_id_fkey;

CREATE TABLE IF NOT EXISTS issue_projection (
  id UUID PRIMARY KEY,
  reporter_user_id UUID NOT NULL,
  description TEXT NOT NULL,
  category_label TEXT,
  locality_text TEXT,
  address_text TEXT,
  identity_visible_to_resolver BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL,
  follow_public BOOLEAN NOT NULL DEFAULT false,
  severity TEXT NOT NULL DEFAULT 'P3',
  severity_source TEXT NOT NULL DEFAULT 'unclassified',
  severity_reason TEXT NOT NULL DEFAULT '',
  safety_guidance TEXT,
  escalation_due_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO issue_projection (
  id, reporter_user_id, description, category_label, locality_text, address_text,
  identity_visible_to_resolver, status, follow_public, severity, severity_source,
  severity_reason, safety_guidance, escalation_due_at, created_at, updated_at
)
SELECT id, reporter_user_id, description, category_label, locality_text, address_text,
  identity_visible_to_resolver, status, follow_public, severity, severity_source,
  severity_reason, safety_guidance, escalation_due_at, created_at, updated_at
FROM issues
ON CONFLICT (id) DO NOTHING;

GRANT SELECT, INSERT, UPDATE ON issue_projection TO lokasolve_app;

CREATE OR REPLACE FUNCTION app_is_issue_reporter(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1
    FROM (
      SELECT reporter_user_id FROM issues WHERE id = target
      UNION ALL
      SELECT reporter_user_id FROM issue_projection WHERE id = target
    ) rows
    WHERE reporter_user_id = app_user_id()
  )
$$;

CREATE OR REPLACE FUNCTION app_issue_in_jurisdiction(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  WITH chosen AS (
    SELECT locality_text
    FROM (
      SELECT locality_text, updated_at, 1 AS preference FROM issues WHERE id = target
      UNION ALL
      SELECT locality_text, updated_at, 0 FROM issue_projection WHERE id = target
    ) rows
    ORDER BY updated_at DESC, preference DESC
    LIMIT 1
  )
  SELECT EXISTS (
    SELECT 1
    FROM chosen c
    WHERE c.locality_text IS NOT NULL
      AND (
        EXISTS (
          SELECT 1
          FROM organization_members m
          JOIN organizations o ON o.id = m.organization_id AND o.status = 'active'
            AND o.org_type IN ('municipality', 'utility', 'institution')
          JOIN organization_jurisdictions j ON j.organization_id = o.id
          WHERE m.user_id = app_user_id() AND m.status = 'active'
            AND lower(btrim(c.locality_text)) = lower(btrim(j.locality_text))
        )
        OR EXISTS (
          SELECT 1
          FROM identity_member_projection m
          JOIN identity_org_projection o ON o.id = m.organization_id AND o.status = 'active'
            AND o.org_type IN ('municipality', 'utility', 'institution')
          JOIN organization_jurisdictions j ON j.organization_id = o.id
          WHERE m.user_id = app_user_id() AND m.status = 'active'
            AND lower(btrim(c.locality_text)) = lower(btrim(j.locality_text))
        )
      )
  )
$$;

CREATE OR REPLACE FUNCTION issue_visible(target uuid)
RETURNS TABLE (
  id uuid,
  reporter_user_id uuid,
  description text,
  category_label text,
  locality_text text,
  address_text text,
  identity_visible_to_resolver boolean,
  status text,
  follow_public boolean,
  severity text,
  severity_source text,
  severity_reason text,
  safety_guidance text,
  escalation_due_at timestamptz,
  created_at timestamptz,
  updated_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  WITH chosen AS (
    SELECT *
    FROM (
      SELECT id, reporter_user_id, description, category_label, locality_text, address_text,
        identity_visible_to_resolver, status, follow_public, severity, severity_source,
        severity_reason, safety_guidance, escalation_due_at, created_at, updated_at, 1 AS preference
      FROM issues WHERE issues.id = target
      UNION ALL
      SELECT id, reporter_user_id, description, category_label, locality_text, address_text,
        identity_visible_to_resolver, status, follow_public, severity, severity_source,
        severity_reason, safety_guidance, escalation_due_at, created_at, updated_at, 0
      FROM issue_projection WHERE issue_projection.id = target
    ) rows
    ORDER BY updated_at DESC, preference DESC
    LIMIT 1
  )
  SELECT c.id, c.reporter_user_id, c.description, c.category_label, c.locality_text, c.address_text,
    c.identity_visible_to_resolver, c.status, c.follow_public, c.severity, c.severity_source,
    c.severity_reason, c.safety_guidance, c.escalation_due_at, c.created_at, c.updated_at
  FROM chosen c
  WHERE COALESCE(app_flag('app.platform_admin'), false)
    OR c.reporter_user_id = app_user_id()
    OR app_is_issue_assignee(target)
    OR app_issue_in_jurisdiction(target)
$$;

GRANT EXECUTE ON FUNCTION issue_visible(uuid) TO lokasolve_app;

CREATE OR REPLACE FUNCTION issue_access_grant(p_user uuid, p_issue uuid, p_locality text)
RETURNS TABLE (assignee boolean, jurisdiction boolean)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT
    EXISTS (
      SELECT 1 FROM assignments a
      JOIN resolvers r ON r.id = a.resolver_id
      WHERE a.issue_id = p_issue AND r.owner_user_id = p_user AND a.status <> 'rejected'
    ),
    EXISTS (
      SELECT 1
      WHERE p_locality IS NOT NULL AND btrim(p_locality) <> ''
        AND (
          EXISTS (
            SELECT 1
            FROM organization_members m
            JOIN organizations o ON o.id = m.organization_id AND o.status = 'active'
              AND o.org_type IN ('municipality', 'utility', 'institution')
            JOIN organization_jurisdictions j ON j.organization_id = o.id
            WHERE m.user_id = p_user AND m.status = 'active'
              AND lower(btrim(p_locality)) = lower(btrim(j.locality_text))
          )
          OR EXISTS (
            SELECT 1
            FROM identity_member_projection m
            JOIN identity_org_projection o ON o.id = m.organization_id AND o.status = 'active'
              AND o.org_type IN ('municipality', 'utility', 'institution')
            JOIN organization_jurisdictions j ON j.organization_id = o.id
            WHERE m.user_id = p_user AND m.status = 'active'
              AND lower(btrim(p_locality)) = lower(btrim(j.locality_text))
          )
        )
    )
$$;

GRANT EXECUTE ON FUNCTION issue_access_grant(uuid, uuid, text) TO lokasolve_app;

DROP POLICY IF EXISTS issue_events_insert ON issue_events;
CREATE POLICY issue_events_insert ON issue_events FOR INSERT WITH CHECK (
  actor_user_id = app_user_id() AND (
    app_flag('app.platform_admin')
    OR app_is_issue_reporter(issue_id)
    OR app_is_issue_assignee(issue_id)
  )
);

DROP POLICY IF EXISTS notifications_insert ON notifications;
CREATE POLICY notifications_insert ON notifications FOR INSERT WITH CHECK (
  user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
);

DROP POLICY IF EXISTS message_attempts_insert ON message_attempts;
CREATE POLICY message_attempts_insert ON message_attempts FOR INSERT WITH CHECK (
  user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
);

DROP POLICY IF EXISTS review_select ON review_items;
CREATE POLICY review_select ON review_items FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR app_is_issue_reporter(issue_id)
);
DROP POLICY IF EXISTS review_insert ON review_items;
CREATE POLICY review_insert ON review_items FOR INSERT WITH CHECK (
  app_flag('app.platform_admin') OR app_is_issue_reporter(issue_id)
);
DROP POLICY IF EXISTS review_update ON review_items;
CREATE POLICY review_update ON review_items FOR UPDATE USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_is_issue_reporter(issue_id)
) WITH CHECK (
  app_flag('app.platform_admin') OR app_flag('app.platform_read') OR app_is_issue_reporter(issue_id)
);

DROP POLICY IF EXISTS assignments_select ON assignments;
CREATE POLICY assignments_select ON assignments FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR app_is_issue_reporter(issue_id)
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
);
-- Drop every remaining foreign key to issues except the tables this
-- database stops writing. Media and severity history stay issue-owned.
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
      AND dst.relname = 'issues'
      AND src.relname NOT IN ('media_objects', 'severity_events')
  LOOP
    EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', r.table_name, r.conname);
  END LOOP;
END $$;

DROP POLICY IF EXISTS assignments_update ON assignments;
CREATE POLICY assignments_update ON assignments FOR UPDATE USING (
  app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
) WITH CHECK (
  app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
);
