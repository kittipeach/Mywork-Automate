// Package sqlguard enforces the SQL-mode security control for MyWork Automate
// (FR-DB-004/005, spec 07 §4): user-supplied SQL must be exactly one read-only
// SELECT statement with no data-modifying or side-effecting construct anywhere
// in the parse tree.
//
// Enforcement is performed on the PostgreSQL parse tree produced by
// pg_query_go, never on the raw string: comment tricks, stacked statements and
// DML hidden inside CTEs or subqueries cannot bypass a real parser the way they
// bypass string matching.
package sqlguard

import (
	"errors"
	"fmt"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Sentinel errors. Callers should match with errors.Is; Validate wraps these
// with fmt.Errorf %w where it adds context.
var (
	ErrEmpty              = errors.New("sqlguard: empty statement")
	ErrParse              = errors.New("sqlguard: parse error")
	ErrMultipleStatements = errors.New("sqlguard: only a single statement is allowed")
	ErrNotSelect          = errors.New("sqlguard: only SELECT statements are allowed")
	ErrForbiddenConstruct = errors.New("sqlguard: forbidden construct") // DML/DDL/CTE-DML/SET/COPY/CALL/etc.
)

// Validate returns nil iff sql is exactly ONE read-only SELECT statement with no
// data-modifying or side-effecting construct anywhere (including inside CTEs and
// subqueries). Otherwise it returns one of the sentinel errors above (matchable
// via errors.Is).
func Validate(sql string) error {
	if strings.TrimSpace(sql) == "" {
		return ErrEmpty
	}

	result, err := pg.Parse(sql)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrParse, err)
	}

	// pg_query yields zero statements for comment-only or bare-semicolon input
	// that trims to non-empty but carries no actual statement.
	switch {
	case len(result.Stmts) == 0:
		return ErrEmpty
	case len(result.Stmts) > 1:
		return fmt.Errorf("%w: found %d statements", ErrMultipleStatements, len(result.Stmts))
	}

	stmt := result.Stmts[0].GetStmt()
	if stmt.GetSelectStmt() == nil {
		return fmt.Errorf("%w: top-level statement is %s", ErrNotSelect, oneofArmName(stmt.ProtoReflect()))
	}

	// A SELECT node carries its own side-effecting constructs (locking clauses,
	// SELECT INTO) plus, in its children, any DML smuggled into a CTE or
	// subquery. Walk the whole tree and default-deny any statement node that is
	// not itself a SelectStmt.
	return walk(stmt.ProtoReflect())
}

// walk recursively descends every message field (singular or repeated) of m.
//
// For every pg_query.Node it encounters it inspects which oneof arm is set: if
// that arm is a statement message (name ends in "Stmt") other than SelectStmt,
// the tree contains a data-modifying or otherwise side-effecting statement and
// is rejected. Two non-Node constructs that are side-effecting on their own — a
// row-locking clause and a SELECT INTO target — are also rejected.
//
// pg_query's schema contains no map fields, so only singular and repeated
// message fields are traversed; that keeps the whitelist walk exhaustive over
// every node the parser can actually produce.
func walk(m protoreflect.Message) error {
	switch m.Descriptor().FullName() {
	case "pg_query.LockingClause":
		// FOR UPDATE / FOR SHARE / FOR NO KEY UPDATE: not read-only.
		return fmt.Errorf("%w: row-locking clause is not read-only", ErrForbiddenConstruct)
	case "pg_query.IntoClause":
		// SELECT ... INTO creates a relation: side-effecting.
		return fmt.Errorf("%w: SELECT INTO creates a relation", ErrForbiddenConstruct)
	case "pg_query.Node":
		if name := oneofArmName(m); strings.HasSuffix(name, "Stmt") && name != "SelectStmt" {
			return fmt.Errorf("%w: %s is not read-only", ErrForbiddenConstruct, name)
		}
	}

	var walkErr error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() != protoreflect.MessageKind {
			return true
		}
		if fd.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				if err := walk(list.Get(i).Message()); err != nil {
					walkErr = err
					return false
				}
			}
			return true
		}
		if err := walk(v.Message()); err != nil {
			walkErr = err
			return false
		}
		return true
	})
	return walkErr
}

// oneofArmName returns the short (unqualified) message name of the arm currently
// set on a pg_query.Node's "node" oneof — e.g. "SelectStmt", "InsertStmt". It
// returns "" if no arm is set (an empty Node). The "node" oneof always exists on
// a Node and its arms are always messages, so no further guards are needed.
func oneofArmName(m protoreflect.Message) string {
	fd := m.WhichOneof(m.Descriptor().Oneofs().ByName("node"))
	if fd == nil {
		return ""
	}
	return string(fd.Message().Name())
}
