-- Refresh grace storage, P0-P4 severity, human review, and honest referral evidence.
-- Existing rows stay. No table is dropped. Official delivery columns are added empty
-- and no statement in this migration fills them.

ALTER TABLE sessions
  ADD COLUMN successor_refresh_sealed TEXT;

ALTER TABLE issues
  ADD COLUMN severity TEXT,
  ADD COLUMN severity_source TEXT,
  ADD COLUMN severity_confidence NUMERIC,
  ADD COLUMN severity_reason TEXT,
  ADD COLUMN severity_overridden BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN severity_updated_at TIMESTAMPTZ,
  ADD COLUMN escalation_due_at TIMESTAMPTZ,
  ADD COLUMN safety_guidance TEXT;

CREATE TABLE severity_rules (
  id UUID PRIMARY KEY,
  severity TEXT NOT NULL,
  pattern TEXT NOT NULL,
  reason TEXT NOT NULL,
  safety_text TEXT NOT NULL DEFAULT '',
  suggested_department TEXT NOT NULL DEFAULT '',
  review_required BOOLEAN NOT NULL DEFAULT true,
  escalation_hours INT NOT NULL,
  priority INT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT true,
  CONSTRAINT severity_rules_level CHECK (severity IN ('P0', 'P1', 'P2', 'P3', 'P4')),
  CONSTRAINT severity_rules_hours CHECK (escalation_hours BETWEEN 1 AND 720)
);

CREATE TABLE severity_events (
  id UUID PRIMARY KEY,
  issue_id UUID NOT NULL REFERENCES issues (id),
  actor_user_id UUID REFERENCES users (id),
  severity TEXT NOT NULL,
  source TEXT NOT NULL,
  confidence NUMERIC,
  reason TEXT NOT NULL,
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT severity_events_level CHECK (severity IN ('P0', 'P1', 'P2', 'P3', 'P4')),
  CONSTRAINT severity_events_source CHECK (source IN ('rule', 'reviewer', 'unclassified'))
);

CREATE INDEX severity_events_issue_idx ON severity_events (issue_id, created_at DESC);

INSERT INTO severity_rules (
  id, severity, pattern, reason, safety_text, suggested_department, review_required, escalation_hours, priority
) VALUES
  (
    '30000000-0000-4000-8000-000000000001', 'P0',
    '(?i)live wire|exposed wire|electrical wire|electric shock|electrocution|building fire|house fire|structural failure|structural collapse|building collapse|not breathing|unconscious|heart attack|gas leak|explosion',
    'A keyword rule matched an immediate physical danger. This is not an official emergency classification.',
    'This may be an immediate danger. If someone is in danger now, contact a locally published emergency number. No locally verified emergency contact is on file for this place. India''s public directory number is 112. LokaSolve has not dispatched anyone.',
    'Emergency services', true, 1, 10
  ),
  (
    '30000000-0000-4000-8000-000000000002', 'P0',
    '(?i)suicid|kill myself|self[- ]harm|want to die',
    'A keyword rule matched a possible mental-health crisis. This is not a clinical assessment.',
    'If this is a mental-health crisis, Tele-MANAS is 14416 and the public emergency number is 112. These are directory numbers, not a dispatch confirmation. LokaSolve has not dispatched anyone.',
    'Emergency services', true, 1, 11
  ),
  (
    '30000000-0000-4000-8000-000000000003', 'P1',
    '(?i)contamination|sewage overflow|water contamination|utility failure|critical public utility|power outage',
    'A keyword rule matched a critical public-health or utility hazard. This is not an official inspection.',
    '', 'Public health or the utility named in the report', true, 4, 20
  ),
  (
    '30000000-0000-4000-8000-000000000004', 'P2',
    '(?i)locality-wide water|water disruption|major road|road damage|drainage failure|drain blocked',
    'A keyword rule matched a locality-wide disruption. This is not an official inspection.',
    '', 'The civic department for this locality', true, 24, 30
  ),
  (
    '30000000-0000-4000-8000-000000000005', 'P3',
    '(?i)street[[:space:]]*light|streetlight|garbage|trash|waste collection',
    'A keyword rule matched a routine civic issue. This is not an official work order.',
    '', 'Civic maintenance', false, 72, 40
  ),
  (
    '30000000-0000-4000-8000-000000000006', 'P4',
    '(?i)plumbing|appliance|electrician|mechanic',
    'A keyword rule matched a private local service request. This does not select a provider.',
    '', 'A private provider, after the resident chooses one', false, 168, 50
  );

