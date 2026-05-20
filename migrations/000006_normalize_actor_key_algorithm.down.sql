UPDATE actor_keys
SET algorithm = 'rsa-sha256'
WHERE algorithm = 'rsa-v1_5-sha256';

ALTER TABLE actor_keys
ALTER COLUMN algorithm SET DEFAULT 'rsa-sha256';

UPDATE actor_keys
SET private_key_pem = ''
WHERE private_key_pem IS NULL;

ALTER TABLE actor_keys
ALTER COLUMN private_key_pem SET NOT NULL;
