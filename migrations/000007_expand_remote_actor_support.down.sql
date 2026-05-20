UPDATE actors
SET type = 'Person'
WHERE type NOT IN ('Person', 'Group');

UPDATE actors
SET followers_url = ap_id || '/followers'
WHERE followers_url IS NULL;

ALTER TABLE actors
ALTER COLUMN followers_url SET NOT NULL;

ALTER TABLE actors
DROP CONSTRAINT IF EXISTS actors_type_check;

ALTER TABLE actors
ADD CONSTRAINT actors_type_check
CHECK (type IN ('Person', 'Group'));
