DROP INDEX IF EXISTS idx_tickets_project_due_date;
ALTER TABLE tickets DROP COLUMN IF EXISTS due_date;
