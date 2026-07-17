-- seed-employees.sql — the demo EXTERNAL data source for the db.query node.
--
-- This is NOT part of the control-plane schema (that is applied by migrate.sh /
-- the API's startup migrator). It stands in for a customer HR/payroll database
-- that the seeded "runnable" flow selects from:
--   SELECT name, salary, citizen_id, phone, email FROM employees ORDER BY id
--
-- Apply it to the SAME database the worker's DATABASE_URL points at so the
-- Test-run / runflow demo returns rows (and shows masking: salary/citizen_id/
-- phone/email are masked for non-exempt roles). Idempotent — safe to re-run.
--
-- Usage:
--   psql "$DATABASE_URL" -f scripts/seed-employees.sql

CREATE TABLE IF NOT EXISTS employees (
    id         serial PRIMARY KEY,
    name       text    NOT NULL,
    salary     numeric NOT NULL,
    citizen_id text    NOT NULL,
    phone      text    NOT NULL,
    email      text    NOT NULL
);

INSERT INTO employees (name, salary, citizen_id, phone, email)
SELECT * FROM (VALUES
    ('Somchai Jaidee',  85000, '1103700123456', '0812345678', 'somchai@ttb.local'),
    ('Ploy Rungruang',  62000, '1509900654321', '0898765432', 'ploy@ttb.local'),
    ('Anan Wattanakul', 120000, '3100600111222', '0861112222', 'anan@ttb.local')
) AS v(name, salary, citizen_id, phone, email)
WHERE NOT EXISTS (SELECT 1 FROM employees);
