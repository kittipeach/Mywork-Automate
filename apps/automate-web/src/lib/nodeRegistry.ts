// Node registry: the catalog the palette renders and the config panel forms are
// generated from (ADR-03: adding a node = adding a schema, no core changes).
// In production this is served by GET /api/automate/v1/nodes; the mock route
// returns exactly this shape.

export type JSONSchema = {
  type?: string;
  title?: string;
  description?: string;
  properties?: Record<string, JSONSchema>;
  required?: string[];
  enum?: string[];
  enumLabels?: string[];
  default?: unknown;
  format?: 'expression' | 'sql' | 'textarea' | 'password' | 'connection';
  /** Narrows a format:'connection' field to one connection type (postgres/sftp). */
  connectionType?: string;
  items?: JSONSchema;
  minimum?: number;
  maximum?: number;
  // conditional fields: fieldName -> fieldValue -> { extra property -> schema }.
  // When the named field equals a value, the extra properties are merged in.
  dependentSchemas?: Record<string, Record<string, Record<string, JSONSchema>>>;
};

export type PortSpec = { id: string; label: string };

export type NodeType = {
  type: string;
  category: 'Trigger' | 'Data' | 'Logic' | 'File' | 'Delivery';
  label: string;
  description: string;
  icon: string; // lucide-react icon name
  accent: string; // tailwind text color class for the node badge
  inputs: PortSpec[];
  outputs: PortSpec[];
  schema: JSONSchema;
};

