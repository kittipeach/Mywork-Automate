import http from 'k6/http';
import { check } from 'k6';

// Scaled baseline smoke (E12-S1): a short, local-friendly slice of the full
// concurrent-runs scenario so we can capture a p95/success baseline against the
// dev stack without a 10-minute soak. The full NFR scenario lives in
// scenario-concurrent-runs.js.
export const options = {
  scenarios: {
    smoke: { executor: 'constant-vus', vus: Number(__ENV.VUS || 10), duration: __ENV.DURATION || '15s' },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'], // NFR-PERF-002
    http_req_failed: ['rate<0.01'],
  },
};

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8090';
const FLOW = __ENV.FLOW_ID || 'flw_payroll';

export default function () {
  const res = http.post(`${BASE}/api/automate/v1/flows/${FLOW}/run`, JSON.stringify({ params: {} }), {
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${__ENV.TOKEN || ''}` },
  });
  check(res, { accepted: (r) => r.status === 202 });
}
