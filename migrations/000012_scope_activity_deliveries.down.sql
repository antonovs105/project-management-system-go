DROP INDEX IF EXISTS idx_activity_deliveries_project_actor_id;

ALTER TABLE activity_deliveries
    DROP COLUMN IF EXISTS project_actor_id;
