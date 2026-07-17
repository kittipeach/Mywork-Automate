package filegen

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestColumns_StableSortedUnion(t *testing.T) {
	items := []map[string]any{
		{"b": 1, "a": 2},
		{"c": 3, "a": 4},
		{"a": 5},
	}
	got := Columns(items)
	require.Equal(t, []string{"a", "b", "c"}, got)
}

func TestColumns_Empty(t *testing.T) {
	require.Empty(t, Columns(nil))
	require.Empty(t, Columns([]map[string]any{}))
	// Items with no keys yield no columns.
	require.Empty(t, Columns([]map[string]any{{}, {}}))
}

func TestWriteCSV_HeaderRowsAndTypes(t *testing.T) {
	items := []map[string]any{
		{"id": 1, "name": "alice", "note": nil},
		{"id": 2, "name": "bob", "note": "x"},
	}
	var buf bytes.Buffer
	err := WriteCSV(&buf, []string{"id", "name", "note"}, items, ',')
	require.NoError(t, err)

	r := csv.NewReader(strings.NewReader(buf.String()))
	recs, err := r.ReadAll()
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"id", "name", "note"},
		{"1", "alice", ""},
		{"2", "bob", "x"},
	}, recs)
}

func TestWriteCSV_CustomDelimiter(t *testing.T) {
	items := []map[string]any{{"a": "x", "b": "y"}}
	var buf bytes.Buffer
	err := WriteCSV(&buf, []string{"a", "b"}, items, ';')
	require.NoError(t, err)
	require.Equal(t, "a;b\nx;y\n", strings.ReplaceAll(buf.String(), "\r\n", "\n"))
}

func TestWriteCSV_EmptyItems_HeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSV(&buf, []string{"a", "b"}, nil, ',')
	require.NoError(t, err)
	require.Equal(t, "a,b\n", strings.ReplaceAll(buf.String(), "\r\n", "\n"))
}

func TestWriteCSV_NumberFormatting(t *testing.T) {
	items := []map[string]any{
		{"i": 42, "f": 3.14, "big": int64(9000000000), "b": true},
	}
	var buf bytes.Buffer
	err := WriteCSV(&buf, []string{"i", "f", "big", "b"}, items, ',')
	require.NoError(t, err)
	got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	require.Equal(t, "i,f,big,b\n42,3.14,9000000000,true\n", got)
}

func TestWriteTXT_Delimited(t *testing.T) {
	items := []map[string]any{
		{"a": "1", "b": "2"},
		{"a": "3", "b": nil},
	}
	var buf bytes.Buffer
	err := WriteTXT(&buf, []string{"a", "b"}, items, "|")
	require.NoError(t, err)
	require.Equal(t, "a|b\n1|2\n3|\n", buf.String())
}

func TestWriteTXT_EmptyItems_HeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTXT(&buf, []string{"x", "y"}, nil, "\t")
	require.NoError(t, err)
	require.Equal(t, "x\ty\n", buf.String())
}

func TestWriteTXT_NumbersAndBool(t *testing.T) {
	items := []map[string]any{{"n": 10, "f": 2.5, "ok": false}}
	var buf bytes.Buffer
	err := WriteTXT(&buf, []string{"n", "f", "ok"}, items, ",")
	require.NoError(t, err)
	require.Equal(t, "n,f,ok\n10,2.5,false\n", buf.String())
}

func TestWriteXLSX_RoundTrip(t *testing.T) {
	items := []map[string]any{
		{"id": 1, "name": "alice", "amount": 100.5},
		{"id": 2, "name": "bob", "amount": 200},
	}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"id", "name", "amount"}, items, "Sheet1")
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close()

	// Header row.
	require.Equal(t, "id", cell(t, f, "Sheet1", "A1"))
	require.Equal(t, "name", cell(t, f, "Sheet1", "B1"))
	require.Equal(t, "amount", cell(t, f, "Sheet1", "C1"))
	// Data rows.
	require.Equal(t, "1", cell(t, f, "Sheet1", "A2"))
	require.Equal(t, "alice", cell(t, f, "Sheet1", "B2"))
	require.Equal(t, "100.5", cell(t, f, "Sheet1", "C2"))
	require.Equal(t, "2", cell(t, f, "Sheet1", "A3"))
	require.Equal(t, "bob", cell(t, f, "Sheet1", "B3"))
	require.Equal(t, "200", cell(t, f, "Sheet1", "C3"))
}

func TestWriteXLSX_CustomSheetName(t *testing.T) {
	items := []map[string]any{{"x": "v"}}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"x"}, items, "Payroll")
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close()
	require.Contains(t, f.GetSheetList(), "Payroll")
	require.Equal(t, "x", cell(t, f, "Payroll", "A1"))
	require.Equal(t, "v", cell(t, f, "Payroll", "A2"))
}

func TestWriteXLSX_DefaultSheetWhenEmptyName(t *testing.T) {
	items := []map[string]any{{"a": 1}}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a"}, items, "")
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close()
	require.Contains(t, f.GetSheetList(), "Sheet1")
}

func TestWriteXLSX_EmptyItems_HeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a", "b"}, nil, "Sheet1")
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, "a", cell(t, f, "Sheet1", "A1"))
	require.Equal(t, "b", cell(t, f, "Sheet1", "B1"))
	require.Equal(t, "", cell(t, f, "Sheet1", "A2"))
}

