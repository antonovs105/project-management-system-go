ALTER TABLE actors
DROP CONSTRAINT IF EXISTS actors_type_check;

ALTER TABLE actors
ADD CONSTRAINT actors_type_check
CHECK (type IN ('Person', 'Group', 'Organization', 'Application', 'Service'));

ALTER TABLE actors
ALTER COLUMN followers_url DROP NOT NULL;
