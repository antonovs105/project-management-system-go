CREATE TABLE remote_project_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invite_ap_id TEXT NOT NULL UNIQUE,
    activity_id UUID NOT NULL REFERENCES ap_activities(id) ON DELETE CASCADE,
    invitee_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    inviter_actor_id UUID NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    project_ap_id TEXT NOT NULL,
    project_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    target_inbox_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    response_activity_id UUID REFERENCES ap_activities(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT remote_project_invites_project_ap_id_not_blank CHECK (btrim(project_ap_id) <> ''),
    CONSTRAINT remote_project_invites_target_inbox_url_not_blank CHECK (btrim(target_inbox_url) <> '')
);

CREATE INDEX idx_remote_project_invites_invitee_status
    ON remote_project_invites(invitee_actor_id, status, updated_at DESC);

CREATE TRIGGER trg_remote_project_invites_updated_at BEFORE UPDATE ON remote_project_invites
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
