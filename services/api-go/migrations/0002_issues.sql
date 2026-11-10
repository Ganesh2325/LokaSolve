CREATE TABLE issues (
  id uuid PRIMARY KEY,
  reporter_user_id uuid NOT NULL REFERENCES users (id),
  description text NOT NULL,
  category_label text,
  stated_urgency text NOT NULL DEFAULT 'normal',
  address_text text,
  locality_text text,
  latitude double precision,
  longitude double precision,
  identity_visible_to_resolver boolean NOT NULL DEFAULT false,
  follow_public boolean NOT NULL DEFAULT false,
  status text NOT NULL DEFAULT 'SUBMITTED',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT issues_description_len CHECK (char_length(description) BETWEEN 8 AND 4000),
  CONSTRAINT issues_urgency_check CHECK (stated_urgency IN ('low', 'normal', 'high', 'immediate_risk')),
  CONSTRAINT issues_status_check CHECK (status IN (
    'SUBMITTED', 'NEEDS_INFORMATION', 'UNDER_REVIEW', 'VERIFICATION_PENDING', 'VERIFIED',
    'ROUTING_PENDING', 'AWAITING_RESOLVER_ACCEPTANCE', 'ASSIGNED', 'SCHEDULED', 'IN_PROGRESS',
    'BLOCKED', 'RESOLUTION_SUBMITTED', 'AWAITING_CONFIRMATION', 'RESOLVED', 'DISPUTED',
    'REOPENED', 'REJECTED', 'CANCELLED', 'UNABLE_TO_RESOLVE', 'DUPLICATE_LINKED'
  )),
  CONSTRAINT issues_coords_pair CHECK ((latitude IS NULL) = (longitude IS NULL)),
  CONSTRAINT issues_latitude_range CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
  CONSTRAINT issues_longitude_range CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

CREATE INDEX issues_reporter_created_idx ON issues (reporter_user_id, created_at DESC);

ALTER TABLE issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE issues FORCE ROW LEVEL SECURITY;

CREATE POLICY issues_select ON issues FOR SELECT
  USING (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_flag('app.platform_read')
  );

CREATE POLICY issues_insert ON issues FOR INSERT
  WITH CHECK (reporter_user_id = app_user_id());

GRANT SELECT, INSERT, UPDATE ON issues TO lokasolve_app;
