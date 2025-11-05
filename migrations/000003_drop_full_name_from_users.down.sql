-- ..._drop_full_name_from_users.down.sql
ALTER TABLE users ADD COLUMN full_name VARCHAR(255) NOT NULL;