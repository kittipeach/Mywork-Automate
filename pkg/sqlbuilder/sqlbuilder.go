// Package sqlbuilder compiles a STRUCTURED, whitelisted query spec into a single
// parameterized read-only SELECT (E6-S2 hardening). It exists so the db.query
// node never accepts free-form SQL from a client: the UI submits a spec (tables,
// joins, columns, filters), the server builds the SQL here, and pkg/sqlguard
// re-validates the result as defence in depth.
//
// Every guard here is about protecting a top-secret MyWork database from
// resource exhaustion and accidental data exfiltration:
//   - identifiers are strictly validated (alnum + underscore, then quoted) so no
//     expression, function call, subquery or comment can be smuggled through a
//     "column name";
//   - every JOIN must carry an ON condition (no accidental cartesian product);
//   - the number of joins is capped (a fan-out bound);
//   - at least one column must be selected (no implicit SELECT *);
//   - a LIMIT is ALWAYS emitted and clamped to a hard row ceiling (no unbounded
//     scan) — combined with the executor's statement timeout this bounds cost;
//   - filter operators come from a whitelist and every value is a bound
//     parameter ($1, $2, …), never interpolated.
package sqlbuilder

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Limits on a single compiled query. Kept as consts (not config) so the bound is
// auditable and cannot be widened per-request.
const (
	// MaxJoins bounds the join fan-out. Five tables is generous for a report and
	// keeps the planner's search space (and the result cardinality) bounded.
	MaxJoins = 5
	// MaxRowCeiling is the absolute LIMIT cap regardless of the requested limit.
	MaxRowCeiling = 100000
	// MaxColumns bounds the projection width.
	MaxColumns = 200
	// MaxFilters bounds the WHERE predicate count.
	MaxFilters = 50
)

// Sentinel errors (matchable via errors.Is).
var (
	ErrInvalidIdent = errors.New("sqlbuilder: invalid identifier")
	ErrNoTable      = errors.New("sqlbuilder: a base table is required")
	ErrNoColumns    = errors.New("sqlbuilder: at least one column must be selected")
	ErrJoinNoOn     = errors.New("sqlbuilder: every join must have an ON condition")
	ErrTooManyJoins = errors.New("sqlbuilder: too many joins")
	ErrBadOperator  = errors.New("sqlbuilder: unsupported filter operator")
	ErrTooBig       = errors.New("sqlbuilder: query exceeds a size limit")
)

// identRe accepts a single SQL identifier: a letter/underscore start, then
// letters, digits or underscores. Deliberately strict — no dots, spaces, quotes,
// parentheses or operators — so a "column" can never carry an expression.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// JoinType is the supported join flavour.
type JoinType string

const (
	InnerJoin JoinType = "inner"
	LeftJoin  JoinType = "left"
)

// OnCond is one equality between two qualified columns (table.column = table.column).
type OnCond struct {
	LeftTable  string `json:"leftTable"`
	LeftCol    string `json:"leftCol"`
	RightTable string `json:"rightTable"`
	RightCol   string `json:"rightCol"`
}

// Join adds one table to the FROM clause with a mandatory ON.
type Join struct {
	Type  JoinType `json:"type"`
	Table string   `json:"table"`
	On    []OnCond `json:"on"`
}

// Column is one projected column, optionally table-qualified and aliased.
type Column struct {
	Table string `json:"table,omitempty"`
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
}

// Op is a whitelisted filter operator.
type Op string

const (
	OpEq   Op = "="
	OpNe   Op = "!="
	OpGt   Op = ">"
	OpGte  Op = ">="
	OpLt   Op = "<"
	OpLte  Op = "<="
	OpLike Op = "like"
)

var allowedOps = map[Op]bool{OpEq: true, OpNe: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true, OpLike: true}

// Filter is one WHERE predicate: <table.column> <op> <bound value>.
type Filter struct {
	Table  string `json:"table,omitempty"`
	Column string `json:"column"`
	Op     Op     `json:"op"`
	Value  any    `json:"value"`
}

// Combinator joins the WHERE predicates.
type Combinator string

const (
	And Combinator = "and"
	Or  Combinator = "or"
)

// Spec is the whitelisted query the UI submits. No raw SQL anywhere.
type Spec struct {
	Table      string     `json:"table"`
	Joins      []Join     `json:"joins,omitempty"`
	Columns    []Column   `json:"columns"`
	Where      []Filter   `json:"where,omitempty"`
	Combinator Combinator `json:"combinator,omitempty"` // default AND
	Limit      int        `json:"limit,omitempty"`
}

