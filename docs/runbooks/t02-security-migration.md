# T-02 security migration

Migration `services/api-go/migrations/0006_access.sql` adds access rules. It does not delete reports, media bytes, users, or assignments.

It does four things:

- Removes the policy that let every signed-in user read every column of a public report.
- Adds assignee and jurisdiction read policies, and a `malware_scan` column whose default for existing and new rows is `not_scanned`.
- Adds `organization_jurisdictions` for an exact locality match. Existing organizations have no jurisdiction rows until one is recorded.
- Adds `public_issue_card`, `public_issue_cards`, and `issue_metrics` so public lists and count dashboards do not select private columns.

Apply it with the normal migrator against a non-production database. The integration tests apply it to `lokasolve_test`. Do not point that command at a production database as part of this change.

## Rollback

Rollback restores the previous, broader public-read policy. Use it only to recover a failed apply. Take a logical backup first. Run the statements below as the migration role, in one transaction.

```sql
BEGIN;
DROP POLICY IF EXISTS issues_assignee_select ON issues;
DROP POLICY IF EXISTS issues_officer_select ON issues;
DROP POLICY IF EXISTS issues_select ON issues;
CREATE POLICY issues_select ON issues FOR SELECT
  USING (
    reporter_user_id = app_user_id()
    OR app_flag('app.platform_admin')
    OR app_flag('app.platform_read')
  );
DROP POLICY IF EXISTS issues_update ON issues;
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
CREATE POLICY issues_public_select ON issues FOR SELECT
  USING (follow_public);
DROP POLICY IF EXISTS issue_events_select ON issue_events;
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
DROP POLICY IF EXISTS media_select ON media_objects;
CREATE POLICY media_select ON media_objects FOR SELECT USING (
  owner_user_id = app_user_id() OR app_flag('app.platform_admin') OR app_flag('app.platform_read')
  OR EXISTS (SELECT 1 FROM issues i WHERE i.id = issue_id AND i.reporter_user_id = app_user_id())
);
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS media_objects_scan;
ALTER TABLE media_objects DROP COLUMN IF EXISTS malware_scan;
DROP TABLE IF EXISTS organization_jurisdictions;
DROP FUNCTION IF EXISTS app_is_issue_reporter(uuid);
DROP FUNCTION IF EXISTS app_is_issue_assignee(uuid);
DROP FUNCTION IF EXISTS app_issue_in_jurisdiction(uuid);
DROP FUNCTION IF EXISTS public_issue_card(uuid);
DROP FUNCTION IF EXISTS public_issue_cards();
DROP FUNCTION IF EXISTS issue_metrics();
DELETE FROM schema_migrations WHERE version IN ('0006_access', '0007_access_checks');
COMMIT;
```

Existing report and media rows remain. Jurisdiction rows exist only if they were added after this migration, and the rollback drops that empty-by-default table.

## Development activation

On 2026-11-10 the local development database received `0006_access` and `0007_access_checks`.

The target was the Docker container `lokasolve-postgres-1`, image `postgis/postgis:16-3.5`, volume `lokasolve_pgdata`, database `lokasolve`, published on localhost port 5432. That is the development database. It was not a production server.

A custom-format dump was taken before the apply and stored at `tmp/lokasolve-pre-t02.dump`. That path is gitignored. The apply used the API process with `AUTO_MIGRATE=true`, which runs the embedded migrator. The database was not dropped or recreated.

The ledger now contains `0001_identity`, `0002_issues`, `0003_taxonomy`, `0004_workflow`, `0005_authority`, `0006_access`, and `0007_access_checks`. After the API was restarted on the new build, the owner of a private report could read it, another account received 404 for that report and its media, and an authorized JPEG download returned the sniffed image type with `malware_scan` `not_scanned`.
