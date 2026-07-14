package sqlguard

import (
	"errors"
	"testing"
)

// validCases are inputs that MUST pass Validate (return nil).
// Each is a single, read-only SELECT statement with no side-effecting construct
// anywhere in the tree.
var validCases = []struct {
	name string
	sql  string
}{
	{"simple_select", "SELECT 1"},
	{"select_columns", "SELECT id, name FROM users"},
	{"select_star", "SELECT * FROM accounts"},
	{"where_clause", "SELECT id FROM users WHERE active = true"},
	{"param_bind_single", "SELECT id FROM users WHERE id = $1"},
	{"param_bind_multi", "SELECT id FROM users WHERE tenant = $1 AND status = $2"},
	{"inner_join", "SELECT u.id, a.balance FROM users u JOIN accounts a ON a.user_id = u.id"},
	{"left_join", "SELECT u.id FROM users u LEFT JOIN accounts a ON a.user_id = u.id"},
	{"subquery_in_from", "SELECT t.id FROM (SELECT id FROM users WHERE active) t"},
	{"subquery_in_where", "SELECT id FROM accounts WHERE user_id IN (SELECT id FROM users)"},
	{"scalar_subquery_in_select", "SELECT (SELECT count(*) FROM accounts) AS n"},
	{"cte_readonly", "WITH active AS (SELECT id FROM users WHERE active) SELECT * FROM active"},
	{"cte_multiple_readonly", "WITH a AS (SELECT 1 AS x), b AS (SELECT 2 AS y) SELECT a.x, b.y FROM a, b"},
	{"cte_recursive_readonly", "WITH RECURSIVE t(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM t WHERE n < 5) SELECT n FROM t"},
	{"window_function", "SELECT id, row_number() OVER (PARTITION BY tenant ORDER BY created_at) FROM users"},
	{"window_named", "SELECT id, sum(balance) OVER w FROM accounts WINDOW w AS (PARTITION BY tenant)"},
	{"union", "SELECT id FROM users UNION SELECT id FROM archived_users"},
	{"union_all", "SELECT id FROM users UNION ALL SELECT id FROM archived_users"},
	{"intersect", "SELECT id FROM a INTERSECT SELECT id FROM b"},
	{"except", "SELECT id FROM a EXCEPT SELECT id FROM b"},
	{"aggregate_group_by", "SELECT tenant, count(*) FROM users GROUP BY tenant"},
	{"having", "SELECT tenant, count(*) FROM users GROUP BY tenant HAVING count(*) > 1"},
	{"order_by_limit", "SELECT id FROM users ORDER BY created_at DESC LIMIT 100"},
	{"limit_offset", "SELECT id FROM users ORDER BY id LIMIT 10 OFFSET 20"},
	{"distinct", "SELECT DISTINCT tenant FROM users"},
	{"case_expression", "SELECT CASE WHEN active THEN 1 ELSE 0 END FROM users"},
	{"function_call", "SELECT lower(name), coalesce(nickname, name) FROM users"},
	{"values_list", "VALUES (1), (2), (3)"},
	{"trailing_semicolon", "SELECT 1;"},
	{"trailing_semicolon_ws", "SELECT id FROM users ;  "},
	{"cte_then_join", "WITH a AS (SELECT id FROM users) SELECT a.id FROM a JOIN accounts ON accounts.user_id = a.id"},
	{"nested_subquery_param", "SELECT id FROM users WHERE id IN (SELECT user_id FROM accounts WHERE balance > $1)"},
}

