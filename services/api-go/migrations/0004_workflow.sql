ALTER TABLE problem_types ADD COLUMN fields jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE issues ADD COLUMN answers jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE POLICY issues_public_select ON issues FOR SELECT
  USING (follow_public);

CREATE TABLE issue_events (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  actor_user_id uuid REFERENCES users (id),
  kind text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT issue_events_body_len CHECK (char_length(body) BETWEEN 1 AND 2000)
);

CREATE INDEX issue_events_issue_idx ON issue_events (issue_id, created_at);

CREATE TABLE media_objects (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  owner_user_id uuid NOT NULL REFERENCES users (id),
  file_name text NOT NULL,
  content_type text NOT NULL,
  byte_size integer NOT NULL,
  content bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT media_objects_size CHECK (byte_size > 0 AND byte_size <= 4000000),
  CONSTRAINT media_objects_type CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp', 'image/gif'))
);

CREATE TABLE incidents (
  id uuid PRIMARY KEY,
  title text NOT NULL,
  locality_text text,
  status text NOT NULL DEFAULT 'open',
  created_by uuid REFERENCES users (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT incidents_status CHECK (status IN ('open', 'closed'))
);

CREATE TABLE incident_links (
  incident_id uuid NOT NULL REFERENCES incidents (id),
  issue_id uuid NOT NULL REFERENCES issues (id),
  relation text NOT NULL,
  PRIMARY KEY (incident_id, issue_id),
  CONSTRAINT incident_links_relation CHECK (relation IN ('member', 'duplicate'))
);

CREATE UNIQUE INDEX incident_links_one_member ON incident_links (issue_id) WHERE relation = 'member';

CREATE TABLE review_items (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  reason text NOT NULL,
  status text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT review_items_status CHECK (status IN ('open', 'resolved'))
);

CREATE TABLE resolvers (
  id uuid PRIMARY KEY,
  display_name text NOT NULL,
  kind text NOT NULL,
  locality_text text,
  skills text NOT NULL,
  verification_status text NOT NULL DEFAULT 'unverified',
  owner_user_id uuid REFERENCES users (id),
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT resolvers_kind CHECK (kind IN ('provider', 'organization', 'field')),
  CONSTRAINT resolvers_verification CHECK (verification_status IN ('unverified', 'reviewed')),
  CONSTRAINT resolvers_name_len CHECK (char_length(display_name) BETWEEN 2 AND 120)
);

CREATE TABLE assignments (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  resolver_id uuid NOT NULL REFERENCES resolvers (id),
  status text NOT NULL,
  explanation text NOT NULL,
  quote_text text,
  quote_approved boolean NOT NULL DEFAULT false,
  appointment_text text,
  evidence_text text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assignments_status CHECK (status IN (
    'offered', 'accepted', 'rejected', 'scheduled', 'in_progress',
    'resolution_submitted', 'resolved', 'disputed', 'reopened'
  ))
);

CREATE POLICY issues_update ON issues FOR UPDATE
  USING (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR EXISTS (
      SELECT 1 FROM assignments a
      JOIN resolvers r ON r.id = a.resolver_id
      WHERE a.issue_id = issues.id AND r.owner_user_id = app_user_id()
    )
  )
  WITH CHECK (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR EXISTS (
      SELECT 1 FROM assignments a
      JOIN resolvers r ON r.id = a.resolver_id
      WHERE a.issue_id = issues.id AND r.owner_user_id = app_user_id()
    )
  );

CREATE TABLE notifications (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  issue_id uuid REFERENCES issues (id),
  title text NOT NULL,
  body text NOT NULL,
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);

CREATE TABLE message_attempts (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  issue_id uuid REFERENCES issues (id),
  channel text NOT NULL,
  status text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT message_attempts_channel CHECK (channel IN ('in_app', 'email', 'sms', 'whatsapp')),
  CONSTRAINT message_attempts_status CHECK (status IN ('recorded', 'not_configured'))
);

CREATE TABLE digital_actions (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  template_name text NOT NULL,
  external_reference text,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT digital_actions_status CHECK (status IN ('recorded', 'not_submitted'))
);

ALTER TABLE issue_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE issue_events FORCE ROW LEVEL SECURITY;
ALTER TABLE media_objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_objects FORCE ROW LEVEL SECURITY;
ALTER TABLE incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE incidents FORCE ROW LEVEL SECURITY;
ALTER TABLE incident_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE incident_links FORCE ROW LEVEL SECURITY;
ALTER TABLE review_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_items FORCE ROW LEVEL SECURITY;
ALTER TABLE resolvers ENABLE ROW LEVEL SECURITY;
ALTER TABLE resolvers FORCE ROW LEVEL SECURITY;
ALTER TABLE assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE assignments FORCE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
ALTER TABLE message_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_attempts FORCE ROW LEVEL SECURITY;
ALTER TABLE digital_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE digital_actions FORCE ROW LEVEL SECURITY;

CREATE POLICY issue_events_select ON issue_events FOR SELECT USING (
  EXISTS (
    SELECT 1 FROM issues i WHERE i.id = issue_events.issue_id AND (
      i.reporter_user_id = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read')
    )
  )
  OR EXISTS (
    SELECT 1 FROM assignments a JOIN resolvers r ON r.id = a.resolver_id
    WHERE a.issue_id = issue_events.issue_id AND r.owner_user_id = app_user_id()
  )
);
CREATE POLICY issue_events_insert ON issue_events FOR INSERT WITH CHECK (
  actor_user_id = app_user_id() AND (
    app_flag('app.platform_admin')
    OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
    OR EXISTS (
      SELECT 1 FROM assignments a JOIN resolvers r ON r.id = a.resolver_id
      WHERE a.issue_id = issue_events.issue_id AND r.owner_user_id = app_user_id()
    )
  )
);

CREATE POLICY media_select ON media_objects FOR SELECT USING (
  owner_user_id = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY media_insert ON media_objects FOR INSERT WITH CHECK (
  owner_user_id = app_user_id()
  AND EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);

CREATE POLICY incidents_all ON incidents FOR ALL USING (
  created_by = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read')
) WITH CHECK (created_by = app_user_id() OR app_flag('app.platform_admin'));

CREATE POLICY incident_links_select ON incident_links FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY incident_links_insert ON incident_links FOR INSERT WITH CHECK (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);

CREATE POLICY review_select ON review_items FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY review_insert ON review_items FOR INSERT WITH CHECK (
  EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
  OR app_flag('app.platform_admin')
);
CREATE POLICY review_update ON review_items FOR UPDATE USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = review_items.issue_id AND i.reporter_user_id = app_user_id())
) WITH CHECK (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = review_items.issue_id AND i.reporter_user_id = app_user_id())
);