UPDATE issues
SET severity = 'P3',
    severity_source = 'unclassified',
    severity_reason = 'Recorded before severity rules. A person should confirm this classification.',
    severity_updated_at = now(),
    escalation_due_at = now() + interval '72 hours'
WHERE severity IS NULL;

INSERT INTO severity_events (id, issue_id, severity, source, reason, evidence)
SELECT gen_random_uuid(), id, severity, severity_source, severity_reason, '["migration:0008"]'::jsonb
FROM issues
WHERE severity IS NOT NULL;

ALTER TABLE issues
  ADD CONSTRAINT issues_severity_level CHECK (severity IN ('P0', 'P1', 'P2', 'P3', 'P4')),
  ADD CONSTRAINT issues_severity_source CHECK (severity_source IN ('rule', 'reviewer', 'unclassified'));

ALTER TABLE issues
  ALTER COLUMN severity SET DEFAULT 'P3',
  ALTER COLUMN severity_source SET DEFAULT 'unclassified',
  ALTER COLUMN severity_reason SET DEFAULT 'Not classified yet.',
  ALTER COLUMN severity SET NOT NULL,
  ALTER COLUMN severity_source SET NOT NULL,
  ALTER COLUMN severity_reason SET NOT NULL;

ALTER TABLE review_items
  ADD COLUMN severity TEXT,
  ADD COLUMN geographic_area TEXT,
  ADD COLUMN suggested_department TEXT,
  ADD COLUMN report_excerpt TEXT,
  ADD COLUMN assigned_user_id UUID REFERENCES users (id),
  ADD COLUMN deadline_at TIMESTAMPTZ,
  ADD COLUMN decision TEXT,
  ADD COLUMN decision_note TEXT;

ALTER TABLE review_items DROP CONSTRAINT review_items_status;
ALTER TABLE review_items ADD CONSTRAINT review_items_status CHECK (status IN (
  'open', 'needs_information', 'escalated', 'rejected', 'no_destination', 'resolved'
));
ALTER TABLE review_items ADD CONSTRAINT review_items_decision CHECK (
  decision IS NULL OR decision IN (
    'confirm_routing', 'correct_routing', 'request_information', 'escalate',
    'reject_classification', 'no_destination'
  )
);

UPDATE review_items
SET severity = COALESCE(severity, 'P3'),
    deadline_at = COALESCE(deadline_at, created_at + interval '72 hours'),
    geographic_area = COALESCE(geographic_area, 'Not provided')
WHERE severity IS NULL OR deadline_at IS NULL OR geographic_area IS NULL;

INSERT INTO review_items (
  id, issue_id, reason, status, severity, geographic_area, deadline_at, report_excerpt
)
SELECT gen_random_uuid(), i.id,
  'Recorded before severity rules. Confirm the classification. Nothing was submitted.',
  'open', i.severity, COALESCE(i.locality_text, 'Not provided'),
  COALESCE(i.escalation_due_at, now() + interval '72 hours'),
  left(i.description, 280)
FROM issues i
WHERE NOT EXISTS (SELECT 1 FROM review_items r WHERE r.issue_id = i.id);

