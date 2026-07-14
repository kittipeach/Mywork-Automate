import http from 'k6/http';
import { check } from 'k6';

// NFR-PERF-004/009 — 100 concurrent flow runs sustained.
export const options = {
  scenarios: { concurrent_runs: { executor: 'constant-vus', vus: 100, duration: '10m' } },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'], // NFR-PERF-002
    http_req_failed: ['rate<0.005'],                 // success >= 99.5%
  },
};

export default function () {
  const res = http.post(
    `${__ENV.BASE_URL}/api/automate/v1/flows/${__ENV.FLOW_ID}/run`,
    JSON.stringify({ params: {} }),
    { headers: { Authorization: `Bearer ${__ENV.TOKEN}`, 'Content-Type': 'application/json' } },
  );
  check(res, { accepted: (r) => r.status === 202 });
}

export function handleSummary(data) {
  return { 'tests/perf/summary-concurrent-runs.json': JSON.stringify(data, null, 2) };
}
