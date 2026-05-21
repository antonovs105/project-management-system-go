ALTER TABLE admin_audit_events
    DROP CONSTRAINT IF EXISTS admin_audit_events_action_check;

UPDATE admin_audit_events
SET action = 'user.role_updated'
WHERE action = 'user.instance_role_updated';

ALTER TABLE admin_audit_events
    ADD CONSTRAINT admin_audit_events_action_check CHECK (
        action IN (
            'user.role_updated',
            'federation.domain_blocked',
            'federation.domain_unblocked',
            'federation.delivery_retried'
        )
    );
