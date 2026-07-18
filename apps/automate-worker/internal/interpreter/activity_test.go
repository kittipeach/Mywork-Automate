package interpreter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/pkg/filestore"
	"github.com/mywork/automate/pkg/mailer"
	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
)

// stubQuerier returns a fixed RowSet, implementing dbquery.Querier.
type stubQuerier struct {
	rs  dbquery.RowSet
	err error
}

func (s stubQuerier) Query(_ context.Context, _ string, _ []any) (dbquery.RowSet, error) {
	return s.rs, s.err
}

// stubResolver satisfies secrets.Resolver without touching disk/Key Vault.
type stubResolver struct{}

func (stubResolver) Resolve(_ context.Context, _ string) (string, error) { return "dsn", nil }

func newTestActivities(t *testing.T, q dbquery.Querier) *Activities {
	t.Helper()
	eng, err := masking.NewEngine(masking.DefaultRules())
	require.NoError(t, err)
	return NewActivities(dbquery.Deps{
		Secrets:   stubResolver{},
		Querier:   q,
		Masking:   eng,
		MaskPoint: masking.PointPreview,
	}, WithAllowRawSQL(true)) // existing tests exercise the raw-SQL execution path
}

// --- Fakes for the file/delivery node deps ---

// fakeFileStore is an in-memory filestore.FileStore for delivery/file tests.
type fakeFileStore struct {
	mu        sync.Mutex
	objects   map[string][]byte
	putErr    error
	getErr    error
	signErr   error
	signedURL string
	lastPut   string
}

func newFakeFileStore() *fakeFileStore {
	return &fakeFileStore{objects: map[string][]byte{}, signedURL: "https://signed/url"}
}

func (f *fakeFileStore) Put(_ context.Context, path string, r io.Reader) (filestore.FileRef, error) {
	if f.putErr != nil {
		return filestore.FileRef{}, f.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return filestore.FileRef{}, err
	}
	f.mu.Lock()
	f.objects[path] = b
	f.lastPut = path
	f.mu.Unlock()
	return filestore.FileRef{Path: path, Size: int64(len(b)), Checksum: "sha-" + path}, nil
}

func (f *fakeFileStore) Get(_ context.Context, path string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.mu.Lock()
	b, ok := f.objects[path]
	f.mu.Unlock()
	if !ok {
		return nil, filestore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeFileStore) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	if f.signErr != nil {
		return "", f.signErr
	}
	return f.signedURL, nil
}

func (f *fakeFileStore) Delete(_ context.Context, path string) error {
	f.mu.Lock()
	delete(f.objects, path)
	f.mu.Unlock()
	return nil
}

// fakeSender records the last sent message (or fails).
type fakeSender struct {
	sent []mailer.Message
	err  error
}

func (s *fakeSender) Send(_ context.Context, m mailer.Message) error {
	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, m)
	return nil
}

// fakeMFT records uploads (or fails).
type fakeMFT struct {
	uploads map[string][]byte
	err     error
}

func newFakeMFT() *fakeMFT { return &fakeMFT{uploads: map[string][]byte{}} }

func (m *fakeMFT) Upload(_ context.Context, remotePath string, r io.Reader) error {
	if m.err != nil {
		return m.err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.uploads[remotePath] = b
	return nil
}

// newDeliveryActivities builds Activities wired with the delivery/file deps.
func newDeliveryActivities(t *testing.T, fs filestore.FileStore, snd mailer.Sender, mft MFTClient) *Activities {
	t.Helper()
	eng, err := masking.NewEngine(masking.DefaultRules())
	require.NoError(t, err)
	return NewActivities(
		dbquery.Deps{Secrets: stubResolver{}, Querier: stubQuerier{}, Masking: eng, MaskPoint: masking.PointPreview},
		WithFileStore(fs),
		WithMailer(snd),
		WithMFT(mft),
	)
}

func TestExecuteNode_DBQuery(t *testing.T) {
	q := stubQuerier{rs: dbquery.RowSet{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{1, "alice"}, {2, "bob"}},
	}}
	a := newTestActivities(t, q)

	cfg, _ := json.Marshal(map[string]any{"sql": "SELECT id, name FROM t", "connSecret": "conn-1", "maxRows": 10})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "q", Type: "db.query", Config: cfg, ViewerRoles: []string{"analyst"},
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, 2, res.Meta["rowCount"])
	require.Empty(t, res.Decision)
}

