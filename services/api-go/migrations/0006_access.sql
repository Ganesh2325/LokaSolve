-- Report rows stay. This migration removes the policy that let every signed-in
-- user read a public report's address, coordinates, and narrative. Rollback is
-- documented in docs/runbooks/t02-security-migration.md.

DROP POLICY issues_public_select ON issues;

DROP POLICY issues_select ON issues;
CREATE POLICY issues_select ON issues FOR SELECT
  USING (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
  );

-- Update and assignee policies are created after the check functions below.
-- Those functions avoid a policy cycle between issues and assignments.

CREATE TABLE organization_jurisdictions (
  organization_id uuid NOT NULL REFERENCES organizations (id),
  locality_text text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, locality_text),
  CONSTRAINT organization_jurisdictions_locality_len CHECK (char_length(btrim(locality_text)) BETWEEN 2 AND 120)
);

ALTER TABLE organization_jurisdictions ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_jurisdictions FORCE ROW LEVEL SECURITY;

CREATE POLICY organization_jurisdictions_select ON organization_jurisdictions FOR SELECT
  USING (
    app_flag('app.platform_admin')
    OR EXISTS (
      SELECT 1 FROM organization_members m
      WHERE m.organization_id = organization_jurisdictions.organization_id
        AND m.user_id = app_user_id()
        AND m.status = 'active'
    )
  );

CREATE POLICY organization_jurisdictions_write ON organization_jurisdictions FOR ALL
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

GRANT SELECT, INSERT, DELETE ON organization_jurisdictions TO lokasolve_app;

CREATE OR REPLACE FUNCTION app_is_issue_reporter(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1 FROM issues i
    WHERE i.id = target AND i.reporter_user_id = app_user_id()
  )
$$;

CREATE OR REPLACE FUNCTION app_is_issue_assignee(target uuid)
RETURNS boolean
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT EXISTS (
    SELECT 1 FROM assignments a
    JOIN resolvers r ON r.id = a.resolver_id
    WHERE a.issue_id = target
      AND r.owner_user_id = app_user_id()
      AND a.status <> 'rejected'
  )
$$;

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
    JOIN organization_members m ON m.user_id = app_user_id() AND m.status = 'active'
    JOIN organizations o ON o.id = m.organization_id AND o.status = 'active'
      AND o.org_type IN ('municipality', 'utility', 'institution')
    JOIN organization_jurisdictions j ON j.organization_id = o.id
    WHERE i.id = target
      AND i.locality_text IS NOT NULL
      AND lower(btrim(i.locality_text)) = lower(btrim(j.locality_text))
  )
$$;

REVOKE ALL ON FUNCTION app_is_issue_reporter(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_is_issue_assignee(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION app_issue_in_jurisdiction(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_is_issue_reporter(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_is_issue_assignee(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION app_issue_in_jurisdiction(uuid) TO lokasolve_app;

DROP POLICY IF EXISTS issues_update ON issues;
CREATE POLICY issues_update ON issues FOR UPDATE
  USING (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_is_issue_assignee(id)
  )
  WITH CHECK (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_is_issue_assignee(id)
  );

CREATE POLICY issues_assignee_select ON issues FOR SELECT
  USING (app_is_issue_assignee(id));

CREATE POLICY issues_officer_select ON issues FOR SELECT
  USING (app_issue_in_jurisdiction(id));

DROP POLICY issue_events_select ON issue_events;
CREATE POLICY issue_events_select ON issue_events FOR SELECT USING (
  app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
  OR app_issue_in_jurisdiction(issue_id)
);

DROP POLICY media_select ON media_objects;
CREATE POLICY media_select ON media_objects FOR SELECT USING (
  owner_user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
  OR app_issue_in_jurisdiction(issue_id)
);

ALTER TABLE media_objects ADD COLUMN malware_scan text NOT NULL DEFAULT 'not_scanned';
ALTER TABLE media_objects ADD CONSTRAINT media_objects_scan
  CHECK (malware_scan IN ('not_scanned', 'clean', 'rejected'));

CREATE OR REPLACE FUNCTION public_issue_card(target uuid)
RETURNS TABLE (
  id uuid,
  category_label text,
  locality_text text,
  status text,
  created_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT i.id, i.category_label, i.locality_text, i.status, i.created_at
  FROM issues i
  WHERE i.id = target AND i.follow_public
$$;

CREATE OR REPLACE FUNCTION public_issue_cards()
RETURNS TABLE (
  id uuid,
  category_label text,
  locality_text text,
  status text,
  created_at timestamptz
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT i.id, i.category_label, i.locality_text, i.status, i.created_at
  FROM issues i
  WHERE i.follow_public
  ORDER BY i.created_at DESC
  LIMIT 50
$$;

CREATE OR REPLACE FUNCTION issue_metrics()
RETURNS TABLE (
  reports bigint,
  open_reviews bigint,
  awaiting_acceptance bigint,
  awaiting_confirmation bigint,
  resolved bigint,
  older_than_48_hours bigint,
  public_nearby bigint,
  external_messages_ready bigint
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NOT (app_flag('app.platform_admin') OR app_flag('app.platform_read')) THEN
    RETURN QUERY SELECT 0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint;
    RETURN;
  END IF;
  RETURN QUERY
  SELECT
    (SELECT count(*) FROM issues),
    (SELECT count(*) FROM review_items WHERE status = 'open'),
    (SELECT count(*) FROM issues WHERE status = 'AWAITING_RESOLVER_ACCEPTANCE'),
    (SELECT count(*) FROM issues WHERE status = 'AWAITING_CONFIRMATION'),
    (SELECT count(*) FROM issues WHERE status = 'RESOLVED'),
    (SELECT count(*) FROM issues WHERE status IN ('SUBMITTED', 'NEEDS_INFORMATION', 'UNDER_REVIEW', 'ROUTING_PENDING') AND created_at < now() - interval '48 hours'),
    (SELECT count(*) FROM issues WHERE follow_public),
    (SELECT count(*) FROM message_attempts WHERE channel <> 'in_app' AND status = 'recorded');
END;
$$;

REVOKE ALL ON FUNCTION public_issue_card(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public_issue_cards() FROM PUBLIC;
REVOKE ALL ON FUNCTION issue_metrics() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_issue_card(uuid) TO lokasolve_app;
GRANT EXECUTE ON FUNCTION public_issue_cards() TO lokasolve_app;
GRANT EXECUTE ON FUNCTION issue_metrics() TO lokasolve_app;
