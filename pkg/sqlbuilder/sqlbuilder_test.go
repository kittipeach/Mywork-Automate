package sqlbuilder

import (
	"errors"
	"strings"
	"testing"
)

func TestBuild_SingleTable(t *testing.T) {
	sql, args, err := Build(Spec{
		Table:   "employees",
		Columns: []Column{{Name: "name"}, {Name: "salary", Alias: "pay"}},
		Limit:   50,
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "name", "salary" AS "pay" FROM "employees" LIMIT 50`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want none", args)
	}
}

func TestBuild_JoinsAndWhere_Parameterized(t *testing.T) {
	sql, args, err := Build(Spec{
		Table: "employees",
		Joins: []Join{
			{Type: LeftJoin, Table: "departments", On: []OnCond{{LeftTable: "employees", LeftCol: "dept_id", RightTable: "departments", RightCol: "id"}}},
		},
		Columns: []Column{{Table: "employees", Name: "name"}, {Table: "departments", Name: "name", Alias: "dept"}},
		Where: []Filter{
			{Table: "employees", Column: "salary", Op: OpGte, Value: 50000},
			{Table: "departments", Column: "name", Op: OpLike, Value: "Fin%"},
		},
		Combinator: And,
		Limit:      100,
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "employees"."name", "departments"."name" AS "dept" FROM "employees" ` +
		`LEFT JOIN "departments" ON "employees"."dept_id" = "departments"."id" ` +
		`WHERE "employees"."salary" >= $1 AND "departments"."name" LIKE $2 LIMIT 100`
	if sql != want {
		t.Fatalf("sql =\n%q\nwant\n%q", sql, want)
	}
	if len(args) != 2 || args[0] != 50000 || args[1] != "Fin%" {
		t.Fatalf("args = %v", args)
	}
}

func TestBuild_InnerJoinDefault_And_OrCombinator(t *testing.T) {
	sql, _, err := Build(Spec{
		Table:      "a",
		Joins:      []Join{{Table: "b", On: []OnCond{{LeftTable: "a", LeftCol: "id", RightTable: "b", RightCol: "a_id"}}}},
		Columns:    []Column{{Table: "a", Name: "x"}},
		Where:      []Filter{{Column: "x", Op: OpEq, Value: 1}, {Column: "y", Op: OpNe, Value: 2}},
		Combinator: Or,
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `INNER JOIN "b" ON "a"."id" = "b"."a_id"`) {
		t.Fatalf("missing inner join: %s", sql)
	}
	if !strings.Contains(sql, "$1 OR ") {
		t.Fatalf("expected OR combinator: %s", sql)
	}
	if !strings.HasSuffix(sql, "LIMIT 10") {
		t.Fatalf("expected clamp to maxRows 10: %s", sql)
	}
}

func TestBuild_LimitClamping(t *testing.T) {
	// no limit → clamp to maxRows
	sql, _, _ := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}}, 200)
	if !strings.HasSuffix(sql, "LIMIT 200") {
		t.Fatalf("no-limit should clamp to maxRows: %s", sql)
	}
	// requested above maxRows → clamp to maxRows
	sql, _, _ = Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Limit: 999999}, 500)
	if !strings.HasSuffix(sql, "LIMIT 500") {
		t.Fatalf("over-limit should clamp to maxRows: %s", sql)
	}
	// requested below maxRows → honoured
	sql, _, _ = Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Limit: 25}, 500)
	if !strings.HasSuffix(sql, "LIMIT 25") {
		t.Fatalf("under-limit should be honoured: %s", sql)
	}
	// maxRows unset / huge → hard ceiling applies
	sql, _, _ = Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}}, 0)
	if !strings.HasSuffix(sql, "LIMIT 100000") {
		t.Fatalf("unset maxRows should use the hard ceiling: %s", sql)
	}
}