func TestExecuteNode_DBQuery_QuerierError(t *testing.T) {
	a := newTestActivities(t, stubQuerier{err: context.DeadlineExceeded})
	cfg, _ := json.Marshal(map[string]any{"sql": "SELECT id FROM t"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "q", Type: "db.query", Config: cfg})
	require.Error(t, err)
}

func TestExecuteNode_DBQuery_BadConfig(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "q", Type: "db.query", Config: json.RawMessage(`{"sql":123}`),
	})
	require.Error(t, err)
}

func TestExecuteNode_LogicIf(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	cfg, _ := json.Marshal(map[string]any{"left": "rowCount", "op": ">", "right": 0})

	tests := []struct {
		name    string
		in      []map[string]any
		wantDec string
	}{
		{"has rows -> true", []map[string]any{{"a": 1}}, "true"},
		{"no rows -> false", nil, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
				NodeID: "if", Type: "logic.if", Config: cfg, InItems: tt.in,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantDec, res.Decision)
			// items pass through unchanged.
			require.Equal(t, tt.in, res.Items)
		})
	}
}

func TestExecuteNode_LogicIf_Operators(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	in := []map[string]any{{"a": 1}, {"a": 2}} // rowCount = 2
	tests := []struct {
		op    string
		right float64
		want  string
	}{
		{">", 1, "true"},
		{">", 5, "false"},
		{">=", 2, "true"},
		{"<", 3, "true"},
		{"<=", 2, "true"},
		{"==", 2, "true"},
		{"!=", 2, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			cfg, _ := json.Marshal(map[string]any{"left": "rowCount", "op": tt.op, "right": tt.right})
			res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
				NodeID: "if", Type: "logic.if", Config: cfg, InItems: in,
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, res.Decision)
		})
	}
}

func TestExecuteNode_LogicIf_BadConfig(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "if", Type: "logic.if", Config: json.RawMessage(`{bad`),
	})
	require.Error(t, err)
}

func TestExecuteNode_LogicIf_UnknownOp(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	cfg, _ := json.Marshal(map[string]any{"left": "rowCount", "op": "~=", "right": 0})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "if", Type: "logic.if", Config: cfg,
	})
	require.Error(t, err)
}

func TestExecuteNode_PassThrough(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	in := []map[string]any{{"x": 1}}
	for _, typ := range []string{"trigger.manual", "noop"} {
		t.Run(typ, func(t *testing.T) {
			res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
				NodeID: "n", Type: typ, InItems: in,
			})
			require.NoError(t, err)
			require.Equal(t, in, res.Items)
			require.Empty(t, res.Decision)
		})
	}
}

func TestExecuteNode_UnknownType(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "n", Type: "does.not.exist"})
	require.Error(t, err)
}

// --- file.generate ---

func TestExecuteNode_FileGenerate_CSV(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"id": 1, "name": "alice"}, {"id": 2, "name": "bob"}}

	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "payroll.csv"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	d := res.Items[0]
	require.Equal(t, "payroll.csv", d["filename"])
	require.Equal(t, "csv", d["format"])
	require.Equal(t, 2, d["rowCount"])
	require.NotEmpty(t, d["path"])
	require.NotEmpty(t, d["checksum"])
	require.Greater(t, d["size"].(int64), int64(0))

	// The bytes were actually stored and contain the header + rows.
	stored := string(fs.objects[d["path"].(string)])
	require.Contains(t, stored, "id,name")
	require.Contains(t, stored, "alice")
}

func TestExecuteNode_FileGenerate_XLSX(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": 1}}
	cfg, _ := json.Marshal(map[string]any{"format": "xlsx", "filename": "out.xlsx"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	require.Equal(t, "xlsx", res.Items[0]["format"])
	require.NotEmpty(t, fs.objects[res.Items[0]["path"].(string)])
}

func TestExecuteNode_FileGenerate_TXTCustomDelimiter(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": "x", "b": "y"}}
	cfg, _ := json.Marshal(map[string]any{"format": "txt", "filename": "out.txt", "delimiter": "|"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	stored := string(fs.objects[res.Items[0]["path"].(string)])
	require.Contains(t, stored, "a|b")
	require.Contains(t, stored, "x|y")
}

func TestExecuteNode_FileGenerate_DynamicFilename(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": 1}}
	cfg, _ := json.Marshal(map[string]any{
		"format":   "csv",
		"filename": `report_{{ $flow.params.period }}.csv`,
	})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
		Meta: map[string]any{"params": map[string]any{"period": "202607"}},
	})
	require.NoError(t, err)
	require.Equal(t, "report_202607.csv", res.Items[0]["filename"])
}

func TestExecuteNode_FileGenerate_OnEmptySkip(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "x.csv", "onEmpty": "skip"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: nil,
	})
	require.NoError(t, err)
	require.Empty(t, res.Items)
	require.Empty(t, fs.objects) // nothing written
}

