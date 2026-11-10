ALTER TABLE media_objects DROP CONSTRAINT media_objects_type;
ALTER TABLE media_objects ADD CONSTRAINT media_objects_type CHECK (content_type IN (
  'image/jpeg', 'image/png', 'image/webp', 'image/gif',
  'audio/webm', 'audio/mpeg', 'audio/mp4', 'video/mp4', 'video/webm'
));

CREATE TABLE authorities (
  id uuid PRIMARY KEY,
  official_name text NOT NULL,
  org_type text NOT NULL,
  locality_text text,
  categories text NOT NULL DEFAULT '',
  verification_status text NOT NULL DEFAULT 'pending',
  verification_source text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT authorities_name_len CHECK (char_length(official_name) BETWEEN 2 AND 160),
  CONSTRAINT authorities_verification CHECK (verification_status IN ('pending', 'verified', 'unavailable'))
);

CREATE TABLE authority_channels (
  id uuid PRIMARY KEY,
  authority_id uuid NOT NULL REFERENCES authorities (id),
  channel text NOT NULL,
  destination text,
  purpose text,
  verification_status text NOT NULL DEFAULT 'pending',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT authority_channels_kind CHECK (channel IN ('email', 'whatsapp', 'sms', 'portal', 'manual')),
  CONSTRAINT authority_channels_verification CHECK (verification_status IN ('pending', 'verified', 'unavailable'))
);

CREATE TABLE escalation_steps (
  id uuid PRIMARY KEY,
  authority_id uuid NOT NULL REFERENCES authorities (id),
  stage integer NOT NULL,
  condition text NOT NULL,
  target_authority_id uuid NOT NULL REFERENCES authorities (id),
  requires_review boolean NOT NULL DEFAULT true,
  CONSTRAINT escalation_steps_stage CHECK (stage BETWEEN 1 AND 8)
);

CREATE TABLE referrals (
  id uuid PRIMARY KEY,
  issue_id uuid NOT NULL REFERENCES issues (id),
  authority_id uuid NOT NULL REFERENCES authorities (id),
  parent_referral_id uuid REFERENCES referrals (id),
  status text NOT NULL,
  channel text NOT NULL,
  consent_at timestamptz,
  packet jsonb NOT NULL DEFAULT '{}'::jsonb,
  external_reference text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT referrals_status CHECK (status IN (
    'drafted', 'consented', 'approved_for_submission', 'sent', 'provider_accepted', 'delivered',
    'acknowledged', 'officially_registered', 'department_assigned', 'action_started',
    'resolution_reported', 'confirmed', 'closed', 'disputed', 'failed', 'rejected',
    'not_supported', 'manual_handoff', 'unknown', 'retrying'
  ))
);

CREATE INDEX referrals_issue_idx ON referrals (issue_id, created_at DESC);

CREATE TABLE referral_events (
  id uuid PRIMARY KEY,
  referral_id uuid NOT NULL REFERENCES referrals (id),
  actor_user_id uuid REFERENCES users (id),
  status text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE authorities ENABLE ROW LEVEL SECURITY;
ALTER TABLE authorities FORCE ROW LEVEL SECURITY;
ALTER TABLE authority_channels ENABLE ROW LEVEL SECURITY;
ALTER TABLE authority_channels FORCE ROW LEVEL SECURITY;
ALTER TABLE escalation_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE escalation_steps FORCE ROW LEVEL SECURITY;
ALTER TABLE referrals ENABLE ROW LEVEL SECURITY;
ALTER TABLE referrals FORCE ROW LEVEL SECURITY;
ALTER TABLE referral_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE referral_events FORCE ROW LEVEL SECURITY;

CREATE POLICY authorities_select ON authorities FOR SELECT USING (true);
CREATE POLICY authorities_write ON authorities FOR ALL
  USING (app_flag('app.platform_admin')) WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY authority_channels_select ON authority_channels FOR SELECT USING (true);
CREATE POLICY authority_channels_write ON authority_channels FOR ALL
  USING (app_flag('app.platform_admin')) WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY escalation_select ON escalation_steps FOR SELECT USING (true);
CREATE POLICY escalation_write ON escalation_steps FOR ALL
  USING (app_flag('app.platform_admin')) WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY referrals_select ON referrals FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = referrals.issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY referrals_insert ON referrals FOR INSERT WITH CHECK (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = referrals.issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY referrals_update ON referrals FOR UPDATE USING (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = referrals.issue_id AND i.reporter_user_id = app_user_id())
) WITH CHECK (
  app_flag('app.platform_admin')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = referrals.issue_id AND i.reporter_user_id = app_user_id())
);

CREATE POLICY referral_events_select ON referral_events FOR SELECT USING (
  EXISTS (
    SELECT 1 FROM referrals r WHERE r.id = referral_events.referral_id AND (
      app_flag('app.platform_admin') OR app_flag('app.platform_read')
      OR EXISTS (SELECT 1 FROM issues i WHERE i.id = r.issue_id AND i.reporter_user_id = app_user_id())
    )
  )
);
CREATE POLICY referral_events_insert ON referral_events FOR INSERT WITH CHECK (
  actor_user_id = app_user_id() AND EXISTS (
    SELECT 1 FROM referrals r WHERE r.id = referral_events.referral_id AND (
      app_flag('app.platform_admin')
      OR EXISTS (SELECT 1 FROM issues i WHERE i.id = r.issue_id AND i.reporter_user_id = app_user_id())
    )
  )
);

GRANT SELECT, INSERT, UPDATE, DELETE ON authorities TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON authority_channels TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON escalation_steps TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE ON referrals TO lokasolve_app;
GRANT SELECT, INSERT ON referral_events TO lokasolve_app;