CREATE POLICY resolvers_select ON resolvers FOR SELECT USING (true);
CREATE POLICY resolvers_write ON resolvers FOR ALL
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY assignments_select ON assignments FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
);
CREATE POLICY assignments_insert ON assignments FOR INSERT WITH CHECK (app_flag('app.platform_admin'));
CREATE POLICY assignments_update ON assignments FOR UPDATE USING (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
) WITH CHECK (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
  OR EXISTS (SELECT 1 FROM resolvers r WHERE r.id = resolver_id AND r.owner_user_id = app_user_id())
);

CREATE POLICY notifications_select ON notifications FOR SELECT USING (user_id = app_user_id() OR app_flag('app.platform_read'));
CREATE POLICY notifications_insert ON notifications FOR INSERT WITH CHECK (
  user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR EXISTS (
    SELECT 1 FROM issues i WHERE i.id = notifications.issue_id AND (
      i.reporter_user_id = app_user_id()
      OR EXISTS (
        SELECT 1 FROM assignments a JOIN resolvers r ON r.id = a.resolver_id
        WHERE a.issue_id = i.id AND r.owner_user_id = app_user_id()
      )
    )
  )
);
CREATE POLICY notifications_update ON notifications FOR UPDATE USING (user_id = app_user_id()) WITH CHECK (user_id = app_user_id());

CREATE POLICY message_attempts_select ON message_attempts FOR SELECT USING (user_id = app_user_id() OR app_flag('app.platform_read'));
CREATE POLICY message_attempts_insert ON message_attempts FOR INSERT WITH CHECK (
  user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR EXISTS (
    SELECT 1 FROM issues i WHERE i.id = message_attempts.issue_id AND (
      i.reporter_user_id = app_user_id()
      OR EXISTS (
        SELECT 1 FROM assignments a JOIN resolvers r ON r.id = a.resolver_id
        WHERE a.issue_id = i.id AND r.owner_user_id = app_user_id()
      )
    )
  )
);

CREATE POLICY digital_select ON digital_actions FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY digital_insert ON digital_actions FOR INSERT WITH CHECK (app_flag('app.platform_admin'));

GRANT SELECT, INSERT, UPDATE, DELETE ON issue_events TO lokasolve_app;
GRANT SELECT, INSERT ON media_objects TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE ON incidents TO lokasolve_app;
GRANT SELECT, INSERT ON incident_links TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE ON review_items TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON resolvers TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE ON assignments TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE ON notifications TO lokasolve_app;
GRANT SELECT, INSERT ON message_attempts TO lokasolve_app;
GRANT SELECT, INSERT ON digital_actions TO lokasolve_app;
