package interpreter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/pkg/expression"
	"github.com/mywork/automate/pkg/filegen"
	"github.com/mywork/automate/pkg/filestore"
	"github.com/mywork/automate/pkg/mailer"
	"github.com/mywork/automate/pkg/templaterender"
)

// NodeExecRequest is the input to one node activity: the node identity/type, its
// opaque config, the items flowing in from the predecessor, the viewer roles
// (threaded through to masking in the db.query executor), and per-run Meta used
// as the sandboxed context for expression rendering (e.g. dynamic filenames).
type NodeExecRequest struct {
	NodeID      string           `json:"nodeId"`
	Type        string           `json:"type"`
	Config      json.RawMessage  `json:"config"`
	InItems     []map[string]any `json:"inItems"`
	ViewerRoles []string         `json:"viewerRoles"`
	// Meta carries run-scoped values available to expressions. It is optional
	// (nil in the base workflow path today); when present, Meta["params"] is
	// exposed to templates as $flow.params.* and Meta["vars"] as $vars.*.
	Meta map[string]any `json:"meta,omitempty"`
}

// NodeExecResult is a node's output. Decision is "" for plain nodes, or the
// branch label ("true"/"false") for a decision node (logic.if) so the workflow
// can select the matching out-edge.
type NodeExecResult struct {
	Items    []map[string]any `json:"items"`
	Meta     map[string]any   `json:"meta"`
	Decision string           `json:"decision"`
}

// MFTClient uploads a file to a remote MFT/SFTP endpoint. The executor depends
// on this seam so it is unit-testable with a fake; the real SFTP adapter (atomic
// .tmp→rename, path sanitisation, host-key verify) is wired in the worker.
type MFTClient interface {
	Upload(ctx context.Context, remotePath string, r io.Reader) error
}

// Activities holds the real per-node collaborators injected by the worker.
//   - deps: the live dbquery.Deps (Querier, Masking, Secrets, MaskPoint).
//   - fileStore: blob storage for file.generate output and delivery reads.
//   - mailer: SMTP (or other) sender for delivery.email.
//   - mft: MFT/SFTP client for delivery.mft.
//
// The file/delivery deps are optional at construction (nil until wired) so the
// existing db.query/logic paths keep working with the one-arg constructor; a
// node that needs a missing dep returns a clear error rather than panicking.
type Activities struct {
	deps      dbquery.Deps
	fileStore filestore.FileStore
	mailer    mailer.Sender
	mft       MFTClient
}

// Option configures optional Activities collaborators (functional options keep
// NewActivities(deps) working unchanged for callers that only run db.query).
type Option func(*Activities)

// WithFileStore injects the blob store used by file.generate and delivery nodes.
func WithFileStore(fs filestore.FileStore) Option {
	return func(a *Activities) { a.fileStore = fs }
}

// WithMailer injects the email sender used by delivery.email.
func WithMailer(s mailer.Sender) Option {
	return func(a *Activities) { a.mailer = s }
}

// WithMFT injects the MFT/SFTP client used by delivery.mft.
func WithMFT(c MFTClient) Option {
	return func(a *Activities) { a.mft = c }
}

