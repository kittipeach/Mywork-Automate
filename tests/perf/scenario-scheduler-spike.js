import http from 'k6/http';
import { check } from 'k6';

// NFR-PERF (E5-S1) — 100 schedules firing in the same minute; scheduler lag < 30s.
export const options = {
  scenarios: {
    scheduler_spike: {
      executor: 'ramping-arrival-rate',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 200,
      stages: [
        { target: 100, duration: '10s' },
        { target: 100, duration: '50s' },
        { target: 0, duration: '10s' },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<30000'], // scheduler lag budget < 30s
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
  return { 'tests/perf/summary-scheduler-spike.json': JSON.stringify(data, null, 2) };
}
