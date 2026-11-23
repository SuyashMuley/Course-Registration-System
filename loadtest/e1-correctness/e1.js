// E1: correctness under contention. Owner: Santosh.
//
// Every virtual user (VU) is one student. All of them send ONE register request for the
// same section at the same time. With 1,000 students and 10 seats, a correct strategy
// enrolls exactly 10; the naive strategy is expected to enroll more.
//
// Environment variables:
//   BASE_URL    default http://localhost:8080
//   SECTION_ID  default 1
//   STUDENTS    default 1000 (must not exceed the students in seed.sql)
//   STRATEGY    label only, printed in the result line (the server decides the real strategy)
//
// Reset the database before each run (see README.md), or setup() stops the test.

import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const SECTION_ID = Number(__ENV.SECTION_ID || 1);
const STUDENTS = Number(__ENV.STUDENTS || 1000);
const STRATEGY = __ENV.STRATEGY || 'unknown';

// Outcome counts, shown in the k6 summary.
const enrolled = new Counter('reg_enrolled');
const full = new Counter('reg_full');
const alreadyEnrolled = new Counter('reg_already_enrolled');
const notFound = new Counter('reg_not_found');
const otherErrors = new Counter('reg_other_errors');

// 409 (full / already enrolled) is a correct answer here, not a failure, so k6's
// http_req_failed metric only counts real failures (5xx, 404, timeouts).
http.setResponseCallback(http.expectedStatuses(200, 409));

export const options = {
  scenarios: {
    burst: {
      executor: 'per-vu-iterations',
      vus: STUDENTS,
      iterations: 1,
      maxDuration: '2m',
    },
  },
  // Fails the run (non-zero exit) on any 5xx, timeout, or unexpected status.
  thresholds: {
    reg_other_errors: ['count==0'],
    reg_not_found: ['count==0'],
  },
};

function getSection() {
  const res = http.get(`${BASE_URL}/sections/${SECTION_ID}`, { tags: { name: 'section' } });
  if (res.status !== 200) {
    fail(`GET /sections/${SECTION_ID} returned ${res.status}: ${res.body}`);
  }
  return res.json();
}

export function setup() {
  // Wait up to 30 s for the service to come up.
  for (let i = 0; i < 30; i++) {
    const res = http.get(`${BASE_URL}/healthz`, { tags: { name: 'healthz' } });
    if (res.status === 200) break;
    if (i === 29) fail(`service not healthy at ${BASE_URL}`);
    sleep(1);
  }
  const section = getSection();
  if (section.enrolled !== 0) {
    fail(`section ${SECTION_ID} already has ${section.enrolled} enrolled; run reset.sql first`);
  }
  return { capacity: section.capacity };
}

export default function () {
  const studentId = __VU; // VUs are numbered 1..STUDENTS, matching the seeded student ids
  const res = http.post(
    `${BASE_URL}/register`,
    JSON.stringify({ student_id: studentId, section_id: SECTION_ID }),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'register' } },
  );

  let status = '';
  try {
    status = res.json('status');
  } catch (_) {
    // Non-JSON body (e.g., connection error); counted as an error below.
  }

  if (res.status === 200 && status === 'enrolled') {
    enrolled.add(1);
  } else if (res.status === 409 && status === 'full') {
    full.add(1);
  } else if (res.status === 409 && status === 'already_enrolled') {
    alreadyEnrolled.add(1);
  } else if (res.status === 404) {
    notFound.add(1);
  } else {
    otherErrors.add(1);
  }

  check(res, { 'answered 200 or 409': (r) => r.status === 200 || r.status === 409 });
}

export function teardown(data) {
  const section = getSection();
  // One line that is easy to copy into the results CSV.
  console.log(
    `E1_RESULT strategy=${STRATEGY} students=${STUDENTS} capacity=${data.capacity} ` +
      `enrolled_counter=${section.enrolled} over_enrolled_by_counter=${Math.max(section.enrolled - data.capacity, 0)}`,
  );
}