func TestExecuteNode_FileGenerate_OnEmptyFail(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "x.csv", "onEmpty": "fail"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: nil,
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_OnEmptyEmptyFile(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	// both spellings supported: emptyFile / generateEmpty
	for _, policy := range []string{"emptyFile", "generateEmpty"} {
		cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "x.csv", "onEmpty": policy})
		res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
			NodeID: "gen", Type: "file.generate", Config: cfg, InItems: nil,
		})
		require.NoError(t, err)
		require.Len(t, res.Items, 1)
		require.Equal(t, 0, res.Items[0]["rowCount"])
	}
}

func TestExecuteNode_FileGenerate_DefaultOnEmptyIsFail(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "x.csv"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: nil,
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_BadConfig(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: json.RawMessage(`{bad`), InItems: []map[string]any{{"a": 1}},
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_UnknownFormat(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "pdf", "filename": "x.pdf"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: []map[string]any{{"a": 1}},
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_BadFilenameExpr(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "{{ unterminated"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: []map[string]any{{"a": 1}},
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_PutError(t *testing.T) {
	fs := newFakeFileStore()
	fs.putErr = errors.New("disk full")
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "x.csv"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: []map[string]any{{"a": 1}},
	})
	require.Error(t, err)
}

// --- delivery.email ---

func TestExecuteNode_DeliveryEmail(t *testing.T) {
	fs := newFakeFileStore()
	snd := &fakeSender{}
	a := newDeliveryActivities(t, fs, snd, newFakeMFT())

	// Seed a stored file + descriptor.
	desc := seedFile(t, fs, "runs/r1/payroll.csv", "id,name\n1,alice\n", "payroll.csv", "csv", 1)

	cfg, _ := json.Marshal(map[string]any{
		"transport": "smtp",
		"to":        "a@x, b@x",
		"subject":   "Payroll",
		"body":      "See attached",
	})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.Len(t, snd.sent, 1)
	m := snd.sent[0]
	require.Equal(t, []string{"a@x", "b@x"}, m.To)
	require.Equal(t, "Payroll", m.Subject)
	require.Equal(t, "payroll.csv", m.AttachmentName)
	require.Equal(t, "id,name\n1,alice\n", string(m.Attachment))
	// descriptor passes through
	require.Equal(t, desc, res.Items[0])
}

func TestExecuteNode_DeliveryEmail_NoInput(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"to": "a@x", "subject": "s", "body": "b"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: cfg, InItems: nil,
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryEmail_GetError(t *testing.T) {
	fs := newFakeFileStore()
	fs.getErr = errors.New("not found")
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := map[string]any{"filename": "x.csv", "path": "runs/r1/x.csv"}
	cfg, _ := json.Marshal(map[string]any{"to": "a@x", "subject": "s", "body": "b"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryEmail_SendError(t *testing.T) {
	fs := newFakeFileStore()
	snd := &fakeSender{err: errors.New("smtp down")}
	a := newDeliveryActivities(t, fs, snd, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x.csv", "data", "x.csv", "csv", 1)
	cfg, _ := json.Marshal(map[string]any{"to": "a@x", "subject": "s", "body": "b"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryEmail_NoRecipients(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x.csv", "data", "x.csv", "csv", 1)
	cfg, _ := json.Marshal(map[string]any{"to": "", "subject": "s", "body": "b"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryEmail_BadConfig(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "email", Type: "delivery.email", Config: json.RawMessage(`{bad`),
		InItems: []map[string]any{{"path": "p"}},
	})
	require.Error(t, err)
}

// --- delivery.download ---

func TestExecuteNode_DeliveryDownload(t *testing.T) {
	fs := newFakeFileStore()
	fs.signedURL = "https://files/signed?token=abc"
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x.csv", "data", "x.csv", "csv", 1)

	cfg, _ := json.Marshal(map[string]any{"expiryHours": 24})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, "https://files/signed?token=abc", res.Items[0]["downloadUrl"])
	// original descriptor fields preserved
	require.Equal(t, "x.csv", res.Items[0]["filename"])
}

func TestExecuteNode_DeliveryDownload_DefaultExpiry(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x.csv", "data", "x.csv", "csv", 1)
	cfg, _ := json.Marshal(map[string]any{}) // no expiryHours -> default
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items[0]["downloadUrl"])
}

func TestExecuteNode_DeliveryDownload_NoInput(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"expiryHours": 1})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: cfg, InItems: nil,
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryDownload_SignError(t *testing.T) {
	fs := newFakeFileStore()
	fs.signErr = errors.New("cannot sign")
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x.csv", "data", "x.csv", "csv", 1)
	cfg, _ := json.Marshal(map[string]any{"expiryHours": 1})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryDownload_BadConfig(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: json.RawMessage(`{bad`),
		InItems: []map[string]any{{"path": "p"}},
	})
	require.Error(t, err)
}

// --- delivery.mft ---

func TestExecuteNode_DeliveryMFT(t *testing.T) {
	fs := newFakeFileStore()
	mft := newFakeMFT()
	a := newDeliveryActivities(t, fs, &fakeSender{}, mft)
	desc := seedFile(t, fs, "runs/r1/payroll.txt", "H1\nrow\nT1\n", "payroll.txt", "txt", 1)

	cfg, _ := json.Marshal(map[string]any{"remotePath": "/incoming/payroll.txt"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.Equal(t, "H1\nrow\nT1\n", string(mft.uploads["/incoming/payroll.txt"]))
	require.Equal(t, desc, res.Items[0]) // passthrough
}

func TestExecuteNode_DeliveryMFT_DefaultRemotePathUsesFilename(t *testing.T) {
	fs := newFakeFileStore()
	mft := newFakeMFT()
	a := newDeliveryActivities(t, fs, &fakeSender{}, mft)
	desc := seedFile(t, fs, "runs/r1/payroll.txt", "data", "payroll.txt", "txt", 1)
	cfg, _ := json.Marshal(map[string]any{"remotePath": "/incoming/"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.Contains(t, mft.uploads, "/incoming/payroll.txt")
}

func TestExecuteNode_DeliveryMFT_NoInput(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	cfg, _ := json.Marshal(map[string]any{"remotePath": "/x"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: nil,
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryMFT_GetError(t *testing.T) {
	fs := newFakeFileStore()
	fs.getErr = errors.New("gone")
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := map[string]any{"filename": "x", "path": "runs/r1/x"}
	cfg, _ := json.Marshal(map[string]any{"remotePath": "/x"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryMFT_UploadError(t *testing.T) {
	fs := newFakeFileStore()
	mft := newFakeMFT()
	mft.err = errors.New("connection reset")
	a := newDeliveryActivities(t, fs, &fakeSender{}, mft)
	desc := seedFile(t, fs, "runs/r1/x", "data", "x", "txt", 1)
	cfg, _ := json.Marshal(map[string]any{"remotePath": "/x"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryMFT_EmptyRemotePath(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x", "data", "x", "txt", 1)
	cfg, _ := json.Marshal(map[string]any{"remotePath": ""})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryMFT_BadConfig(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: json.RawMessage(`{bad`),
		InItems: []map[string]any{{"path": "p"}},
	})
	require.Error(t, err)
}

// --- logic.transform ---

func TestExecuteNode_LogicTransform_Select(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	in := []map[string]any{
		{"id": 1, "name": "alice", "secret": "x"},
		{"id": 2, "name": "bob", "secret": "y"},
	}
	cfg, _ := json.Marshal(map[string]any{"op": "select", "fields": []string{"id", "name"}})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "t", Type: "logic.transform", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, map[string]any{"id": 1, "name": "alice"}, res.Items[0])
	require.NotContains(t, res.Items[0], "secret")
}

func TestExecuteNode_LogicTransform_Filter(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	in := []map[string]any{
		{"id": 1, "email": "a@x"},
		{"id": 2, "email": ""},
		{"id": 3, "email": nil},
		{"id": 4}, // missing key
		{"id": 5, "email": "b@x"},
	}
	cfg, _ := json.Marshal(map[string]any{"op": "filter", "fields": []string{"email"}})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "t", Type: "logic.transform", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Equal(t, 1, res.Items[0]["id"])
	require.Equal(t, 5, res.Items[1]["id"])
}

func TestExecuteNode_LogicTransform_UnknownOpPassThrough(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": 1}}
	cfg, _ := json.Marshal(map[string]any{"op": "sort"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "t", Type: "logic.transform", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	require.Equal(t, in, res.Items)
}

func TestExecuteNode_LogicTransform_BadConfig(t *testing.T) {
	a := newDeliveryActivities(t, newFakeFileStore(), &fakeSender{}, newFakeMFT())
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "t", Type: "logic.transform", Config: json.RawMessage(`{bad`),
	})
	require.Error(t, err)
}

func TestExecuteNode_FileGenerate_CSVCustomDelimiter(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": "x", "b": "y"}}
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": "out.csv", "delimiter": ";"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	stored := string(fs.objects[res.Items[0]["path"].(string)])
	require.Contains(t, stored, "a;b")
}

func TestExecuteNode_FileGenerate_TXTDefaultDelimiter(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": "x", "b": "y"}}
	// No delimiter -> default '|'.
	cfg, _ := json.Marshal(map[string]any{"format": "txt", "filename": "out.txt"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	stored := string(fs.objects[res.Items[0]["path"].(string)])
	require.Contains(t, stored, "a|b")
}

func TestExecuteNode_FileGenerate_TXTNonDefaultDelimiter(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": "x", "b": "y"}}
	cfg, _ := json.Marshal(map[string]any{"format": "txt", "filename": "out.txt", "delimiter": "~"})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
	})
	require.NoError(t, err)
	stored := string(fs.objects[res.Items[0]["path"].(string)])
	require.Contains(t, stored, "a~b")
}

func TestExecuteNode_FileGenerate_DynamicFilenameVars(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	in := []map[string]any{{"a": 1}}
	cfg, _ := json.Marshal(map[string]any{"format": "csv", "filename": `f_{{ $vars.tag }}.csv`})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "gen", Type: "file.generate", Config: cfg, InItems: in,
		Meta: map[string]any{"vars": map[string]any{"tag": "abc"}},
	})
	require.NoError(t, err)
	require.Equal(t, "f_abc.csv", res.Items[0]["filename"])
}

// --- missing-dep guards ---

func TestExecuteNode_NodesRequireDeps(t *testing.T) {
	// Activities with no file/mailer/mft deps wired.
	eng, err := masking.NewEngine(masking.DefaultRules())
	require.NoError(t, err)
	bare := NewActivities(dbquery.Deps{Secrets: stubResolver{}, Querier: stubQuerier{}, Masking: eng, MaskPoint: masking.PointPreview})

	desc := map[string]any{"path": "p", "filename": "f"}
	cases := []struct {
		typ string
		cfg map[string]any
		in  []map[string]any
	}{
		{"file.generate", map[string]any{"format": "csv", "filename": "x.csv"}, []map[string]any{{"a": 1}}},
		{"delivery.email", map[string]any{"to": "a@x"}, []map[string]any{desc}},
		{"delivery.download", map[string]any{"expiryHours": 1}, []map[string]any{desc}},
		{"delivery.mft", map[string]any{"remotePath": "/x"}, []map[string]any{desc}},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			cfg, _ := json.Marshal(c.cfg)
			_, err := bare.ExecuteNode(context.Background(), NodeExecRequest{
				NodeID: "n", Type: c.typ, Config: cfg, InItems: c.in,
			})
			require.Error(t, err)
		})
	}
}

func TestExecuteNode_DeliveryMFT_TraversalRejected(t *testing.T) {
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := seedFile(t, fs, "runs/r1/x", "data", "x", "txt", 1)
	cfg, _ := json.Marshal(map[string]any{"remotePath": "/incoming/../../etc/passwd"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "mft", Type: "delivery.mft", Config: cfg, InItems: []map[string]any{desc},
	})
	require.Error(t, err)
}

func TestExecuteNode_DeliveryDownload_MissingPathField(t *testing.T) {
	// Descriptor lacking a "path" -> descString returns "" -> SignedURL called
	// with "". The fake tolerates it; this covers descString's missing-key path.
	fs := newFakeFileStore()
	a := newDeliveryActivities(t, fs, &fakeSender{}, newFakeMFT())
	desc := map[string]any{"filename": "x.csv"} // no path
	cfg, _ := json.Marshal(map[string]any{"expiryHours": 1})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{
		NodeID: "dl", Type: "delivery.download", Config: cfg, InItems: []map[string]any{desc},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items[0]["downloadUrl"])
}

// seedFile writes bytes to fs and returns the matching descriptor item.
func seedFile(t *testing.T, fs *fakeFileStore, path, content, filename, format string, rowCount int) map[string]any {
	t.Helper()
	ref, err := fs.Put(context.Background(), path, strings.NewReader(content))
	require.NoError(t, err)
	return map[string]any{
		"filename": filename,
		"path":     ref.Path,
		"size":     ref.Size,
		"checksum": ref.Checksum,
		"format":   format,
		"rowCount": rowCount,
	}
}

// Compile-time proof that secrets.Resolver is the interface we stub.
var _ secrets.Resolver = stubResolver{}

func TestExecuteNode_TriggerPassThrough(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	in := []map[string]any{{"n": 1}}
	for _, typ := range []string{"trigger.manual", "trigger.schedule", "noop"} {
		res, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "t", Type: typ, InItems: in})
		require.NoError(t, err, "type %s should pass through, not error", typ)
		require.Equal(t, in, res.Items, "type %s should pass items through unchanged", typ)
	}
}

// --- E6-S2 hardening: builder-only query policy ---

func TestExecDBQuery_BuilderSpecCompiles(t *testing.T) {
	// The querier records the SQL it is handed so we can assert the server built
	// it from the structured spec (not from any client SQL).
	var gotSQL string
	var gotArgs []any
	rec := recordingQuerier{rs: dbquery.RowSet{Columns: []string{"name"}, Rows: [][]any{{"a"}}}, sql: &gotSQL, args: &gotArgs}
	a := newTestActivities(t, rec)

	cfg, _ := json.Marshal(map[string]any{
		"mode":    "builder",
		"maxRows": 100,
		"builder": map[string]any{
			"table":   "employees",
			"columns": []map[string]any{{"name": "name"}, {"name": "salary"}},
			"where":   []map[string]any{{"column": "salary", "op": ">=", "value": 50000}},
			"limit":   10,
		},
	})
	res, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "q", Type: "db.query", Config: cfg})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, `SELECT "name", "salary" FROM "employees" WHERE "salary" >= $1 LIMIT 10`, gotSQL)
	require.Equal(t, []any{float64(50000)}, gotArgs) // JSON numbers decode to float64
}

func TestExecDBQuery_RawSQLDisabledByDefault(t *testing.T) {
	// Default policy (no WithAllowRawSQL): a node carrying hand-written SQL is
	// rejected — no free query reaches the database.
	a := NewActivities(dbquery.Deps{Secrets: stubResolver{}, Querier: stubQuerier{}, Masking: mustEngine(t), MaskPoint: masking.PointPreview})
	cfg, _ := json.Marshal(map[string]any{"mode": "sql", "sql": "SELECT * FROM employees"})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "q", Type: "db.query", Config: cfg})
	require.Error(t, err)
	require.ErrorIs(t, err, errRawSQLDisabled)
}

func TestExecDBQuery_NoQuerySpec(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	cfg, _ := json.Marshal(map[string]any{"maxRows": 10}) // no builder, no sql
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "q", Type: "db.query", Config: cfg})
	require.Error(t, err)
}

func TestExecDBQuery_BuilderRejectsInjection(t *testing.T) {
	a := newTestActivities(t, stubQuerier{})
	cfg, _ := json.Marshal(map[string]any{
		"mode":    "builder",
		"builder": map[string]any{"table": "employees", "columns": []map[string]any{{"name": "name; DROP TABLE x"}}},
	})
	_, err := a.ExecuteNode(context.Background(), NodeExecRequest{NodeID: "q", Type: "db.query", Config: cfg})
	require.Error(t, err) // sqlbuilder rejects the malicious identifier
}

func mustEngine(t *testing.T) *masking.Engine {
	t.Helper()
	e, err := masking.NewEngine(masking.DefaultRules())
	require.NoError(t, err)
	return e
}

// recordingQuerier captures the SQL/args passed to Query.
type recordingQuerier struct {
	rs   dbquery.RowSet
	sql  *string
	args *[]any
}

func (r recordingQuerier) Query(_ context.Context, sql string, args []any) (dbquery.RowSet, error) {
	*r.sql = sql
	*r.args = args
	return r.rs, nil
}