// NewActivities builds the activity struct around a wired dbquery.Deps, plus any
// optional file/delivery collaborators supplied via options. The existing
// single-argument call site (NewActivities(deps)) remains valid.
func NewActivities(deps dbquery.Deps, opts ...Option) *Activities {
	a := &Activities{deps: deps}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// dbQueryConfig is the db.query node config shape. The Conn* dial fields are
// injected at run start by the API (resolving the node's connectionId to its
// stored connection); the executor uses them to dial the external database.
type dbQueryConfig struct {
	SQL        string `json:"sql"`
	ConnSecret string `json:"connSecret"`
	MaxRows    int    `json:"maxRows"`

	ConnHost     string `json:"connHost"`
	ConnPort     int    `json:"connPort"`
	ConnDatabase string `json:"connDatabase"`
	ConnUsername string `json:"connUsername"`
	ConnSSLMode  string `json:"connSslMode"`
}

// ifConfig is the logic.if node config: a simple {left op right} comparison.
// left is currently always "rowCount" (count of incoming items).
type ifConfig struct {
	Left  string  `json:"left"`
	Op    string  `json:"op"`
	Right float64 `json:"right"`
}

// fileGenConfig is the file.generate node config (spec 05 §3.1, 08 E7-S1).
type fileGenConfig struct {
	Format    string `json:"format"`    // xlsx|csv|txt
	Filename  string `json:"filename"`  // expression template
	Delimiter string `json:"delimiter"` // optional; CSV/TXT field separator
	OnEmpty   string `json:"onEmpty"`   // skip|emptyFile(=generateEmpty)|fail (default fail)
}

// emailConfig is the delivery.email node config (spec 05 §4.2).
type emailConfig struct {
	Transport string `json:"transport"` // smtp (only supported today)
	To        string `json:"to"`        // comma-separated recipients
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// downloadConfig is the delivery.download node config (spec 05 §4.3).
type downloadConfig struct {
	ExpiryHours int `json:"expiryHours"` // signed-URL TTL; <=0 => default
}

// mftConfig is the delivery.mft node config (spec 05 §4.1).
type mftConfig struct {
	RemotePath string `json:"remotePath"` // target dir or full path
}

// transformConfig is the logic.transform node config (spec 05, data.transform).
type transformConfig struct {
	Op     string   `json:"op"`     // select|filter (others pass through)
	Fields []string `json:"fields"` // columns for select/filter
}

// defaultSignedURLTTL is the signed-download validity when expiryHours is unset.
const defaultSignedURLTTL = 7 * 24 * time.Hour

// ExecuteNode dispatches one node by Type. It is the single activity registered
// on the worker; the interpreter workflow calls it once per node.
func (a *Activities) ExecuteNode(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	switch r.Type {
	case "db.query":
		return a.execDBQuery(ctx, r)
	case "logic.if":
		return execIf(r)
	case "logic.transform":
		return execTransform(r)
	case "file.generate":
		return a.execFileGenerate(ctx, r)
	case "delivery.email":
		return a.execDeliveryEmail(ctx, r)
	case "delivery.download":
		return a.execDeliveryDownload(ctx, r)
	case "delivery.mft":
		return a.execDeliveryMFT(ctx, r)
	case "trigger.manual", "noop":
		// Pass incoming items through with no side effects.
		return NodeExecResult{Items: r.InItems}, nil
	default:
		return NodeExecResult{}, fmt.Errorf("interpreter: unknown node type %q", r.Type)
	}
}

// execDBQuery unmarshals the db.query config and runs the verified executor with
// the worker-injected deps and the request's viewer roles.
func (a *Activities) execDBQuery(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	var cfg dbQueryConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: db.query bad config: %w", err)
	}
	out, err := dbquery.Execute(ctx, dbquery.Input{
		SQL:          cfg.SQL,
		MaxRows:      cfg.MaxRows,
		ViewerRoles:  r.ViewerRoles,
		ConnSecret:   cfg.ConnSecret,
		ConnHost:     cfg.ConnHost,
		ConnPort:     cfg.ConnPort,
		ConnDatabase: cfg.ConnDatabase,
		ConnUsername: cfg.ConnUsername,
		ConnSSLMode:  cfg.ConnSSLMode,
	}, a.deps)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: db.query node %q: %w", r.NodeID, err)
	}
	return NodeExecResult{
		Items: out.Items,
		Meta: map[string]any{
			"rowCount":  out.Meta.RowCount,
			"truncated": out.Meta.Truncated,
			"columns":   out.Meta.Columns,
		},
	}, nil
}

// execIf evaluates the simple condition and sets Decision to "true"/"false".
// Items pass through unchanged (the branch itself carries no transformation).
func execIf(r NodeExecRequest) (NodeExecResult, error) {
	var cfg ifConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: logic.if bad config: %w", err)
	}
	// The only supported left operand today is the incoming item count.
	left := float64(len(r.InItems))
	pass, err := compare(left, cfg.Op, cfg.Right)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: logic.if node %q: %w", r.NodeID, err)
	}
	decision := "false"
	if pass {
		decision = "true"
	}
	return NodeExecResult{Items: r.InItems, Decision: decision}, nil
}

// execTransform applies a select/filter projection to the incoming items. An
// unknown op passes the items through unchanged (forward-compatible).
func execTransform(r NodeExecRequest) (NodeExecResult, error) {
	var cfg transformConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: logic.transform bad config: %w", err)
	}
	switch cfg.Op {
	case "select":
		return NodeExecResult{Items: transformSelect(r.InItems, cfg.Fields)}, nil
	case "filter":
		return NodeExecResult{Items: transformFilter(r.InItems, cfg.Fields)}, nil
	default:
		// Pass-through for unimplemented ops (e.g. sort/rename land later).
		return NodeExecResult{Items: r.InItems}, nil
	}
}

// transformSelect keeps only the listed fields on each item.
func transformSelect(items []map[string]any, fields []string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := make(map[string]any, len(fields))
		for _, f := range fields {
			if v, ok := it[f]; ok {
				row[f] = v
			}
		}
		out = append(out, row)
	}
	return out
}

