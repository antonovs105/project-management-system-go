DROP TRIGGER IF EXISTS tickets_activity_trigger ON tickets;
DROP TRIGGER IF EXISTS projects_activity_trigger ON projects;
DROP TRIGGER IF EXISTS tickets_version_trigger ON tickets;
DROP TRIGGER IF EXISTS projects_version_trigger ON projects;
DROP FUNCTION IF EXISTS bump_entity_version();
DROP FUNCTION IF EXISTS record_project_activity();
DROP TABLE IF EXISTS project_activity_events;

ALTER TABLE tickets DROP COLUMN IF EXISTS archived_at, DROP COLUMN IF EXISTS version;
ALTER TABLE projects DROP COLUMN IF EXISTS archived_at, DROP COLUMN IF EXISTS version;
