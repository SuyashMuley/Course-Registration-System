-- Empty section 1 before each E1 run. Owner: Santosh.
BEGIN;
DELETE FROM waitlist WHERE section_id = 1;
DELETE FROM enrollments WHERE section_id = 1;
UPDATE sections SET enrolled = 0 WHERE id = 1;
COMMIT;