// transformFilter drops items where ANY listed field is empty (missing, nil or
// an empty string). With no fields configured, all items pass.
func transformFilter(items []map[string]any, fields []string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if allFieldsPresent(it, fields) {
			out = append(out, it)
		}
	}
	return out
}

// allFieldsPresent reports whether every field is present and non-empty on it.
func allFieldsPresent(it map[string]any, fields []string) bool {
	for _, f := range fields {
		v, ok := it[f]
		if !ok || v == nil {
			return false
		}
		if s, isStr := v.(string); isStr && s == "" {
			return false
		}
	}
	return true
}

// execFileGenerate renders the dynamic filename, applies the onEmpty policy,
// writes the items to bytes in the requested format, stores them via the
// FileStore under a run-scoped key, and returns one file-descriptor item.
func (a *Activities) execFileGenerate(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	if a.fileStore == nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: file.generate node %q: no file store configured", r.NodeID)
	}
	var cfg fileGenConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: file.generate bad config: %w", err)
	}

	// Empty-input policy (default: fail).
	if len(r.InItems) == 0 {
		switch cfg.OnEmpty {
		case "skip":
			return NodeExecResult{Items: nil}, nil
		case "emptyFile", "generateEmpty":
			// fall through and write a header-only file
		default: // "fail" or unset
			return NodeExecResult{}, fmt.Errorf("interpreter: file.generate node %q: no rows and onEmpty=fail", r.NodeID)
		}
	}

	filename, err := renderTemplate(cfg.Filename, r.Meta)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: file.generate node %q filename: %w", r.NodeID, err)
	}

	cols := filegen.Columns(r.InItems)
	buf, err := writeFormat(cfg.Format, cols, r.InItems, cfg.Delimiter)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: file.generate node %q: %w", r.NodeID, err)
	}

	key := path.Join("runs", r.NodeID, filename)
	ref, err := a.fileStore.Put(ctx, key, buf)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: file.generate node %q store: %w", r.NodeID, err)
	}

	desc := map[string]any{
		"filename": filename,
		"path":     ref.Path,
		"size":     ref.Size,
		"checksum": ref.Checksum,
		"format":   cfg.Format,
		"rowCount": len(r.InItems),
	}
	return NodeExecResult{
		Items: []map[string]any{desc},
		Meta:  map[string]any{"rowCount": len(r.InItems), "path": ref.Path},
	}, nil
}

// writeFormat writes items to a buffer in the given format, returning it as a
// reader for FileStore.Put. delimiter overrides the default separator for
// csv/txt when non-empty. The writer error is returned uniformly; for the
// in-memory buffer here only WriteXLSX can realistically fail (e.g. a corrupt
// workbook), but all three propagate any error.
func writeFormat(format string, cols []string, items []map[string]any, delimiter string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	var err error
	switch format {
	case "csv":
		err = filegen.WriteCSV(&buf, cols, items, csvDelimiter(delimiter))
	case "txt":
		err = filegen.WriteTXT(&buf, cols, items, txtDelimiter(delimiter))
	case "xlsx":
		err = filegen.WriteXLSX(&buf, cols, items, "Sheet1")
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	if err != nil {
		return nil, err
	}
	return &buf, nil
}

// csvDelimiter maps the config delimiter (a string) to a rune, defaulting to ','.
func csvDelimiter(d string) rune {
	if d == "" {
		return ','
	}
	return []rune(d)[0]
}

// txtDelimiter defaults the TXT field separator to '|' when unset.
func txtDelimiter(d string) string {
	if d == "" {
		return "|"
	}
	return d
}

// execDeliveryEmail reads the file descriptor from InItems[0], fetches the bytes
// from the FileStore, builds a Message with the file attached, and sends it. The
// descriptor passes through so downstream delivery branches can reuse it.
func (a *Activities) execDeliveryEmail(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	if a.mailer == nil || a.fileStore == nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email node %q: mailer/file store not configured", r.NodeID)
	}
	var cfg emailConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email bad config: %w", err)
	}
	desc, err := firstDescriptor(r.InItems)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email node %q: %w", r.NodeID, err)
	}

	to := splitRecipients(cfg.To)
	if len(to) == 0 {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email node %q: no recipients", r.NodeID)
	}

	data, err := a.readFile(ctx, descString(desc, "path"))
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email node %q: %w", r.NodeID, err)
	}

	msg := mailer.Message{
		To:             to,
		Subject:        cfg.Subject,
		Body:           cfg.Body,
		AttachmentName: descString(desc, "filename"),
		Attachment:     data,
	}
	if err := a.mailer.Send(ctx, msg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.email node %q: send: %w", r.NodeID, err)
	}
	return NodeExecResult{Items: []map[string]any{desc}}, nil
}

