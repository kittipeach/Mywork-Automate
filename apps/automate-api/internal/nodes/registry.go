// Package nodes is the Go mirror of the web node registry
// (apps/automate-web/src/lib/nodeRegistry.ts). GET /nodes serves this catalog
// so the UI palette and config-panel forms can be generated from it
// (ADR-03: adding a node = adding a schema, no core changes).
//
// The JSON shapes here match the TS NodeType/JSONSchema byte-for-byte: optional
// fields use omitempty so a marshalled node contains exactly the keys the TS
// object literal set.
package nodes

// JSONSchema mirrors the TS JSONSchema type. Every field is optional and uses
// omitempty; Default is any so it can hold a string, number, or bool.
type JSONSchema struct {
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Properties is a pointer so a nil (leaf schema) is omitted while a non-nil
	// empty map serialises as `{}` (the TS trigger.manual schema is
	// `properties: {}`, which omitempty on a plain map would wrongly drop).
	Properties *map[string]JSONSchema `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Enum       []string               `json:"enum,omitempty"`
	EnumLabels []string               `json:"enumLabels,omitempty"`
	Default    any                    `json:"default,omitempty"`
	Format     string                 `json:"format,omitempty"`
	// ConnectionType narrows a format:"connection" field to one connection type
	// (e.g. "postgres", "sftp") so the UI dropdown only offers matching ones.
	ConnectionType   string                                      `json:"connectionType,omitempty"`
	Items            *JSONSchema                                 `json:"items,omitempty"`
	Minimum          *float64                                    `json:"minimum,omitempty"`
	Maximum          *float64                                    `json:"maximum,omitempty"`
	DependentSchemas map[string]map[string]map[string]JSONSchema `json:"dependentSchemas,omitempty"`
}

// PortSpec mirrors the TS PortSpec.
type PortSpec struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// NodeType mirrors the TS NodeType. Inputs/Outputs are always non-nil so an
// empty port list serialises as `[]` (matching the TS array literals).
type NodeType struct {
	Type        string     `json:"type"`
	Category    string     `json:"category"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Icon        string     `json:"icon"`
	Accent      string     `json:"accent"`
	Inputs      []PortSpec `json:"inputs"`
	Outputs     []PortSpec `json:"outputs"`
	Schema      JSONSchema `json:"schema"`
}

// fptr is a tiny helper for the *float64 minimum/maximum fields.
func fptr(v float64) *float64 { return &v }

// props wraps a properties map in a pointer so it is always emitted (even when
// empty) for object schemas.
func props(m map[string]JSONSchema) *map[string]JSONSchema { return &m }

