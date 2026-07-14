#!/usr/bin/env node
// Enforce frontend statement-coverage floor from vitest's json-summary reporter.
// Usage: node scripts/check-fe-coverage.mjs --min 90
import { readFileSync } from 'node:fs';

const args = process.argv.slice(2);
const minIdx = args.indexOf('--min');
const min = minIdx >= 0 ? Number(args[minIdx + 1]) : 90;
const summaryPath = 'apps/automate-web/coverage/coverage-summary.json';

let summary;
try {
  summary = JSON.parse(readFileSync(summaryPath, 'utf8'));
} catch (err) {
  console.error(`check-fe-coverage: cannot read ${summaryPath}: ${err.message}`);
  process.exit(1);
}

const total = summary.total;
if (!total) {
  console.error('check-fe-coverage: no "total" key in coverage summary');
  process.exit(1);
}

let fail = false;
for (const metric of ['statements', 'branches', 'functions', 'lines']) {
  const pct = total[metric]?.pct ?? 0;
  const floor = metric === 'statements' || metric === 'lines' ? min : Math.max(min - 10, 0);
  const ok = pct >= floor;
  console.log(`FE ${metric}: ${pct}% (min ${floor}%) ${ok ? 'OK' : 'FAIL'}`);
  if (!ok) fail = true;
}

process.exit(fail ? 1 : 0);
