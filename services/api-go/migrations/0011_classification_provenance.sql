-- Routing reads provenance from the projection. A missing value stays unclassified.
-- Historical report rows are not deleted.

ALTER TABLE issue_projection ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION;
ALTER TABLE issue_projection ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;
ALTER TABLE issue_projection ADD COLUMN IF NOT EXISTS classification_provenance TEXT NOT NULL DEFAULT 'UNCLASSIFIED';
ALTER TABLE issue_projection ADD COLUMN IF NOT EXISTS suggested_label TEXT;

ALTER TABLE issue_projection DROP CONSTRAINT IF EXISTS issue_projection_provenance;
ALTER TABLE issue_projection ADD CONSTRAINT issue_projection_provenance
  CHECK (classification_provenance IN (
    'USER_SELECTED_VALIDATED', 'RULE_SUGGESTED', 'AI_SUGGESTED', 'HUMAN_VERIFIED', 'UNCLASSIFIED'
  ));

UPDATE issue_projection
SET classification_provenance = 'RULE_SUGGESTED',
    suggested_label = COALESCE(suggested_label, category_label)
WHERE classification_provenance = 'UNCLASSIFIED'
  AND category_label IN ('Lighting', 'Roads', 'Water', 'Drainage', 'Power', 'Waste');

DROP FUNCTION IF EXISTS issue_visible(uuid);

CREATE FUNCTION issue_visible(target uuid)
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
  updated_at timestamptz,
  latitude double precision,
  longitude double precision,
  classification_provenance text,
  suggested_label text
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
        severity_reason, safety_guidance, escalation_due_at, created_at, updated_at,
        latitude, longitude, 'UNCLASSIFIED'::text AS classification_provenance, NULL::text AS suggested_label,
        1 AS preference
      FROM issues WHERE issues.id = target
      UNION ALL
      SELECT id, reporter_user_id, description, category_label, locality_text, address_text,
        identity_visible_to_resolver, status, follow_public, severity, severity_source,
        severity_reason, safety_guidance, escalation_due_at, created_at, updated_at,
        latitude, longitude, classification_provenance, suggested_label,
        0
      FROM issue_projection WHERE issue_projection.id = target
    ) rows
    ORDER BY updated_at DESC, preference DESC
    LIMIT 1
  )
  SELECT c.id, c.reporter_user_id, c.description, c.category_label, c.locality_text, c.address_text,
    c.identity_visible_to_resolver, c.status, c.follow_public, c.severity, c.severity_source,
    c.severity_reason, c.safety_guidance, c.escalation_due_at, c.created_at, c.updated_at,
    c.latitude, c.longitude, c.classification_provenance, c.suggested_label
  FROM chosen c
  WHERE COALESCE(app_flag('app.platform_admin'), false)
    OR c.reporter_user_id = app_user_id()
    OR app_is_issue_assignee(target)
    OR app_issue_in_jurisdiction(target)
$$;

GRANT EXECUTE ON FUNCTION issue_visible(uuid) TO lokasolve_app;
