ALTER TABLE projects
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN archived_at TIMESTAMPTZ;

ALTER TABLE tickets
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    ADD COLUMN archived_at TIMESTAMPTZ;

CREATE INDEX idx_projects_archived_at ON projects (archived_at) WHERE archived_at IS NOT NULL;
CREATE INDEX idx_tickets_archived_at ON tickets (project_id, archived_at) WHERE archived_at IS NOT NULL;

CREATE TABLE project_activity_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES actors(id) ON DELETE SET NULL,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('project', 'ticket')),
    entity_id UUID NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('created', 'updated', 'archived', 'restored')),
    before_state JSONB,
    after_state JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_project_activity_project_created
    ON project_activity_events (project_id, created_at DESC, id DESC);

CREATE FUNCTION record_project_activity() RETURNS trigger AS $$
DECLARE
    actor UUID;
    event_action TEXT;
BEGIN
    actor := NULLIF(current_setting('progo.actor_id', true), '')::uuid;
    IF TG_OP = 'INSERT' THEN
        event_action := 'created';
    ELSIF OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL THEN
        event_action := 'archived';
    ELSIF OLD.archived_at IS NOT NULL AND NEW.archived_at IS NULL THEN
        event_action := 'restored';
    ELSE
        event_action := 'updated';
    END IF;

    IF TG_TABLE_NAME = 'projects' THEN
        INSERT INTO project_activity_events (project_id, actor_id, entity_type, entity_id, action, before_state, after_state)
        VALUES (NEW.id, actor, 'project', NEW.id, event_action, CASE WHEN TG_OP = 'UPDATE' THEN to_jsonb(OLD) END, to_jsonb(NEW));
    ELSE
        INSERT INTO project_activity_events (project_id, actor_id, entity_type, entity_id, action, before_state, after_state)
        VALUES (NEW.project_id, actor, 'ticket', NEW.id, event_action, CASE WHEN TG_OP = 'UPDATE' THEN to_jsonb(OLD) END, to_jsonb(NEW));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION bump_entity_version() RETURNS trigger AS $$
BEGIN
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER projects_version_trigger
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION bump_entity_version();

CREATE TRIGGER tickets_version_trigger
    BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION bump_entity_version();

CREATE TRIGGER projects_activity_trigger
    AFTER INSERT OR UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION record_project_activity();

CREATE TRIGGER tickets_activity_trigger
    AFTER INSERT OR UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION record_project_activity();
