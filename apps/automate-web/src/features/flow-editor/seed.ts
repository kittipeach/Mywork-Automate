// A realistic starter graph so opening any flow id shows a populated canvas:
// Schedule → DB Query → If → Generate File → MFT, with the If's False branch
// wired to a Download node. Config is pre-filled so most nodes validate.
import type { GraphSnapshot, ConfigByNode } from './types';

export function seedFlow(): { snapshot: GraphSnapshot; config: ConfigByNode } {
  const snapshot: GraphSnapshot = {
    nodes: [
      { id: 'n_schedule', type: 'automateNode', position: { x: 40, y: 200 }, data: { nodeType: 'trigger.schedule', name: 'Daily 06:00' } },
      { id: 'n_query', type: 'automateNode', position: { x: 300, y: 200 }, data: { nodeType: 'db.query', name: 'Query Headcount' } },
      { id: 'n_if', type: 'automateNode', position: { x: 560, y: 200 }, data: { nodeType: 'logic.if', name: 'Has rows?' } },
      { id: 'n_file', type: 'automateNode', position: { x: 820, y: 120 }, data: { nodeType: 'file.generate', name: 'Generate XLSX' } },
      { id: 'n_mft', type: 'automateNode', position: { x: 1080, y: 120 }, data: { nodeType: 'delivery.mft', name: 'MFT to Bank' } },
      { id: 'n_download', type: 'automateNode', position: { x: 820, y: 320 }, data: { nodeType: 'delivery.download', name: 'Skip → Download' } },
    ],
    edges: [
      { id: 'e1', source: 'n_schedule', target: 'n_query', type: 'smoothstep' },
      { id: 'e2', source: 'n_query', target: 'n_if', type: 'smoothstep' },
      { id: 'e3', source: 'n_if', sourceHandle: 'true', target: 'n_file', type: 'smoothstep', label: 'True' },
      { id: 'e4', source: 'n_if', sourceHandle: 'false', target: 'n_download', type: 'smoothstep', label: 'False' },
      { id: 'e5', source: 'n_file', target: 'n_mft', type: 'smoothstep' },
    ],
  };

  const config: ConfigByNode = {
    n_schedule: { mode: 'simple', timezone: 'Asia/Bangkok', overlapPolicy: 'skip', everyMinutes: 1440 },
    n_query: { connectionId: 'conn_hr', mode: 'sql', sql: 'SELECT * FROM headcount', maxRows: 1000 },
    n_if: { left: '{{ $node.rowCount }}', op: '>', right: 0 },
    n_file: { format: 'xlsx', filename: 'headcount_{{ $flow.runDate }}.xlsx', onEmpty: 'skip', retentionDays: 30 },
    // n_mft intentionally missing connectionId so Validate has something to flag
    n_mft: { remotePath: '/upload/', auth: 'sshKey', retries: 3 },
    n_download: { expiryHours: 24, notify: false },
  };

  return { snapshot, config };
}
