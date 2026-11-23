-- Quick E1 result check after a run (the full checker is analysis/e1_check.py, owned by Sameep).
-- Counts real enrollment rows rather than trusting the sections.enrolled counter,
-- because the naive strategy can leave the two out of step. Owner: Santosh.
SELECT s.id                                   AS section_id,
       s.capacity,
       s.enrolled                             AS enrolled_counter,
       COUNT(e.student_id)                    AS enrollment_rows,
       GREATEST(COUNT(e.student_id) - s.capacity, 0) AS over_enrolled,
       s.enrolled = COUNT(e.student_id)       AS counter_matches_rows
FROM sections s
LEFT JOIN enrollments e ON e.section_id = s.id
WHERE s.id = 1
GROUP BY s.id, s.capacity, s.enrolled;
