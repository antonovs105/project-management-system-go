ALTER TABLE activity_deliveries
    ADD COLUMN project_actor_id UUID REFERENCES actors(id) ON DELETE SET NULL;

WITH scoped_deliveries AS (
    SELECT
        delivery.id,
        COALESCE(
            target_project_actor.id,
            object_project_actor.id,
            activity_project_actor.id,
            object_ticket.project_id,
            target_ticket.project_id,
            object_comment_ticket.project_id,
            target_comment_ticket.project_id
        ) AS project_actor_id
    FROM activity_deliveries delivery
    JOIN ap_activities activity ON activity.id = delivery.activity_id
    LEFT JOIN actors target_project_actor
        ON target_project_actor.ap_id = activity.target_ap_id
        AND target_project_actor.type = 'Group'
        AND target_project_actor.is_local = true
    LEFT JOIN actors object_project_actor
        ON object_project_actor.ap_id = activity.object_ap_id
        AND object_project_actor.type = 'Group'
        AND object_project_actor.is_local = true
    LEFT JOIN actors activity_project_actor
        ON activity_project_actor.id = activity.actor_id
        AND activity_project_actor.type = 'Group'
        AND activity_project_actor.is_local = true
    LEFT JOIN ap_objects object_scope ON object_scope.ap_id = activity.object_ap_id
    LEFT JOIN tickets object_ticket
        ON object_scope.local_ref_table = 'tickets'
        AND object_ticket.id = object_scope.local_ref_id
    LEFT JOIN comments object_comment
        ON object_scope.local_ref_table = 'comments'
        AND object_comment.id = object_scope.local_ref_id
    LEFT JOIN tickets object_comment_ticket ON object_comment_ticket.id = object_comment.ticket_id
    LEFT JOIN ap_objects target_scope ON target_scope.ap_id = activity.target_ap_id
    LEFT JOIN tickets target_ticket
        ON target_scope.local_ref_table = 'tickets'
        AND target_ticket.id = target_scope.local_ref_id
    LEFT JOIN comments target_comment
        ON target_scope.local_ref_table = 'comments'
        AND target_comment.id = target_scope.local_ref_id
    LEFT JOIN tickets target_comment_ticket ON target_comment_ticket.id = target_comment.ticket_id
)
UPDATE activity_deliveries delivery
SET project_actor_id = scoped.project_actor_id
FROM scoped_deliveries scoped
WHERE scoped.id = delivery.id
    AND scoped.project_actor_id IS NOT NULL;

CREATE INDEX idx_activity_deliveries_project_actor_id
    ON activity_deliveries(project_actor_id);
