package preview

import (
	"context"
	"fmt"
)

// Column is one column of a table: its name and SQL data type.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Table is one base table in the public schema with its ordered columns.
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// Schema is the connection's public-schema shape: its base tables.
type Schema struct {
	Tables []Table `json:"tables"`
}

// schemaQuery lists every column of every public base table, ordered so that a
// table's columns arrive together in ordinal order and tables arrive
// alphabetically. It reads only information_schema and takes no user input.
const schemaQuery = `SELECT table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name IN (
    SELECT table_name FROM information_schema.tables
    WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
  )
ORDER BY table_name ASC, ordinal_position ASC`

// FetchSchema introspects the public base tables and their columns via q,
// grouping the flat (table, column, type) rows into the nested Schema shape.
// Tables and their columns preserve the query's ordering (table name, then
// ordinal position). It reads only information_schema — no connection data is
// exposed, so no masking is applied.
func FetchSchema(ctx context.Context, q Querier) (Schema, error) {
	rows, err := q.Query(ctx, schemaQuery)
	if err != nil {
		return Schema{}, fmt.Errorf("preview: schema query: %w", err)
	}
	defer rows.Close()

	// Preserve first-seen table order while grouping columns.
	order := make([]string, 0)
	byTable := make(map[string]*Table)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return Schema{}, fmt.Errorf("preview: read schema row: %w", err)
		}
		tableName, _ := vals[0].(string)
		colName, _ := vals[1].(string)
		dataType, _ := vals[2].(string)

		t, ok := byTable[tableName]
		if !ok {
			t = &Table{Name: tableName, Columns: []Column{}}
			byTable[tableName] = t
			order = append(order, tableName)
		}
		t.Columns = append(t.Columns, Column{Name: colName, Type: dataType})
	}
	if err := rows.Err(); err != nil {
		return Schema{}, fmt.Errorf("preview: schema rows: %w", err)
	}

	tables := make([]Table, 0, len(order))
	for _, name := range order {
		tables = append(tables, *byTable[name])
	}
	return Schema{Tables: tables}, nil
}
