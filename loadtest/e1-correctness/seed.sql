-- E1 seed data: one section with 10 seats and 1,000 students (ids 1..1000).
-- Runs automatically when the local Postgres volume is first created (docker-compose.yml).
-- Safe to run again. Owner: Santosh.

INSERT INTO students (id)
SELECT g FROM generate_series(1, 1000) AS g
ON CONFLICT (id) DO NOTHING;

INSERT INTO sections (id, capacity, waitlist_cap, enrolled)
VALUES (1, 10, 0, 0)
ON CONFLICT (id) DO UPDATE
    SET capacity = EXCLUDED.capacity,
        waitlist_cap = EXCLUDED.waitlist_cap,
        enrolled = 0;