// Build compiles spec into a parameterized SELECT and its args, enforcing every
// guard. maxRows further clamps the LIMIT (the effective cap is
// min(spec.Limit>0?spec.Limit:maxRows, maxRows, MaxRowCeiling)).
func Build(spec Spec, maxRows int) (string, []any, error) {
	if strings.TrimSpace(spec.Table) == "" {
		return "", nil, ErrNoTable
	}
	if err := checkIdent(spec.Table); err != nil {
		return "", nil, err
	}
	if len(spec.Joins) > MaxJoins {
		return "", nil, fmt.Errorf("%w: %d > %d", ErrTooManyJoins, len(spec.Joins), MaxJoins)
	}
	if len(spec.Columns) == 0 {
		return "", nil, ErrNoColumns
	}
	if len(spec.Columns) > MaxColumns {
		return "", nil, fmt.Errorf("%w: too many columns", ErrTooBig)
	}
	if len(spec.Where) > MaxFilters {
		return "", nil, fmt.Errorf("%w: too many filters", ErrTooBig)
	}

	// SELECT list.
	cols := make([]string, 0, len(spec.Columns))
	for _, c := range spec.Columns {
		ref, err := qualified(c.Table, c.Name)
		if err != nil {
			return "", nil, err
		}
		if c.Alias != "" {
			if err := checkIdent(c.Alias); err != nil {
				return "", nil, err
			}
			ref += " AS " + quote(c.Alias)
		}
		cols = append(cols, ref)
	}

	// FROM + JOINs.
	from := quote(spec.Table)
	for _, j := range spec.Joins {
		if err := checkIdent(j.Table); err != nil {
			return "", nil, err
		}
		if len(j.On) == 0 {
			return "", nil, fmt.Errorf("%w: join on %q", ErrJoinNoOn, j.Table)
		}
		kw := "INNER JOIN"
		if j.Type == LeftJoin {
			kw = "LEFT JOIN"
		} else if j.Type != InnerJoin && j.Type != "" {
			return "", nil, fmt.Errorf("%w: unknown join type %q", ErrBadOperator, j.Type)
		}
		onParts := make([]string, 0, len(j.On))
		for _, o := range j.On {
			// A join condition must reference specific tables on both sides — no
			// unqualified/ambiguous columns.
			l, err := mustQualified(o.LeftTable, o.LeftCol)
			if err != nil {
				return "", nil, err
			}
			r, err := mustQualified(o.RightTable, o.RightCol)
			if err != nil {
				return "", nil, err
			}
			onParts = append(onParts, l+" = "+r)
		}
		from += fmt.Sprintf(" %s %s ON %s", kw, quote(j.Table), strings.Join(onParts, " AND "))
	}

	// WHERE (parameterized).
	var args []any
	var whereSQL string
	if len(spec.Where) > 0 {
		comb := " AND "
		if spec.Combinator == Or {
			comb = " OR "
		}
		preds := make([]string, 0, len(spec.Where))
		for _, f := range spec.Where {
			op := Op(strings.ToLower(string(f.Op)))
			if !allowedOps[op] {
				return "", nil, fmt.Errorf("%w: %q", ErrBadOperator, f.Op)
			}
			ref, err := qualified(f.Table, f.Column)
			if err != nil {
				return "", nil, err
			}
			args = append(args, f.Value)
			sqlOp := string(op)
			if op == OpLike {
				sqlOp = "LIKE"
			}
			preds = append(preds, fmt.Sprintf("%s %s $%d", ref, sqlOp, len(args)))
		}
		whereSQL = " WHERE " + strings.Join(preds, comb)
	}

	limit := effectiveLimit(spec.Limit, maxRows)

	sql := fmt.Sprintf("SELECT %s FROM %s%s LIMIT %d",
		strings.Join(cols, ", "), from, whereSQL, limit)
	return sql, args, nil
}

// effectiveLimit clamps the requested limit to a hard ceiling, always returning
// a positive value so a LIMIT is always emitted.
func effectiveLimit(requested, maxRows int) int {
	ceiling := MaxRowCeiling
	if maxRows > 0 && maxRows < ceiling {
		ceiling = maxRows
	}
	if requested <= 0 || requested > ceiling {
		return ceiling
	}
	return requested
}

// qualified renders an optional table + required column as a safe reference.
func qualified(table, col string) (string, error) {
	if err := checkIdent(col); err != nil {
		return "", err
	}
	if strings.TrimSpace(table) == "" {
		return quote(col), nil
	}
	if err := checkIdent(table); err != nil {
		return "", err
	}
	return quote(table) + "." + quote(col), nil
}

// mustQualified is qualified with a REQUIRED table (used for join ON columns).
func mustQualified(table, col string) (string, error) {
	if err := checkIdent(table); err != nil {
		return "", err
	}
	return qualified(table, col)
}

// checkIdent enforces the strict identifier grammar.
func checkIdent(name string) error {
	if !identRe.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("%w: %q", ErrInvalidIdent, name)
	}
	return nil
}

// quote wraps an already-validated identifier in double quotes (identRe has
// already guaranteed there is no embedded quote to escape).
func quote(name string) string {
	return `"` + strings.TrimSpace(name) + `"`
}