func TestWriteXLSX_NilAndMissingCells(t *testing.T) {
	items := []map[string]any{
		{"a": nil},      // present but nil
		{"b": "only-b"}, // "a" missing entirely
	}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a", "b"}, items, "Sheet1")
	require.NoError(t, err)

	f, err := excelize.OpenReader(&buf)
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, "", cell(t, f, "Sheet1", "A2"))
	require.Equal(t, "", cell(t, f, "Sheet1", "B2"))
	require.Equal(t, "", cell(t, f, "Sheet1", "A3"))
	require.Equal(t, "only-b", cell(t, f, "Sheet1", "B3"))
}

// cell reads a single cell value, failing the test on error.
func cell(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, ref)
	require.NoError(t, err)
	return v
}

// failWriter fails after n successful bytes to exercise writer-error paths.
type failWriter struct {
	remaining int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errWrite
	}
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, errWrite
	}
	w.remaining -= len(p)
	return len(p), nil
}

var errWrite = errWriteType{}

type errWriteType struct{}

func (errWriteType) Error() string { return "boom" }

func TestWriteCSV_WriterError_Header(t *testing.T) {
	items := []map[string]any{{"a": "x"}}
	err := WriteCSV(&failWriter{remaining: 0}, []string{"a"}, items, ',')
	require.Error(t, err)
}

func TestWriteCSV_WriterError_Row(t *testing.T) {
	// csv.Writer buffers; the error surfaces on Flush -> cw.Error(). "a\n" is 2
	// bytes for the header, then the row + flush fails.
	items := []map[string]any{{"a": "x"}}
	err := WriteCSV(&failWriter{remaining: 2}, []string{"a"}, items, ',')
	require.Error(t, err)
}

func TestWriteTXT_WriterError_Header(t *testing.T) {
	items := []map[string]any{{"a": "x"}}
	err := WriteTXT(&failWriter{remaining: 0}, []string{"a"}, items, ",")
	require.Error(t, err)
}

func TestWriteTXT_WriterError_Row(t *testing.T) {
	// "a\n" header is 2 bytes; the data row write then fails.
	items := []map[string]any{{"a": "x"}}
	err := WriteTXT(&failWriter{remaining: 2}, []string{"a"}, items, ",")
	require.Error(t, err)
}

func TestWriteXLSX_WriterError(t *testing.T) {
	items := []map[string]any{{"a": "x"}}
	err := WriteXLSX(&failWriter{remaining: 0}, []string{"a"}, items, "Sheet1")
	require.Error(t, err)
}

func TestWriteXLSX_InvalidSheetName(t *testing.T) {
	// excelize rejects sheet names containing :\/?*[] — this exercises the
	// SetSheetName error branch.
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a"}, []map[string]any{{"a": 1}}, "bad:name")
	require.Error(t, err)
	require.Contains(t, err.Error(), "name sheet")
}

func TestWriteXLSX_HeaderRowError(t *testing.T) {
	restore := setStreamRow
	t.Cleanup(func() { setStreamRow = restore })
	setStreamRow = func(_ *excelize.StreamWriter, _ int, _ []string) error {
		return errWrite
	}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a"}, []map[string]any{{"a": 1}}, "Sheet1")
	require.ErrorIs(t, err, errWrite)
}

func TestWriteXLSX_DataRowError(t *testing.T) {
	restore := setStreamRow
	t.Cleanup(func() { setStreamRow = restore })
	// Fail only on the data row (rowNum >= 2); the header row (1) succeeds.
	setStreamRow = func(sw *excelize.StreamWriter, rowNum int, fields []string) error {
		if rowNum >= 2 {
			return errWrite
		}
		return restore(sw, rowNum, fields)
	}
	var buf bytes.Buffer
	err := WriteXLSX(&buf, []string{"a"}, []map[string]any{{"a": 1}}, "Sheet1")
	require.ErrorIs(t, err, errWrite)
}

func TestWriteXLSX_SetStreamRow_RealSetRowError(t *testing.T) {
	// Directly exercise the real setStreamRow SetRow error branch: writing row 1
	// after row 2 is rejected by excelize ("row 1 has already been written").
	f := excelize.NewFile()
	defer f.Close()
	sw, err := f.NewStreamWriter("Sheet1")
	require.NoError(t, err)
	require.NoError(t, setStreamRow(sw, 2, []string{"x"}))
	err = setStreamRow(sw, 1, []string{"y"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "set row 1")
}

func TestCellString_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hi", "hi"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int", 42, "42"},
		{"int negative", -7, "-7"},
		{"int8", int8(8), "8"},
		{"int16", int16(16), "16"},
		{"int32", int32(32), "32"},
		{"int64", int64(9000000000), "9000000000"},
		{"uint", uint(1), "1"},
		{"uint8", uint8(2), "2"},
		{"uint16", uint16(3), "3"},
		{"uint32", uint32(4), "4"},
		{"uint64", uint64(5), "5"},
		{"float32", float32(3.5), "3.5"},
		{"float64 exact int", float64(200), "200"},
		{"float64 frac", 3.14, "3.14"},
		{"fallback slice", []int{1, 2}, "[1 2]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, cellString(tt.in))
		})
	}
}
