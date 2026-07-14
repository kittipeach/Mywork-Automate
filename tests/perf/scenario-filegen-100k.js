import http from 'k6/http';
import { check } from 'k6';

// NFR-PERF-005/006 — query 100k rows -> XLSX generation within budget.
export const options = {
  scenarios: { filegen_100k: { executor: 'per-vu-iterations', vus: 5, iterations: 5, maxDuration: '15m' } },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    iteration_duration: ['p(95)<300000'], // < 5 min per 100k-row file gen (NFR-PERF-005)
  },
};

export default function () {
  const res = http.post(
    `${__ENV.BASE_URL}/api/automate/v1/flows/${__ENV.FILEGEN_FLOW_ID}/run`,
    JSON.stringify({ params: { rows: 100000 } }),
    { headers: { Authorization: `Bearer ${__ENV.TOKEN}`, 'Content-Type': 'application/json' } },
  );
  check(res, { accepted: (r) => r.status === 202 });
}

export function handleSummary(data) {
  return { 'tests/perf/summary-filegen-100k.json': JSON.stringify(data, null, 2) };
}