// execDeliveryDownload produces a signed URL for the descriptor's file and
// returns the descriptor augmented with a downloadUrl field.
func (a *Activities) execDeliveryDownload(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	if a.fileStore == nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.download node %q: no file store configured", r.NodeID)
	}
	var cfg downloadConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.download bad config: %w", err)
	}
	desc, err := firstDescriptor(r.InItems)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.download node %q: %w", r.NodeID, err)
	}

	ttl := defaultSignedURLTTL
	if cfg.ExpiryHours > 0 {
		ttl = time.Duration(cfg.ExpiryHours) * time.Hour
	}
	url, err := a.fileStore.SignedURL(ctx, descString(desc, "path"), ttl)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.download node %q: sign: %w", r.NodeID, err)
	}

	out := cloneDescriptor(desc)
	out["downloadUrl"] = url
	return NodeExecResult{Items: []map[string]any{out}}, nil
}

// execDeliveryMFT reads the descriptor, fetches the bytes, and uploads them to
// the remote path via the MFTClient. When remotePath ends in '/', the file's
// name is appended. The descriptor passes through.
func (a *Activities) execDeliveryMFT(ctx context.Context, r NodeExecRequest) (NodeExecResult, error) {
	if a.mft == nil || a.fileStore == nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft node %q: mft/file store not configured", r.NodeID)
	}
	var cfg mftConfig
	if err := json.Unmarshal(r.Config, &cfg); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft bad config: %w", err)
	}
	desc, err := firstDescriptor(r.InItems)
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft node %q: %w", r.NodeID, err)
	}

	remote, err := resolveRemotePath(cfg.RemotePath, descString(desc, "filename"))
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft node %q: %w", r.NodeID, err)
	}

	data, err := a.readFile(ctx, descString(desc, "path"))
	if err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft node %q: %w", r.NodeID, err)
	}
	if err := a.mft.Upload(ctx, remote, bytes.NewReader(data)); err != nil {
		return NodeExecResult{}, fmt.Errorf("interpreter: delivery.mft node %q: upload: %w", r.NodeID, err)
	}
	return NodeExecResult{Items: []map[string]any{desc}}, nil
}

// resolveRemotePath sanitises the configured remote path and, when it names a
// directory (empty or trailing '/'), appends the file's base name.
func resolveRemotePath(remotePath, filename string) (string, error) {
	rp := strings.TrimSpace(remotePath)
	if rp == "" {
		return "", fmt.Errorf("remotePath is required")
	}
	if strings.HasSuffix(rp, "/") {
		rp += path.Base(filename)
	}
	// Reject traversal in the resolved path (defence-in-depth; the SFTP adapter
	// also confines writes server-side).
	if strings.Contains(rp, "..") {
		return "", fmt.Errorf("remotePath %q contains '..'", remotePath)
	}
	return rp, nil
}

// readFile fetches an object's bytes via the FileStore and closes the reader.
func (a *Activities) readFile(ctx context.Context, key string) ([]byte, error) {
	rc, err := a.fileStore.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get %q: %w", key, err)
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// firstDescriptor returns InItems[0] as the file descriptor, erroring when no
// input file flowed in.
func firstDescriptor(items []map[string]any) (map[string]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no input file descriptor")
	}
	return items[0], nil
}

// cloneDescriptor makes a shallow copy so augmenting the output does not mutate
// the shared input map.
func cloneDescriptor(d map[string]any) map[string]any {
	out := make(map[string]any, len(d)+1)
	for k, v := range d {
		out[k] = v
	}
	return out
}

// descString reads a string field from a descriptor, tolerating a missing key.
func descString(d map[string]any, key string) string {
	if v, ok := d[key].(string); ok {
		return v
	}
	return ""
}

// splitRecipients splits a comma-separated recipient list, trimming blanks.
func splitRecipients(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// renderTemplate renders a filename/expression template against a minimal
// EvalContext built from the request Meta (Meta["params"] => $flow.params,
// Meta["vars"] => $vars). A plain literal (no placeholders) renders to itself.
func renderTemplate(tmpl string, meta map[string]any) (string, error) {
	ec := expression.EvalContext{}
	if params, ok := meta["params"].(map[string]any); ok {
		ec.Flow.Params = params
	}
	if vars, ok := meta["vars"].(map[string]any); ok {
		ec.Vars = vars
	}
	return templaterender.Render(tmpl, ec)
}

// compare evaluates left <op> right for the supported comparison operators.
func compare(left float64, op string, right float64) (bool, error) {
	switch op {
	case ">":
		return left > right, nil
	case ">=":
		return left >= right, nil
	case "<":
		return left < right, nil
	case "<=":
		return left <= right, nil
	case "==":
		return left == right, nil
	case "!=":
		return left != right, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", op)
	}
}
