-- 0008_connection_dsn — external DB connection dial fields (E6-S1).
-- The connections table gains the fields needed to dial an external Postgres
-- from the admin UI: host is already present; add port/database/username/
-- ssl_mode and secret_ref (the NAME of the secret holding the password — the
-- password itself is never stored here, it lives in the secret store).
-- All nullable/defaulted so existing rows and the demo seed keep working.

ALTER TABLE connections ADD COLUMN IF NOT EXISTS port      INT  NOT NULL DEFAULT 0;
ALTER TABLE connections ADD COLUMN IF NOT EXISTS database  TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN IF NOT EXISTS username  TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN IF NOT EXISTS ssl_mode  TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN IF NOT EXISTS secret_ref TEXT NOT NULL DEFAULT '';
