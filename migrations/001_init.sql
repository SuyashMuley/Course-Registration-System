-- Owner: Suyash. Initial schema.

CREATE TABLE IF NOT EXISTS students (
    id BIGINT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS sections (
    id           BIGINT PRIMARY KEY,
    capacity     INT NOT NULL CHECK (capacity >= 0),
    waitlist_cap INT NOT NULL DEFAULT 0 CHECK (waitlist_cap >= 0),
    enrolled     INT NOT NULL DEFAULT 0 CHECK (enrolled >= 0)
    -- The CHECK below is the safety net that the naive strategy (E1) is expected to
    -- violate. Keep it OFF in the schema used for E1 so over-enrollment is observable,
    -- and ON for all other strategies:
    -- , CONSTRAINT enrolled_within_capacity CHECK (enrolled <= capacity)
);

CREATE TABLE IF NOT EXISTS enrollments (
    student_id BIGINT NOT NULL REFERENCES students(id),
    section_id BIGINT NOT NULL REFERENCES sections(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, section_id)
);

-- Capped waitlist. Lives in Postgres (not SQS) because it needs positions, leaving the
-- list, and promotion in the same transaction as a drop. Position = order of joined_at,
-- with id as a tie-breaker. Promotion deletes the first row here and inserts into
-- enrollments in one transaction. A student must never be both enrolled and waitlisted
-- in the same section; the invariant checker verifies this.
CREATE TABLE IF NOT EXISTS waitlist (
    id         BIGSERIAL PRIMARY KEY,
    section_id BIGINT NOT NULL REFERENCES sections(id),
    student_id BIGINT NOT NULL REFERENCES students(id),
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (section_id, student_id)
);

CREATE INDEX IF NOT EXISTS waitlist_order ON waitlist (section_id, joined_at, id);
