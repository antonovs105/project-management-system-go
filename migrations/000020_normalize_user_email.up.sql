DO $$
BEGIN
    IF EXISTS (
        SELECT lower(btrim(email))
        FROM users
        GROUP BY lower(btrim(email))
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot normalize user emails: duplicate emails differ only by case or surrounding whitespace';
    END IF;
END $$;

UPDATE users
SET email = lower(btrim(email))
WHERE email <> lower(btrim(email));

CREATE UNIQUE INDEX idx_users_email_lower
    ON users (lower(email));