CREATE TABLE review_events (
  id UUID PRIMARY KEY,
  review_id UUID NOT NULL REFERENCES review_items (id),
  actor_user_id UUID REFERENCES users (id),
  decision TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX review_events_review_idx ON review_events (review_id, created_at);

ALTER TABLE referrals
  ADD COLUMN packet_prepared_at TIMESTAMPTZ,
  ADD COLUMN authorized_at TIMESTAMPTZ,
  ADD COLUMN authorized_by UUID REFERENCES users (id),
  ADD COLUMN submission_attempted_at TIMESTAMPTZ,
  ADD COLUMN submission_channel TEXT,
  ADD COLUMN delivered_at TIMESTAMPTZ,
  ADD COLUMN delivery_evidence TEXT,
  ADD COLUMN official_acknowledged_at TIMESTAMPTZ,
  ADD COLUMN acknowledgement_evidence TEXT,
  ADD COLUMN acknowledgement_kind TEXT,
  ADD COLUMN acknowledgement_note TEXT,
  ADD COLUMN reference_kind TEXT,
  ADD COLUMN assigned_department TEXT,
  ADD COLUMN assignment_kind TEXT,
  ADD COLUMN next_actor TEXT,
  ADD COLUMN escalation_due_at TIMESTAMPTZ,
  ADD COLUMN handoff_at TIMESTAMPTZ,
  ADD COLUMN handoff_by UUID REFERENCES users (id),
  ADD COLUMN handoff_channel TEXT,
  ADD COLUMN handoff_evidence TEXT,
  ADD CONSTRAINT referrals_ack_kind CHECK (acknowledgement_kind IS NULL OR acknowledgement_kind IN ('operator_note')),
  ADD CONSTRAINT referrals_reference_kind CHECK (reference_kind IS NULL OR reference_kind IN ('operator_entered')),
  ADD CONSTRAINT referrals_assignment_kind CHECK (assignment_kind IS NULL OR assignment_kind IN ('operator_entered'));

UPDATE referrals
SET packet_prepared_at = created_at,
    next_actor = COALESCE(next_actor, 'operator')
WHERE status <> 'drafted' AND packet_prepared_at IS NULL;

UPDATE referrals
SET reference_kind = 'operator_entered'
WHERE external_reference IS NOT NULL AND reference_kind IS NULL;

ALTER TABLE severity_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE severity_rules FORCE ROW LEVEL SECURITY;
ALTER TABLE severity_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE severity_events FORCE ROW LEVEL SECURITY;
ALTER TABLE review_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_events FORCE ROW LEVEL SECURITY;

CREATE POLICY severity_rules_read ON severity_rules FOR SELECT USING (true);
CREATE POLICY severity_rules_write ON severity_rules FOR ALL
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY severity_events_select ON severity_events FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
CREATE POLICY severity_events_insert ON severity_events FOR INSERT WITH CHECK (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);

CREATE POLICY review_events_select ON review_events FOR SELECT USING (
  app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (
    SELECT 1 FROM review_items r
    JOIN issues i ON i.id = r.issue_id
    WHERE r.id = review_events.review_id AND i.reporter_user_id = app_user_id()
  )
);
CREATE POLICY review_events_insert ON review_events FOR INSERT WITH CHECK (
  actor_user_id = app_user_id() AND (
    app_flag('app.platform_admin') OR app_flag('app.platform_read')
  )
);

-- Reviewers record a corrected severity without gaining a general issue read.
CREATE OR REPLACE FUNCTION app_record_severity(
  target uuid, level text, source text, reason text, actor uuid
) RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NOT (COALESCE(app_flag('app.platform_admin'), false) OR COALESCE(app_flag('app.platform_read'), false)) THEN
    RAISE EXCEPTION 'forbidden' USING ERRCODE = '42501';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM review_items WHERE issue_id = target) THEN
    RAISE EXCEPTION 'forbidden' USING ERRCODE = '42501';
  END IF;
  IF level NOT IN ('P0', 'P1', 'P2', 'P3', 'P4') THEN
    RAISE EXCEPTION 'invalid severity' USING ERRCODE = '23514';
  END IF;
  IF source NOT IN ('reviewer') THEN
    RAISE EXCEPTION 'invalid source' USING ERRCODE = '23514';
  END IF;
  UPDATE issues
  SET severity = level,
      severity_source = source,
      severity_confidence = NULL,
      severity_reason = reason,
      severity_overridden = true,
      severity_updated_at = now()
  WHERE id = target;
  INSERT INTO severity_events (id, issue_id, actor_user_id, severity, source, reason, evidence)
  VALUES (gen_random_uuid(), target, actor, level, source, reason, '["reviewer"]'::jsonb);
END;
$$;

REVOKE ALL ON FUNCTION app_record_severity(uuid, text, text, text, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_record_severity(uuid, text, text, text, uuid) TO lokasolve_app;

GRANT SELECT, UPDATE ON severity_rules TO lokasolve_app;
GRANT SELECT, INSERT ON severity_events TO lokasolve_app;
GRANT SELECT, INSERT ON review_events TO lokasolve_app;
