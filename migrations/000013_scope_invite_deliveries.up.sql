WITH scoped_invite_deliveries AS (
    SELECT
        delivery.id,
        COALESCE(
            object_invite.project_id,
            target_invite.project_id
        ) AS project_actor_id
    FROM activity_deliveries delivery
    JOIN ap_activities activity ON activity.id = delivery.activity_id
    LEFT JOIN project_invites object_invite ON object_invite.ap_id = activity.object_ap_id
    LEFT JOIN project_invites target_invite ON target_invite.ap_id = activity.target_ap_id
    WHERE delivery.project_actor_id IS NULL
)
UPDATE activity_deliveries delivery
SET project_actor_id = scoped.project_actor_id
FROM scoped_invite_deliveries scoped
WHERE scoped.id = delivery.id
    AND scoped.project_actor_id IS NOT NULL;
