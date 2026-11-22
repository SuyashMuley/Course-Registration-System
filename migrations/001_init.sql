-- Owner: Suyash. Initial schema.

CREATE TABLE IF NOT EXISTS students (
    id BIGINT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS sections (
    id       BIGINT PRIMARY KEY,
    capacity INT NOT NULL CHECK (capacity >= 0),
    enrolled INT NOT NULL DEFAULT 0 CHECK (enrolled >= 0)
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
