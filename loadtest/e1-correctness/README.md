# E1: Correctness under contention

Goal: show that the naive strategy over-enrolls and the fixed strategies do not.

Planned run: ~1,000 concurrent students registering for a section with 10 seats, once per strategy, repeated 3 times.
Measure: final `enrolled` vs `capacity`, duplicate enrollments, error counts.
