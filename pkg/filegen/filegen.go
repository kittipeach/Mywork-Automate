// Package filegen contains the pure, IO-free row writers behind the
// file.generate node (spec 08 E7-S1, spec 05 §3.1). It turns a slice of
// row items (map[string]any) plus an ordered column list into an XLSX, CSV or
// delimited-TXT byte stream. Each writer emits a header row (the column titles)
// followed by one record per item, projecting each item onto the column order.
//
// The writers are deliberately format-only: they take an io.Writer and never
// touch the filesystem, the FileStore or the network. The file.generate
// executor owns rendering the dynamic filename, the onEmpty policy and the
// FileStore.Put; this package owns byte layout so it can be golden-tested in
// isolation to the ≥95% gate.
//
// # Cell formatting
//
// A cell value is stringified once, uniformly, by cellString:
//   - nil renders as the empty string (a missing key is treated as nil);
//   - integers render without a decimal point (42, not 42.000000);
//   - floats render in their shortest exact decimal form (3.14, 200);
//   - bool renders as "true"/"false";
//   - everything else uses its default fmt string form.
//
// This keeps CSV, TXT and XLSX cell text identical for the same input, so a
// value that reads "9000000000" in a CSV reads the same in the spreadsheet.
package filegen

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// defaultSheet is used by WriteXLSX when the caller passes an empty sheet name.
const defaultSheet = "Sheet1"

// Columns returns the stable, sorted union of all keys across items. It is used
// by the executor to derive an "auto layout" column order when the node config
// does not pin an explicit column list (spec 05 §3.1 layoutMode:auto). The
// result is deterministic (sorted) so generated files are reproducible.
func Columns(items []map[string]any) []string {
	seen := make(map[string]struct{})
	for _, it := range items {
		for k := range it {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// WriteCSV writes a header row of cols followed by one CSV record per item,
// using delimiter as the field separator (encoding/csv). Cells are formatted by
// cellString; a missing/nil key yields an empty field. Empty items still emit
// the header row.
func WriteCSV(w io.Writer, cols []string, items []map[string]any, delimiter rune) error {
	cw := csv.NewWriter(w)
	cw.Comma = delimiter

	// csv.Writer buffers internally and defers any underlying write error until
	// Flush; the per-record Write error is therefore always nil here (checking it
	// would be dead code). The single authoritative error check is cw.Error()
	// after Flush, which surfaces the first underlying write failure.
	_ = cw.Write(cols)
	for _, it := range items {
		_ = cw.Write(rowStrings(cols, it))
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("filegen: write csv: %w", err)
	}
	return nil
}

// WriteTXT writes a header row of cols followed by one delimited line per item,
// joining fields with delimiter (a string, allowing multi-byte separators).
// Cells are formatted by cellString. Empty items still emit the header line.
func WriteTXT(w io.Writer, cols []string, items []map[string]any, delimiter string) error {
	if err := writeLine(w, cols, delimiter); err != nil {
		return fmt.Errorf("filegen: write txt header: %w", err)
	}
	for i, it := range items {
		if err := writeLine(w, rowStrings(cols, it), delimiter); err != nil {
			return fmt.Errorf("filegen: write txt row %d: %w", i, err)
		}
	}
	return nil
}

// writeLine joins fields with sep and writes the line plus a trailing newline.
func writeLine(w io.Writer, fields []string, sep string) error {
	_, err := io.WriteString(w, strings.Join(fields, sep)+"\n")
	return err
}

// setStreamRow writes one row of cells at 1-based rowNum, column A onward. It is
// a package var (defaulting to the excelize StreamWriter path) so a test can
// inject a write failure — a genuine StreamWriter.SetRow error (e.g. rows
// written out of order) is otherwise not reachable through WriteXLSX, which
// always writes strictly increasing rows.
var setStreamRow = func(sw *excelize.StreamWriter, rowNum int, fields []string) error {
	// A1-anchored axis for column 1, rowNum>=1 is always a valid cell reference,
	// so it is composed directly (no error-returning coordinate call needed).
	axis := "A" + strconv.Itoa(rowNum)
	cells := make([]any, len(fields))
	for i, v := range fields {
		cells[i] = v
	}
	if err := sw.SetRow(axis, cells); err != nil {
		return fmt.Errorf("filegen: set row %d: %w", rowNum, err)
	}
	return nil
}

// WriteXLSX streams cols as a header row and one row per item into a single
// worksheet named sheet (defaulting to "Sheet1" when empty), then writes the
// workbook to w. It uses excelize's StreamWriter so a large item set does not
// materialise the whole workbook in memory (spec: XLSX = excelize StreamWriter).
func WriteXLSX(w io.Writer, cols []string, items []map[string]any, sheet string) error {
	if strings.TrimSpace(sheet) == "" {
		sheet = defaultSheet
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// A fresh workbook already has "Sheet1"; rename it to the target so there is
	// exactly one sheet with the requested name. SetSheetName rejects names with
	// illegal characters or over 31 chars, surfaced here.
	if err := f.SetSheetName(defaultSheet, sheet); err != nil {
		return fmt.Errorf("filegen: name sheet %q: %w", sheet, err)
	}

	// NewStreamWriter only fails for an unknown sheet, which cannot happen
	// immediately after a successful SetSheetName; the error is still propagated.
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return fmt.Errorf("filegen: stream writer for %q: %w", sheet, err)
	}

	if err := setStreamRow(sw, 1, cols); err != nil {
		return err
	}
	for i, it := range items {
		if err := setStreamRow(sw, i+2, rowStrings(cols, it)); err != nil {
			return err
		}
	}
	if err := sw.Flush(); err != nil {
		return fmt.Errorf("filegen: flush stream: %w", err)
	}
	if err := f.Write(w); err != nil {
		return fmt.Errorf("filegen: write xlsx: %w", err)
	}
	return nil
}

// rowStrings projects an item onto cols in order, formatting each cell.
func rowStrings(cols []string, it map[string]any) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = cellString(it[c])
	}
	return out
}

// cellString renders one cell value to its canonical string form (see package
// doc). A nil (including a missing map key) becomes "".
func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.FormatInt(int64(t), 10)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case uint8:
		return strconv.FormatUint(uint64(t), 10)
	case uint16:
		return strconv.FormatUint(uint64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}
