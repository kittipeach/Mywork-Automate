package interpreter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
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
	})
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
		name     string
		in       []map[string]any
		wantDec  string
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

// Compile-time proof that secrets.Resolver is the interface we stub.
var _ secrets.Resolver = stubResolver{}
