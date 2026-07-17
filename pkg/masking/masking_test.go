package masking

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewEngine
// ---------------------------------------------------------------------------

func TestNewEngine_InvalidRegex(t *testing.T) {
	_, err := NewEngine([]Rule{
		{
			Name:      "broken",
			MatchType: MatchColumnName,
			Pattern:   "(unclosed",
			Style:     StyleFull,
			AppliesTo: []Point{PointPreview},
			Active:    true,
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("expected error to mention rule name %q, got %v", "broken", err)
	}
}

func TestNewEngine_Valid(t *testing.T) {
	e, err := NewEngine([]Rule{
		{
			Name:      "salary",
			MatchType: MatchColumnName,
			Pattern:   "(?i)salary",
			Style:     StyleFull,
			AppliesTo: []Point{PointPreview},
			Active:    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestNewEngine_InactiveRuleWithBadRegexStillValidates(t *testing.T) {
	// Inactive rules must still be compiled/validated — a bad regex is a
	// configuration error regardless of the Active flag.
	_, err := NewEngine([]Rule{
		{
			Name:      "inactive-broken",
			MatchType: MatchColumnName,
			Pattern:   "([a-z",
			Style:     StyleFull,
			AppliesTo: []Point{PointPreview},
			Active:    false,
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid regex even when rule inactive")
	}
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

func mustEngine(t *testing.T, rules []Rule) *Engine {
	t.Helper()
	e, err := NewEngine(rules)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return e
}

func maskOne(t *testing.T, e *Engine, key string, val any, point Point, roles []string) any {
	t.Helper()
	out := e.MaskItems([]map[string]any{{key: val}}, point, roles)
	return out[0][key]
}

func TestMaskItems_Styles(t *testing.T) {
	rules := []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^full$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
		{Name: "last4", MatchType: MatchColumnName, Pattern: "(?i)^last4$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
		{Name: "hash", MatchType: MatchColumnName, Pattern: "(?i)^hashcol$", Style: StyleHash, AppliesTo: []Point{PointPreview}, Active: true},
	}
	e := mustEngine(t, rules)

	sum := sha256.Sum256([]byte("secretvalue"))
	wantHash := hex.EncodeToString(sum[:])

	tests := []struct {
		name string
		key  string
		val  any
		want any
	}{
		{"full string", "full", "1234567890", maskToken},
		{"full number", "full", 987654, maskToken},
		{"partial keeps last4", "last4", "1234567890", "******7890"},
		{"partial number keeps last4", "last4", 1234567890, "******7890"},
		{"partial exactly 4 fully masked (safe degrade)", "last4", "6789", "****"},
		{"partial shorter than 4 fully masked", "last4", "12", "**"},
		{"partial empty stays empty", "last4", "", ""},
		{"hash string", "hashcol", "secretvalue", wantHash},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskOne(t, e, tc.key, tc.val, PointPreview, nil)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestMaskItems_PartialLast4NeverRevealsMoreThanLast4(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "l4", MatchType: MatchColumnName, Pattern: "(?i)^acct$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "acct", "123", PointPreview, nil).(string)
	// value shorter than 4 must reveal nothing (all stars), never the digits
	if strings.ContainsAny(got, "123") {
		t.Errorf("short value leaked cleartext: %q", got)
	}
	if got != "***" {
		t.Errorf("got %q, want %q", got, "***")
	}
}

func TestMaskItems_HashDeterministic(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "h", MatchType: MatchColumnName, Pattern: "(?i)^h$", Style: StyleHash, AppliesTo: []Point{PointPreview}, Active: true},
	})
	a := maskOne(t, e, "h", "same", PointPreview, nil)
	b := maskOne(t, e, "h", "same", PointPreview, nil)
	if a != b {
		t.Errorf("hash not deterministic: %v != %v", a, b)
	}
	c := maskOne(t, e, "h", "different", PointPreview, nil)
	if a == c {
		t.Errorf("distinct inputs produced identical hash")
	}
}

func TestMaskItems_HashNumberEqualsHashOfDecimalString(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "h", MatchType: MatchColumnName, Pattern: "(?i)^h$", Style: StyleHash, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "h", 42, PointPreview, nil)
	sum := sha256.Sum256([]byte("42"))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("hash(number 42)=%v, want %v", got, want)
	}
}

// unknown style => value left untouched (defensive default branch)
func TestMaskItems_UnknownStyleLeavesValue(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "weird", MatchType: MatchColumnName, Pattern: "(?i)^weird$", Style: Style("nonexistent"), AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "weird", "keepme", PointPreview, nil)
	if got != "keepme" {
		t.Errorf("unknown style should leave value untouched, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Value types: string, numeric, null
// ---------------------------------------------------------------------------

func TestMaskItems_NumericTypes(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^n$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	numeric := []any{
		int(5), int8(5), int16(5), int32(5), int64(5),
		uint(5), uint8(5), uint16(5), uint32(5), uint64(5),
		float32(5.5), float64(5.5),
	}
	for _, v := range numeric {
		got := maskOne(t, e, "n", v, PointPreview, nil)
		if got != maskToken {
			t.Errorf("numeric %T not masked: got %v", v, got)
		}
	}
}

func TestMaskItems_PartialNumericFormatting(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "l4", MatchType: MatchColumnName, Pattern: "(?i)^n$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
	})
	tests := []struct {
		val  any
		want string
	}{
		{int64(1234567), "***4567"}, // "1234567" -> keep "4567", star 3
		{float64(50000), "*0000"},   // "50000" -> keep "0000", star "5"
		{uint(9876), "****"},        // exactly 4 chars -> fully masked (safe degrade)
	}
	for _, tc := range tests {
		got := maskOne(t, e, "n", tc.val, PointPreview, nil)
		if got != tc.want {
			t.Errorf("val %v(%T): got %v, want %v", tc.val, tc.val, got, tc.want)
		}
	}
}

func TestMaskItems_FloatWithFractionUsesStrconv(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "l4", MatchType: MatchColumnName, Pattern: "(?i)^n$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
	})
	// 1234.5 -> "1234.5" (6 chars) -> keep last 4 "34.5"
	got := maskOne(t, e, "n", float64(1234.5), PointPreview, nil)
	if got != "**34.5" {
		t.Errorf("got %v, want %v", got, "**34.5")
	}
}

// pgNumeric mimics a driver database value (e.g. pgx's pgtype.Numeric): it is
// not a native Go numeric/string, but it stringifies to a number via
// driver.Valuer. Sensitive numeric DB columns (salary) arrive as this shape at
// runtime, so the engine must mask them rather than pass them through.
type pgNumeric struct{ s string }

func (n pgNumeric) Value() (driver.Value, error) { return n.s, nil }

// stringerVal implements fmt.Stringer only.
type stringerVal struct{ s string }

func (v stringerVal) String() string { return v.s }

// errValuer / nilValuer are driver.Valuers whose Value() cannot yield a usable
// string (error, or a nil driver value). The engine treats them as
// non-stringifiable and, per the long-standing contract, leaves them untouched.
type errValuer struct{}

func (errValuer) Value() (driver.Value, error) { return nil, fmt.Errorf("boom") }

type nilValuer struct{}

func (nilValuer) Value() (driver.Value, error) { return nil, nil }

func TestMaskItems_ValuerUnusableLeftAsIs(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^salary$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	for _, v := range []any{errValuer{}, nilValuer{}} {
		got := maskOne(t, e, "salary", v, PointPreview, nil)
		if _, isString := got.(string); isString {
			t.Errorf("unusable valuer %T should be left as-is, got string %v", v, got)
		}
	}
}

// TestMaskItems_ExoticNumericFullMask proves the runtime salary leak is fixed:
// a Postgres numeric column arrives as a driver.Valuer (pgtype.Numeric), not a
// native Go numeric, and must be masked by a StyleFull rule. fmt.Stringer and
// json.Number — the other shapes DB/JSON values take — are covered too.
func TestMaskItems_ExoticNumericFullMask(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^salary$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	for _, v := range []any{
		pgNumeric{"85000"},   // driver.Valuer (pgtype.Numeric-like) — the real salary leak
		stringerVal{"85000"}, // fmt.Stringer
		json.Number("85000"), // encoding/json numeric
	} {
		got := maskOne(t, e, "salary", v, PointPreview, nil)
		if got != maskToken {
			t.Errorf("StyleFull leaked %T: got %v, want %q", v, got, maskToken)
		}
	}
}

// TestMaskItems_ExoticNumericPartialHash proves the partial/hash styles now
// format DB-sourced exotic numerics correctly (stringified via driver.Valuer /
// fmt.Stringer) instead of leaking them.
func TestMaskItems_ExoticNumericPartialHash(t *testing.T) {
	part := mustEngine(t, []Rule{
		{Name: "l4", MatchType: MatchColumnName, Pattern: "(?i)^acct$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
	})
	if got := maskOne(t, part, "acct", pgNumeric{"1234567"}, PointPreview, nil); got != "***4567" {
		t.Errorf("partial pgNumeric: got %v, want ***4567", got)
	}
	if got := maskOne(t, part, "acct", stringerVal{"1234567"}, PointPreview, nil); got != "***4567" {
		t.Errorf("partial stringer: got %v, want ***4567", got)
	}

	hash := mustEngine(t, []Rule{
		{Name: "h", MatchType: MatchColumnName, Pattern: "(?i)^acct$", Style: StyleHash, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, hash, "acct", pgNumeric{"85000"}, PointPreview, nil)
	if s, ok := got.(string); !ok || len(s) != 64 {
		t.Errorf("hash pgNumeric: got %v (%T), want 64-hex sha256", got, got)
	}
}

func TestMaskItems_NilLeftAsIs(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^x$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "x", nil, PointPreview, nil)
	if got != nil {
		t.Errorf("nil should stay nil, got %#v", got)
	}
}

func TestMaskItems_UnsupportedValueTypeLeftAsIs(t *testing.T) {
	// bool is not a string/number => nothing to mask, left as-is
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^b$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "b", true, PointPreview, nil)
	if got != true {
		t.Errorf("bool should be left as-is, got %#v", got)
	}
}

func TestMaskItems_NonMatchingKeyUnchanged(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^secret$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	got := maskOne(t, e, "public", "hello", PointPreview, nil)
	if got != "hello" {
		t.Errorf("non-matching key must be unchanged, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// applies_to / points
// ---------------------------------------------------------------------------

func TestMaskItems_AppliesToPointGating(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "logonly", MatchType: MatchColumnName, Pattern: "(?i)^s$", Style: StyleFull, AppliesTo: []Point{PointLog}, Active: true},
	})
	// at log => masked
	if got := maskOne(t, e, "s", "v", PointLog, nil); got != maskToken {
		t.Errorf("log point should mask, got %v", got)
	}
	// at preview => NOT masked (preview not in AppliesTo)
	if got := maskOne(t, e, "s", "v", PointPreview, nil); got != "v" {
		t.Errorf("preview point must not mask a log-only rule, got %v", got)
	}
	// at ai => NOT masked
	if got := maskOne(t, e, "s", "v", PointAI, nil); got != "v" {
		t.Errorf("ai point must not mask a log-only rule, got %v", got)
	}
}

func TestMaskItems_AllThreePoints(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "all", MatchType: MatchColumnName, Pattern: "(?i)^s$", Style: StyleFull, AppliesTo: []Point{PointPreview, PointLog, PointAI}, Active: true},
	})
	for _, p := range []Point{PointPreview, PointLog, PointAI} {
		if got := maskOne(t, e, "s", "v", p, nil); got != maskToken {
			t.Errorf("point %s should mask, got %v", p, got)
		}
	}
}

func TestMaskItems_InactiveRuleNeverMasks(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "off", MatchType: MatchColumnName, Pattern: "(?i)^s$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: false},
	})
	if got := maskOne(t, e, "s", "v", PointPreview, nil); got != "v" {
		t.Errorf("inactive rule must not mask, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Exempt roles
// ---------------------------------------------------------------------------

func TestMaskItems_ExemptRoles(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "salary", MatchType: MatchColumnName, Pattern: "(?i)^salary$", Style: StyleFull, ExemptRoles: []string{"admin", "hr_lead"}, AppliesTo: []Point{PointPreview}, Active: true},
	})
	tests := []struct {
		name   string
		roles  []string
		masked bool
	}{
		{"no roles => masked", nil, true},
		{"unrelated role => masked", []string{"viewer"}, true},
		{"exempt role => cleartext", []string{"admin"}, false},
		{"second exempt role => cleartext", []string{"hr_lead"}, false},
		{"any exempt among many => cleartext", []string{"viewer", "operator", "admin"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskOne(t, e, "salary", "100000", PointPreview, tc.roles)
			if tc.masked && got != maskToken {
				t.Errorf("expected masked, got %v", got)
			}
			if !tc.masked && got != "100000" {
				t.Errorf("expected cleartext, got %v", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Nested structures
// ---------------------------------------------------------------------------

func TestMaskItems_NestedMapsAndSlices(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "salary", MatchType: MatchColumnName, Pattern: "(?i)salary", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	items := []map[string]any{
		{
			"name": "Alice",
			"detail": map[string]any{
				"salary": 50000,
				"dept":   "IT",
			},
			"history": []any{
				map[string]any{"salary": 10, "year": 2020},
				map[string]any{"salary": 20, "year": 2021},
			},
			"tags": []any{"a", "b"},
		},
	}
	out := e.MaskItems(items, PointPreview, nil)
	detail := out[0]["detail"].(map[string]any)
	if detail["salary"] != maskToken {
		t.Errorf("nested map salary not masked: %v", detail["salary"])
	}
	if detail["dept"] != "IT" {
		t.Errorf("nested non-sensitive changed: %v", detail["dept"])
	}
	hist := out[0]["history"].([]any)
	for i, h := range hist {
		m := h.(map[string]any)
		if m["salary"] != maskToken {
			t.Errorf("history[%d] salary not masked: %v", i, m["salary"])
		}
	}
	tags := out[0]["tags"].([]any)
	if !reflect.DeepEqual(tags, []any{"a", "b"}) {
		t.Errorf("scalar slice mutated: %v", tags)
	}
}

func TestMaskItems_TypedSliceOfMaps(t *testing.T) {
	// []map[string]any (typed, not []any) must also be traversed.
	e := mustEngine(t, []Rule{
		{Name: "salary", MatchType: MatchColumnName, Pattern: "(?i)salary", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	items := []map[string]any{
		{
			"rows": []map[string]any{
				{"salary": 1},
				{"salary": 2},
			},
		},
	}
	out := e.MaskItems(items, PointPreview, nil)
	rows := out[0]["rows"].([]map[string]any)
	for i, r := range rows {
		if r["salary"] != maskToken {
			t.Errorf("rows[%d] not masked: %v", i, r["salary"])
		}
	}
}

func TestMaskItems_NilItemInSliceHandled(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "salary", MatchType: MatchColumnName, Pattern: "(?i)salary", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	items := []map[string]any{
		nil,
		{"salary": 1},
	}
	out := e.MaskItems(items, PointPreview, nil)
	if out[0] != nil {
		t.Errorf("nil item should remain nil, got %#v", out[0])
	}
	if out[1]["salary"] != maskToken {
		t.Errorf("second item salary not masked: %v", out[1]["salary"])
	}
}

func TestMaskItems_EmptyInput(t *testing.T) {
	e := mustEngine(t, DefaultRules())
	if got := e.MaskItems(nil, PointPreview, nil); got != nil {
		t.Errorf("nil input should return nil, got %#v", got)
	}
	if got := e.MaskItems([]map[string]any{}, PointPreview, nil); len(got) != 0 {
		t.Errorf("empty input should return empty, got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// Deep copy / non-mutation
// ---------------------------------------------------------------------------

func TestMaskItems_DoesNotMutateInput(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "salary", MatchType: MatchColumnName, Pattern: "(?i)salary", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
	})
	nested := map[string]any{"salary": 50000, "dept": "IT"}
	sliceEl := map[string]any{"salary": 10}
	items := []map[string]any{
		{
			"salary":  99999,
			"detail":  nested,
			"history": []any{sliceEl},
		},
	}
	_ = e.MaskItems(items, PointPreview, nil)

	if items[0]["salary"] != 99999 {
		t.Errorf("top-level input mutated: %v", items[0]["salary"])
	}
	if nested["salary"] != 50000 {
		t.Errorf("nested input map mutated: %v", nested["salary"])
	}
	if sliceEl["salary"] != 10 {
		t.Errorf("input slice element mutated: %v", sliceEl["salary"])
	}
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestMaskItems_IdempotentFullAndPartial(t *testing.T) {
	e := mustEngine(t, []Rule{
		{Name: "full", MatchType: MatchColumnName, Pattern: "(?i)^full$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
		{Name: "l4", MatchType: MatchColumnName, Pattern: "(?i)^l4$", Style: StylePartialLast4, AppliesTo: []Point{PointPreview}, Active: true},
	})
	// full: mask(mask(x)) == mask(x)
	once := maskOne(t, e, "full", "1234567890", PointPreview, nil)
	twice := maskOne(t, e, "full", once, PointPreview, nil)
	if once != twice {
		t.Errorf("full not idempotent: %v != %v", once, twice)
	}
	// partial: masking the already masked "***...6789" keeps its last-4 which
	// are the mask chars, so it remains stable.
	p1 := maskOne(t, e, "l4", "1234567890", PointPreview, nil)
	p2 := maskOne(t, e, "l4", p1, PointPreview, nil)
	if p1 != p2 {
		t.Errorf("partial not idempotent: %v != %v", p1, p2)
	}
}

func TestMaskItems_HashIdempotencyIsHashOfHash(t *testing.T) {
	// Documented behavior: hash is NOT idempotent in the mask(mask(x))==mask(x)
	// sense — re-hashing yields hash(hash(x)). It is stable per input though.
	e := mustEngine(t, []Rule{
		{Name: "h", MatchType: MatchColumnName, Pattern: "(?i)^h$", Style: StyleHash, AppliesTo: []Point{PointPreview}, Active: true},
	})
	once := maskOne(t, e, "h", "x", PointPreview, nil).(string)
	twice := maskOne(t, e, "h", once, PointPreview, nil).(string)
	if once == twice {
		t.Errorf("expected hash(hash(x)) != hash(x)")
	}
	sum := sha256.Sum256([]byte(once))
	if twice != hex.EncodeToString(sum[:]) {
		t.Errorf("re-hash mismatch")
	}
}

// ---------------------------------------------------------------------------
// Regex match type on the KEY NAME (all match types match against the field name)
// ---------------------------------------------------------------------------

func TestMaskItems_MatchTypeRegexAndConnectionColumn(t *testing.T) {
	// All three match types compare the compiled pattern against the key name.
	for _, mt := range []MatchType{MatchColumnName, MatchRegex, MatchConnectionColumn} {
		e := mustEngine(t, []Rule{
			{Name: string(mt), MatchType: mt, Pattern: "(?i)^field$", Style: StyleFull, AppliesTo: []Point{PointPreview}, Active: true},
		})
		if got := maskOne(t, e, "field", "v", PointPreview, nil); got != maskToken {
			t.Errorf("match type %s did not mask, got %v", mt, got)
		}
	}
}

// ---------------------------------------------------------------------------
// DefaultRules
// ---------------------------------------------------------------------------

func TestDefaultRules_Shape(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Fatal("DefaultRules returned empty set")
	}
	for _, r := range rules {
		if !r.Active {
			t.Errorf("default rule %q should be active", r.Name)
		}
		if len(r.ExemptRoles) != 0 {
			t.Errorf("default rule %q should have no exempt roles by default, got %v", r.Name, r.ExemptRoles)
		}
		// enforced at preview+log+ai
		want := map[Point]bool{PointPreview: false, PointLog: false, PointAI: false}
		for _, p := range r.AppliesTo {
			want[p] = true
		}
		for p, ok := range want {
			if !ok {
				t.Errorf("default rule %q missing point %s", r.Name, p)
			}
		}
	}
	// must compile
	if _, err := NewEngine(rules); err != nil {
		t.Fatalf("DefaultRules do not compile: %v", err)
	}
}

func TestDefaultRules_MatchThaiAndEnglishColumns(t *testing.T) {
	e := mustEngine(t, DefaultRules())

	// Each category with English + Thai column-name variants that MUST be masked.
	cases := []struct {
		category string
		keys     []string
	}{
		{"salary", []string{"salary", "base_salary", "monthly_salary", "เงินเดือน", "salary_thb"}},
		{"citizen_id", []string{"citizen_id", "citizenid", "national_id", "เลขบัตรประชาชน", "เลขประจำตัวประชาชน"}},
		{"bank_account", []string{"bank_account", "bank_account_no", "account_number", "เลขที่บัญชี", "เลขบัญชี"}},
		{"phone", []string{"phone", "phone_number", "mobile", "tel", "เบอร์โทร", "โทรศัพท์"}},
		{"email", []string{"email", "email_address", "e_mail", "อีเมล", "อีเมล์"}},
	}
	for _, c := range cases {
		for _, k := range c.keys {
			t.Run(c.category+"/"+k, func(t *testing.T) {
				for _, p := range []Point{PointPreview, PointLog, PointAI} {
					got := maskOne(t, e, k, "sensitive-value", p, nil)
					if got == "sensitive-value" {
						t.Errorf("category %s: key %q not masked at %s", c.category, k, p)
					}
				}
			})
		}
	}
}

func TestDefaultRules_DoNotOverMaskBenignColumns(t *testing.T) {
	e := mustEngine(t, DefaultRules())
	benign := []string{"name", "department", "employee_code", "start_date", "position", "id"}
	for _, k := range benign {
		got := maskOne(t, e, k, "keep-me", PointPreview, nil)
		if got != "keep-me" {
			t.Errorf("benign column %q was masked: %v", k, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Performance
// ---------------------------------------------------------------------------

func TestMaskItems_Performance100k(t *testing.T) {
	e := mustEngine(t, DefaultRules())
	const n = 100_000
	items := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		items[i] = map[string]any{
			"employee_id": i,
			"name":        "Employee",
			"salary":      50000 + i,
			"citizen_id":  fmt.Sprintf("1%012d", i%1_000_000_000_000),
			"phone":       "0812345678",
			"email":       "e@example.com",
			"department":  "IT",
		}
	}
	start := time.Now()
	out := e.MaskItems(items, PointLog, nil)
	elapsed := time.Since(start)
	if len(out) != n {
		t.Fatalf("expected %d items, got %d", n, len(out))
	}
	// spot check masking really happened
	if out[0]["salary"] == 50000 {
		t.Fatal("salary not masked in perf run")
	}
	// The wall-clock bound is only meaningful without the race detector, whose
	// instrumentation inflates timing 5-10x. Under -race the 100k masking still
	// runs (correctness + coverage), but the <1s assertion is skipped.
	if raceEnabled {
		t.Logf("skipping <1s wall-clock assertion under -race; masked %d items in %v", n, elapsed)
		return
	}
	if elapsed >= time.Second {
		t.Fatalf("masking 100k items took %v, want < 1s", elapsed)
	}
	t.Logf("masked %d items in %v", n, elapsed)
}
