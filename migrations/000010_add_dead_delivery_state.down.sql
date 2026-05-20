UPDATE activity_deliveries
SET state = 'failed'
WHERE state = 'dead';

ALTER TABLE activity_deliveries
DROP CONSTRAINT IF EXISTS activity_deliveries_state_check;

ALTER TABLE activity_deliveries
ADD CONSTRAINT activity_deliveries_state_check
CHECK (state IN ('pending', 'processing', 'delivered', 'failed'));