// nodeTypes is the static catalog, a faithful port of NODE_TYPES in
// nodeRegistry.ts (same order, same fields, same defaults).
var nodeTypes = []NodeType{
	{
		Type:        "trigger.schedule",
		Category:    "Trigger",
		Label:       "Schedule",
		Description: "Run on a recurring schedule (cron or simple).",
		Icon:        "CalendarClock",
		Accent:      "text-info",
		Inputs:      []PortSpec{},
		Outputs:     []PortSpec{{ID: "out", Label: "Next"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"mode":          {Type: "string", Title: "Mode", Enum: []string{"simple", "cron"}, EnumLabels: []string{"Simple", "Cron"}, Default: "simple"},
				"timezone":      {Type: "string", Title: "Timezone", Default: "Asia/Bangkok"},
				"overlapPolicy": {Type: "string", Title: "Overlap policy", Enum: []string{"skip", "queue", "parallel"}, Default: "skip"},
			}),
			Required: []string{"mode", "timezone"},
			DependentSchemas: map[string]map[string]map[string]JSONSchema{
				"mode": {
					"simple": {"everyMinutes": {Type: "number", Title: "Every (minutes)", Minimum: fptr(1), Default: float64(1440)}},
					"cron":   {"cron": {Type: "string", Title: "Cron expression", Default: "0 6 * * *"}},
				},
			},
		},
	},
	{
		Type:        "trigger.manual",
		Category:    "Trigger",
		Label:       "Manual",
		Description: "Run on demand with optional input parameters.",
		Icon:        "MousePointerClick",
		Accent:      "text-info",
		Inputs:      []PortSpec{},
		Outputs:     []PortSpec{{ID: "out", Label: "Next"}},
		Schema:      JSONSchema{Type: "object", Properties: props(map[string]JSONSchema{})},
	},
	{
		Type:        "db.query",
		Category:    "Data",
		Label:       "DB Query",
		Description: "Run a read-only SELECT against a Postgres connection.",
		Icon:        "Database",
		Accent:      "text-brand",
		Inputs:      []PortSpec{{ID: "in", Label: "In"}},
		Outputs:     []PortSpec{{ID: "out", Label: "Rows"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"connectionId": {Type: "string", Title: "Connection", Format: "connection", ConnectionType: "postgres", Description: "Postgres connection (RBAC-filtered)."},
				"maxRows":      {Type: "number", Title: "Max rows", Minimum: fptr(1), Maximum: fptr(100000), Default: float64(1000)},
				// The query itself is built with the visual QueryBuilder (a
				// structured, server-compiled spec — no free-form SQL field). See
				// pkg/sqlbuilder + the automate-web QueryBuilder component.
			}),
			Required: []string{"connectionId"},
		},
	},
	{
		Type:        "logic.if",
		Category:    "Logic",
		Label:       "If",
		Description: "Branch true/false on a condition.",
		Icon:        "GitBranch",
		Accent:      "text-warning",
		Inputs:      []PortSpec{{ID: "in", Label: "In"}},
		Outputs:     []PortSpec{{ID: "true", Label: "True"}, {ID: "false", Label: "False"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"left":  {Type: "string", Title: "Left", Format: "expression", Default: "{{ $node.rowCount }}"},
				"op":    {Type: "string", Title: "Operator", Enum: []string{">", ">=", "<", "<=", "==", "!="}, Default: ">"},
				"right": {Type: "number", Title: "Right", Default: float64(0)},
			}),
			Required: []string{"left", "op", "right"},
		},
	},
	{
		Type:        "logic.transform",
		Category:    "Logic",
		Label:       "Transform",
		Description: "Select / rename / filter / sort items.",
		Icon:        "Shuffle",
		Accent:      "text-warning",
		Inputs:      []PortSpec{{ID: "in", Label: "In"}},
		Outputs:     []PortSpec{{ID: "out", Label: "Out"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"op":         {Type: "string", Title: "Operation", Enum: []string{"select", "rename", "filter", "sort"}, Default: "select"},
				"expression": {Type: "string", Title: "Expression", Format: "expression"},
			}),
			Required: []string{"op"},
		},
	},
	{
		Type:        "file.generate",
		Category:    "File",
		Label:       "Generate File",
		Description: "Produce XLSX / CSV / TXT from items.",
		Icon:        "FileSpreadsheet",
		Accent:      "text-success",
		Inputs:      []PortSpec{{ID: "in", Label: "Items"}},
		Outputs:     []PortSpec{{ID: "out", Label: "File"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"format":        {Type: "string", Title: "Format", Enum: []string{"xlsx", "csv", "txt"}, EnumLabels: []string{"Excel (XLSX)", "CSV", "Text"}, Default: "xlsx"},
				"filename":      {Type: "string", Title: "Filename", Format: "expression", Default: `report_{{ $flow.runDate | format:"20060102" }}.xlsx`},
				"onEmpty":       {Type: "string", Title: "On empty", Enum: []string{"skip", "emptyFile", "fail"}, Default: "skip"},
				"retentionDays": {Type: "number", Title: "Retention (days)", Minimum: fptr(1), Default: float64(30)},
			}),
			Required: []string{"format", "filename"},
			DependentSchemas: map[string]map[string]map[string]JSONSchema{
				"format": {
					"txt": {"delimiter": {Type: "string", Title: "Delimiter", Default: ","}},
				},
			},
		},
	},
	{
		Type:        "delivery.mft",
		Category:    "Delivery",
		Label:       "MFT / SFTP",
		Description: "Send file to an SFTP endpoint (atomic + retry).",
		Icon:        "Send",
		Accent:      "text-brand",
		Inputs:      []PortSpec{{ID: "in", Label: "File"}},
		Outputs:     []PortSpec{{ID: "out", Label: "Done"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"connectionId": {Type: "string", Title: "SFTP connection", Format: "connection", ConnectionType: "sftp"},
				"remotePath":   {Type: "string", Title: "Remote path", Format: "expression", Default: "/upload/"},
				"auth":         {Type: "string", Title: "Auth", Enum: []string{"password", "sshKey"}, Default: "sshKey"},
				"retries":      {Type: "number", Title: "Retries", Minimum: fptr(0), Maximum: fptr(10), Default: float64(3)},
			}),
			Required: []string{"connectionId", "remotePath"},
		},
	},
	{
		Type:        "delivery.email",
		Category:    "Delivery",
		Label:       "Email",
		Description: "Send email with the generated file attached.",
		Icon:        "Mail",
		Accent:      "text-brand",
		Inputs:      []PortSpec{{ID: "in", Label: "File"}},
		Outputs:     []PortSpec{{ID: "out", Label: "Done"}},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"transport": {Type: "string", Title: "Transport", Enum: []string{"smtp", "graph"}, EnumLabels: []string{"SMTP", "MS Graph"}, Default: "smtp"},
				"to":        {Type: "string", Title: "To", Format: "expression"},
				"subject":   {Type: "string", Title: "Subject", Format: "expression"},
				"body":      {Type: "string", Title: "Body", Format: "textarea"},
				"sendMode":  {Type: "string", Title: "Send mode", Enum: []string{"single", "perItem"}, Default: "single"},
			}),
			Required: []string{"to", "subject"},
		},
	},
	{
		Type:        "delivery.download",
		Category:    "Delivery",
		Label:       "Download",
		Description: "Make the file available in My Files (signed URL).",
		Icon:        "Download",
		Accent:      "text-brand",
		Inputs:      []PortSpec{{ID: "in", Label: "File"}},
		Outputs:     []PortSpec{},
		Schema: JSONSchema{
			Type: "object",
			Properties: props(map[string]JSONSchema{
				"expiryHours": {Type: "number", Title: "Link expiry (hours)", Minimum: fptr(1), Maximum: fptr(168), Default: float64(24)},
				"notify":      {Type: "boolean", Title: "Notify users"},
			}),
		},
	},
}

// All returns the node catalog. The slice is returned directly (callers only
// read it); it is package-level static data assembled once at init.
func All() []NodeType { return nodeTypes }
