// Derives the "My Files" listing from executions until the API exposes a
// dedicated /files endpoint. A file row is produced for every successful
// `file.generate` step. Pure + unit-tested; the page is a thin renderer.
import type { Execution } from '@/lib/mock/store';

export type GeneratedFile = {
  id: string;
  filename: string;
  ext: string;
  sizeBytes: number;
  createdAt: string;
  expiresAt: string;
  flowName: string;
  executionId: string;
};

const RETENTION_DAYS = 7;
// Heuristic size until the API returns real bytes: ~180 bytes per output row,
// floored so a single-row file still shows a plausible size.
const BYTES_PER_ROW = 180;

function extFor(nodeName: string): string {
  const m = /\b(xlsx|csv|pdf|txt|json|xml)\b/i.exec(nodeName);
  return (m?.[1] ?? 'xlsx').toLowerCase();
}

function addDays(iso: string, days: number): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  return new Date(t + days * 86400000).toISOString();
}

/** Flatten executions → generated-file rows (successful file.generate steps). */
export function deriveFiles(executions: Execution[]): GeneratedFile[] {
  const files: GeneratedFile[] = [];
  for (const exe of executions) {
    for (const step of exe.steps) {
      if (step.nodeType !== 'file.generate' || step.status !== 'success') continue;
      const ext = extFor(step.nodeName);
      files.push({
        id: `${exe.id}_${step.nodeId}`,
        filename: `${exe.flowName.replace(/[^a-z0-9]+/gi, '_').toLowerCase()}.${ext}`,
        ext,
        sizeBytes: Math.max(step.inputCount, 1) * BYTES_PER_ROW,
        createdAt: exe.startedAt,
        expiresAt: addDays(exe.startedAt, RETENTION_DAYS),
        flowName: exe.flowName,
        executionId: exe.id,
      });
    }
  }
  // Newest first.
  return files.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

/** True once the retention window has passed (download link would be dead). */
export function isExpired(file: GeneratedFile, now: Date = new Date()): boolean {
  const t = new Date(file.expiresAt).getTime();
  if (Number.isNaN(t)) return false;
  return t < now.getTime();
}