export const NODE_TYPES: NodeType[] = [
  {
    type: 'trigger.schedule',
    category: 'Trigger',
    label: 'Schedule',
    description: 'Run on a recurring schedule (cron or simple).',
    icon: 'CalendarClock',
    accent: 'text-info',
    inputs: [],
    outputs: [{ id: 'out', label: 'Next' }],
    schema: {
      type: 'object',
      properties: {
        mode: { type: 'string', title: 'Mode', enum: ['simple', 'cron'], enumLabels: ['Simple', 'Cron'], default: 'simple' },
        timezone: { type: 'string', title: 'Timezone', default: 'Asia/Bangkok' },
        overlapPolicy: { type: 'string', title: 'Overlap policy', enum: ['skip', 'queue', 'parallel'], default: 'skip' },
      },
      required: ['mode', 'timezone'],
      dependentSchemas: {
        mode: {
          simple: { everyMinutes: { type: 'number', title: 'Every (minutes)', minimum: 1, default: 1440 } },
          cron: { cron: { type: 'string', title: 'Cron expression', default: '0 6 * * *' } },
        },
      },
    },
  },
  {
    type: 'trigger.manual',
    category: 'Trigger',
    label: 'Manual',
    description: 'Run on demand with optional input parameters.',
    icon: 'MousePointerClick',
    accent: 'text-info',
    inputs: [],
    outputs: [{ id: 'out', label: 'Next' }],
    schema: { type: 'object', properties: {} },
  },
  {
    type: 'db.query',
    category: 'Data',
    label: 'DB Query',
    description: 'Run a read-only SELECT against a Postgres connection.',
    icon: 'Database',
    accent: 'text-brand',
    inputs: [{ id: 'in', label: 'In' }],
    outputs: [{ id: 'out', label: 'Rows' }],
    schema: {
      type: 'object',
      properties: {
        connectionId: { type: 'string', title: 'Connection', format: 'connection', connectionType: 'postgres', description: 'Postgres connection (RBAC-filtered).' },
        maxRows: { type: 'number', title: 'Max rows', minimum: 1, maximum: 100000, default: 1000 },
        // The query is built with the visual QueryBuilder (a structured,
        // server-compiled spec — no free-form SQL field).
      },
      required: ['connectionId'],
    },
  },
  {
    type: 'logic.if',
    category: 'Logic',
    label: 'If',
    description: 'Branch true/false on a condition.',
    icon: 'GitBranch',
    accent: 'text-warning',
    inputs: [{ id: 'in', label: 'In' }],
    outputs: [{ id: 'true', label: 'True' }, { id: 'false', label: 'False' }],
    schema: {
      type: 'object',
      properties: {
        left: { type: 'string', title: 'Left', format: 'expression', default: '{{ $node.rowCount }}' },
        op: { type: 'string', title: 'Operator', enum: ['>', '>=', '<', '<=', '==', '!='], default: '>' },
        right: { type: 'number', title: 'Right', default: 0 },
      },
      required: ['left', 'op', 'right'],
    },
  },
  {
    type: 'logic.transform',
    category: 'Logic',
    label: 'Transform',
    description: 'Select / rename / filter / sort items.',
    icon: 'Shuffle',
    accent: 'text-warning',
    inputs: [{ id: 'in', label: 'In' }],
    outputs: [{ id: 'out', label: 'Out' }],
    schema: {
      type: 'object',
      properties: {
        op: { type: 'string', title: 'Operation', enum: ['select', 'rename', 'filter', 'sort'], default: 'select' },
        expression: { type: 'string', title: 'Expression', format: 'expression' },
      },
      required: ['op'],
    },
  },
  {
    type: 'file.generate',
    category: 'File',
    label: 'Generate File',
    description: 'Produce XLSX / CSV / TXT from items.',
    icon: 'FileSpreadsheet',
    accent: 'text-success',
    inputs: [{ id: 'in', label: 'Items' }],
    outputs: [{ id: 'out', label: 'File' }],
    schema: {
      type: 'object',
      properties: {
        format: { type: 'string', title: 'Format', enum: ['xlsx', 'csv', 'txt'], enumLabels: ['Excel (XLSX)', 'CSV', 'Text'], default: 'xlsx' },
        filename: { type: 'string', title: 'Filename', format: 'expression', default: 'report_{{ $flow.runDate | format:"20060102" }}.xlsx' },
        onEmpty: { type: 'string', title: 'On empty', enum: ['skip', 'emptyFile', 'fail'], default: 'skip' },
        retentionDays: { type: 'number', title: 'Retention (days)', minimum: 1, default: 30 },
      },
      required: ['format', 'filename'],
      dependentSchemas: {
        format: {
          txt: { delimiter: { type: 'string', title: 'Delimiter', default: ',' } },
        },
      },
    },
  },
  {
    type: 'delivery.mft',
    category: 'Delivery',
    label: 'MFT / SFTP',
    description: 'Send file to an SFTP endpoint (atomic + retry).',
    icon: 'Send',
    accent: 'text-brand',
    inputs: [{ id: 'in', label: 'File' }],
    outputs: [{ id: 'out', label: 'Done' }],
    schema: {
      type: 'object',
      properties: {
        connectionId: { type: 'string', title: 'SFTP connection', format: 'connection', connectionType: 'sftp' },
        remotePath: { type: 'string', title: 'Remote path', format: 'expression', default: '/upload/' },
        auth: { type: 'string', title: 'Auth', enum: ['password', 'sshKey'], default: 'sshKey' },
        retries: { type: 'number', title: 'Retries', minimum: 0, maximum: 10, default: 3 },
      },
      required: ['connectionId', 'remotePath'],
    },
  },
  {
    type: 'delivery.email',
    category: 'Delivery',
    label: 'Email',
    description: 'Send email with the generated file attached.',
    icon: 'Mail',
    accent: 'text-brand',
    inputs: [{ id: 'in', label: 'File' }],
    outputs: [{ id: 'out', label: 'Done' }],
    schema: {
      type: 'object',
      properties: {
        transport: { type: 'string', title: 'Transport', enum: ['smtp', 'graph'], enumLabels: ['SMTP', 'MS Graph'], default: 'smtp' },
        to: { type: 'string', title: 'To', format: 'expression' },
        subject: { type: 'string', title: 'Subject', format: 'expression' },
        body: { type: 'string', title: 'Body', format: 'textarea' },
        sendMode: { type: 'string', title: 'Send mode', enum: ['single', 'perItem'], default: 'single' },
      },
      required: ['to', 'subject'],
    },
  },
  {
    type: 'delivery.download',
    category: 'Delivery',
    label: 'Download',
    description: 'Make the file available in My Files (signed URL).',
    icon: 'Download',
    accent: 'text-brand',
    inputs: [{ id: 'in', label: 'File' }],
    outputs: [],
    schema: {
      type: 'object',
      properties: {
        expiryHours: { type: 'number', title: 'Link expiry (hours)', minimum: 1, maximum: 168, default: 24 },
        notify: { type: 'boolean', title: 'Notify users' },
      },
    },
  },
];

export const NODE_BY_TYPE: Record<string, NodeType> = Object.fromEntries(
  NODE_TYPES.map((n) => [n.type, n]),
);

export const NODE_CATEGORIES = ['Trigger', 'Data', 'Logic', 'File', 'Delivery'] as const;