func TestBuild_Guards(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		want error
	}{
		{"no table", Spec{Columns: []Column{{Name: "c"}}}, ErrNoTable},
		{"no columns", Spec{Table: "t"}, ErrNoColumns},
		{"join without ON", Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: []Join{{Table: "b"}}}, ErrJoinNoOn},
		{"too many joins", Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: manyJoins(MaxJoins + 1)}, ErrTooManyJoins},
		{"bad operator", Spec{Table: "t", Columns: []Column{{Name: "c"}}, Where: []Filter{{Column: "c", Op: "; DROP", Value: 1}}}, ErrBadOperator},
		{"unknown join type", Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: []Join{{Type: "cross", Table: "b", On: []OnCond{{LeftTable: "t", LeftCol: "id", RightTable: "b", RightCol: "id"}}}}}, ErrBadOperator},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Build(tc.spec, 100); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestBuild_RejectsInjectionInIdentifiers proves an attacker cannot smuggle SQL
// through any identifier slot (table, column, alias, join table, ON columns,
// filter column).
func TestBuild_RejectsInjectionInIdentifiers(t *testing.T) {
	evil := []string{
		`name"; DROP TABLE employees; --`,
		"name); SELECT",
		"a.b",         // dotted — must be split into table/column, not one ident
		"count(*)",    // function call
		"1=1",         // predicate
		"name OR 1=1", // space
		"",            // empty
		"  ",          // blank
	}
	for _, bad := range evil {
		// as a base table
		if _, _, err := Build(Spec{Table: bad, Columns: []Column{{Name: "c"}}}, 100); err == nil {
			t.Errorf("table %q was accepted", bad)
		}
		// as a column name
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: bad}}}, 100); err == nil {
			t.Errorf("column %q was accepted", bad)
		}
		// as a column's table qualifier (non-empty bad values only; "" means
		// "unqualified column", which is legitimate)
		if strings.TrimSpace(bad) != "" {
			if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Table: bad, Name: "c"}}}, 100); err == nil {
				t.Errorf("column table %q was accepted", bad)
			}
			// and as a filter's table qualifier
			if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Where: []Filter{{Table: bad, Column: "c", Op: OpEq, Value: 1}}}, 100); err == nil {
				t.Errorf("filter table %q was accepted", bad)
			}
		}
		// as an alias
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c", Alias: bad}}}, 100); err == nil && bad != "" && strings.TrimSpace(bad) != "" {
			t.Errorf("alias %q was accepted", bad)
		}
		// as a filter column
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Where: []Filter{{Column: bad, Op: OpEq, Value: 1}}}, 100); err == nil {
			t.Errorf("filter column %q was accepted", bad)
		}
		// as a join table / ON column
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: []Join{{Table: bad, On: []OnCond{{LeftTable: "t", LeftCol: "id", RightTable: "x", RightCol: "id"}}}}}, 100); err == nil {
			t.Errorf("join table %q was accepted", bad)
		}
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: []Join{{Table: "x", On: []OnCond{{LeftTable: bad, LeftCol: "id", RightTable: "x", RightCol: "id"}}}}}, 100); err == nil {
			t.Errorf("ON left table %q was accepted", bad)
		}
		if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Joins: []Join{{Table: "x", On: []OnCond{{LeftTable: "t", LeftCol: "id", RightTable: "x", RightCol: bad}}}}}, 100); err == nil {
			t.Errorf("ON right col %q was accepted", bad)
		}
	}
}

func TestBuild_SizeCeilings(t *testing.T) {
	// too many columns
	cols := make([]Column, MaxColumns+1)
	for i := range cols {
		cols[i] = Column{Name: "c"}
	}
	if _, _, err := Build(Spec{Table: "t", Columns: cols}, 100); !errors.Is(err, ErrTooBig) {
		t.Fatalf("too-many-columns err = %v", err)
	}
	// too many filters
	fs := make([]Filter, MaxFilters+1)
	for i := range fs {
		fs[i] = Filter{Column: "c", Op: OpEq, Value: i}
	}
	if _, _, err := Build(Spec{Table: "t", Columns: []Column{{Name: "c"}}, Where: fs}, 100); !errors.Is(err, ErrTooBig) {
		t.Fatalf("too-many-filters err = %v", err)
	}
}

func manyJoins(n int) []Join {
	out := make([]Join, n)
	for i := range out {
		out[i] = Join{Table: "b", On: []OnCond{{LeftTable: "t", LeftCol: "id", RightTable: "b", RightCol: "id"}}}
	}
	return out
}
