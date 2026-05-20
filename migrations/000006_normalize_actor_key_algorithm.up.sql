UPDATE actor_keys
SET algorithm = 'rsa-v1_5-sha256'
WHERE algorithm = 'rsa-sha256';

ALTER TABLE actor_keys
ALTER COLUMN algorithm SET DEFAULT 'rsa-v1_5-sha256';

ALTER TABLE actor_keys
ALTER COLUMN private_key_pem DROP NOT NULL;