// invalidCases are inputs that MUST fail Validate; want is the sentinel error
// expected via errors.Is.
var invalidCases = []struct {
	name string
	sql  string
	want error
}{
	// Empty / comment-only.
	{"empty", "", ErrEmpty},
	{"whitespace_only", "   \t\n  ", ErrEmpty},
	{"line_comment_only", "-- just a comment", ErrEmpty},
	{"block_comment_only", "/* nothing here */", ErrEmpty},
	{"semicolon_only", ";", ErrEmpty},
	{"comment_then_semicolon", "-- x\n;", ErrEmpty},

	// Unparseable.
	{"garbage", "NOT SQL AT ALL @@@", ErrParse},
	{"unterminated_string", "SELECT 'oops", ErrParse},
	{"incomplete_select", "SELECT FROM", ErrParse},

	// Multiple statements.
	{"two_selects", "SELECT 1; SELECT 2", ErrMultipleStatements},
	{"two_selects_no_space", "SELECT 1;SELECT 2", ErrMultipleStatements},
	{"select_then_drop", "SELECT 1; DROP TABLE t", ErrMultipleStatements},
	{"select_then_delete", "SELECT id FROM t; DELETE FROM t", ErrMultipleStatements},
	{"select_semicolon_comment_stmt", "SELECT 1; -- comment\nDROP TABLE t", ErrMultipleStatements},
	{"stacked_injection", "SELECT * FROM users WHERE id = 1; DROP TABLE users; --", ErrMultipleStatements},

	// Top-level non-SELECT DML.
	{"insert", "INSERT INTO t (a) VALUES (1)", ErrNotSelect},
	{"update", "UPDATE t SET a = 1 WHERE id = 2", ErrNotSelect},
	{"delete", "DELETE FROM t WHERE id = 1", ErrNotSelect},
	{"merge", "MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN DELETE", ErrNotSelect},

	// Top-level DDL.
	{"create_table", "CREATE TABLE t (id int)", ErrNotSelect},
	{"alter_table", "ALTER TABLE t ADD COLUMN c int", ErrNotSelect},
	{"drop_table", "DROP TABLE t", ErrNotSelect},
	{"truncate", "TRUNCATE t", ErrNotSelect},
	{"create_index", "CREATE INDEX idx ON t (a)", ErrNotSelect},

	// Top-level side-effecting / control statements.
	{"grant", "GRANT SELECT ON t TO r", ErrNotSelect},
	{"set", "SET search_path TO public", ErrNotSelect},
	{"copy_from", "COPY t FROM '/etc/passwd'", ErrNotSelect},
	{"copy_to", "COPY t TO '/tmp/out.csv'", ErrNotSelect},
	{"call", "CALL do_thing()", ErrNotSelect},
	{"do_block", "DO $$ BEGIN PERFORM 1; END $$", ErrNotSelect},
	{"vacuum", "VACUUM t", ErrNotSelect},
	{"explain_analyze", "EXPLAIN ANALYZE SELECT 1", ErrNotSelect},
	{"begin_tx", "BEGIN", ErrNotSelect},

	// DML hidden inside CTE.
	{"cte_insert", "WITH x AS (INSERT INTO t VALUES (1) RETURNING *) SELECT * FROM x", ErrForbiddenConstruct},
	{"cte_update", "WITH x AS (UPDATE t SET a = 1 RETURNING id) SELECT * FROM x", ErrForbiddenConstruct},
	{"cte_delete", "WITH x AS (DELETE FROM t WHERE id = 1 RETURNING id) SELECT * FROM x", ErrForbiddenConstruct},
	{"cte_nested_delete", "WITH a AS (SELECT 1), b AS (DELETE FROM t RETURNING id) SELECT * FROM a, b", ErrForbiddenConstruct},
	{"cte_dml_in_subquery_cte", "SELECT * FROM (WITH d AS (DELETE FROM t RETURNING *) SELECT * FROM d) s", ErrForbiddenConstruct},

	// Locking (not read-only).
	{"for_update", "SELECT id FROM users FOR UPDATE", ErrForbiddenConstruct},
	{"for_share", "SELECT id FROM users FOR SHARE", ErrForbiddenConstruct},
	{"for_no_key_update", "SELECT id FROM users FOR NO KEY UPDATE", ErrForbiddenConstruct},

	// SELECT INTO creates a table (side effect).
	{"select_into", "SELECT * INTO new_table FROM users", ErrForbiddenConstruct},

	// Injection payloads that survive to a parse but carry a second statement or
	// a modifying construct.
	{"union_then_drop", "SELECT id FROM users UNION SELECT 1; DROP TABLE users", ErrMultipleStatements},
	{"comment_hidden_stmt", "SELECT 1 /* x */; DELETE FROM t", ErrMultipleStatements},
}

func TestValidate_Valid(t *testing.T) {
	for _, tc := range validCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.sql); err != nil {
				t.Fatalf("Validate(%q) = %v; want nil", tc.sql, err)
			}
		})
	}
}

func TestValidate_Invalid(t *testing.T) {
	for _, tc := range invalidCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.sql)
			if err == nil {
				t.Fatalf("Validate(%q) = nil; want %v", tc.sql, tc.want)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Validate(%q) = %v; want errors.Is(_, %v)", tc.sql, err, tc.want)
			}
		})
	}
}

// TestValidate_ErrorsWrapContext proves returned errors carry context while
// still matching the sentinel via errors.Is.
func TestValidate_ErrorsWrapContext(t *testing.T) {
	err := Validate("garbage @@@")
	if err == nil || !errors.Is(err, ErrParse) {
		t.Fatalf("want ErrParse, got %v", err)
	}
	if err.Error() == ErrParse.Error() {
		t.Fatalf("expected wrapped context on parse error, got bare sentinel %q", err.Error())
	}
}

func FuzzValidate(f *testing.F) {
	for _, tc := range validCases {
		f.Add(tc.sql)
	}
	for _, tc := range invalidCases {
		f.Add(tc.sql)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		// Contract: Validate must never panic on any input, and must return
		// either nil or one of the sentinel errors.
		err := Validate(sql)
		if err == nil {
			return
		}
		switch {
		case errors.Is(err, ErrEmpty),
			errors.Is(err, ErrParse),
			errors.Is(err, ErrMultipleStatements),
			errors.Is(err, ErrNotSelect),
			errors.Is(err, ErrForbiddenConstruct):
			// ok
		default:
			t.Fatalf("Validate(%q) returned non-sentinel error: %v", sql, err)
		}
	})
}
