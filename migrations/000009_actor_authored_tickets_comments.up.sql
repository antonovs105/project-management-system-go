ALTER TABLE tickets
DROP CONSTRAINT IF EXISTS tickets_reporter_id_fkey;

ALTER TABLE tickets
ADD CONSTRAINT tickets_reporter_id_fkey
FOREIGN KEY (reporter_id) REFERENCES actors(id) ON DELETE RESTRICT;

ALTER TABLE comments
DROP CONSTRAINT IF EXISTS comments_author_id_fkey;

ALTER TABLE comments
ADD CONSTRAINT comments_author_id_fkey
FOREIGN KEY (author_id) REFERENCES actors(id) ON DELETE RESTRICT;
