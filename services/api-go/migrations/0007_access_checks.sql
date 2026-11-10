-- The access policies in 0006 queried assignments while assignment policies
-- queried issues. Postgres rejects that cycle. These functions read the check
-- as the migration owner and return only a boolean for the current user.

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

DROP POLICY IF EXISTS issues_assignee_select ON issues;
CREATE POLICY issues_assignee_select ON issues FOR SELECT
  USING (app_is_issue_assignee(id));

DROP POLICY IF EXISTS issues_officer_select ON issues;
CREATE POLICY issues_officer_select ON issues FOR SELECT
  USING (app_issue_in_jurisdiction(id));

DROP POLICY IF EXISTS issue_events_select ON issue_events;
CREATE POLICY issue_events_select ON issue_events FOR SELECT USING (
  app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
  OR app_issue_in_jurisdiction(issue_id)
);

DROP POLICY IF EXISTS media_select ON media_objects;
CREATE POLICY media_select ON media_objects FOR SELECT USING (
  owner_user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
  OR app_issue_in_jurisdiction(issue_id)
);
