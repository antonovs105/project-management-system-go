CREATE UNIQUE INDEX idx_project_invites_one_pending_per_invitee
ON project_invites(project_id, invitee_actor_id)
WHERE status = 'pending';
