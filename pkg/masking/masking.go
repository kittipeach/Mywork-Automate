// Package masking implements MyWork Automate's column-level data masking
// engine (story E2-S4, MVP subset). It applies configurable masking rules to
// tabular result sets ("items") at the server-side enforcement points defined
// in docs/spec/07-security-testing.md §2.2 — query preview, execution I/O log,
// and AI/LLM egress.
//
// Design guarantees:
//   - Masking is one-way by construction for the full and partial_last4 styles;
//     originals are never stored and cannot be recovered from the output.
//   - The hash style is deterministic (equal inputs hash equal) so masked
//     values remain comparable, at the cost of not being idempotent under
//     re-masking (see MaskItems).
//   - MaskItems returns a deep copy; the caller's input is never mutated.
//
// The package depends only on the Go standard library.
package masking

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
)

// maskToken is the fixed replacement used by StyleFull. It is a constant so it
// never leaks length-sensitive information about the original value.
const maskToken = "******"

// maskChar is the character used to pad hidden positions in StylePartialLast4.
const maskChar = '*'

// Style selects how a matched value is transformed.
type Style string

const (
	// StyleFull replaces the whole value with a fixed token (maskToken).
	StyleFull Style = "full"
	// StylePartialLast4 keeps at most the last four characters visible and
	// masks the rest, e.g. "***-**-6789" -> "*******6789".
	StylePartialLast4 Style = "partial_last4"
	// StyleHash replaces the value with the hex SHA-256 of its string form.
	StyleHash Style = "hash"
)

// MatchType describes how a rule's pattern is matched. In every case the
// compiled pattern is evaluated against the field/column name; the distinction
// is metadata carried from masking_rules.match_type for admin/reporting use.
type MatchType string

const (
	MatchColumnName       MatchType = "column_name"
	MatchRegex            MatchType = "regex"
	MatchConnectionColumn MatchType = "connection_column"
)

// Point is an enforcement point where masking is applied
// (docs/spec/07 §2.2): preview | log | ai.
type Point string

const (
	PointPreview Point = "preview"
	PointLog     Point = "log"
	PointAI      Point = "ai"
)

// Rule is a single masking rule, mirroring the masking_rules table
// (docs/spec/06-data-model-api.md).
type Rule struct {
	Name        string
	MatchType   MatchType
	Pattern     string // regex matched against the column/field name
	Style       Style
	ExemptRoles []string // role codes that see cleartext
	AppliesTo   []Point  // points where this rule is enforced
	Active      bool
}

// compiledRule is a Rule with its pattern compiled and lookup sets prebuilt.
type compiledRule struct {
	rule    Rule
	re      *regexp.Regexp
	points  map[Point]struct{}
	exempts map[string]struct{}
}

// Engine holds the compiled rule set and applies masking to items.
type Engine struct {
	rules []compiledRule
}

// NewEngine compiles rules; it returns an error if any rule's Pattern is not a
// valid regular expression. Inactive rules are compiled too so that
// configuration errors surface regardless of the Active flag.
func NewEngine(rules []Rule) (*Engine, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("masking: invalid pattern for rule %q: %w", r.Name, err)
		}
		points := make(map[Point]struct{}, len(r.AppliesTo))
		for _, p := range r.AppliesTo {
			points[p] = struct{}{}
		}
		exempts := make(map[string]struct{}, len(r.ExemptRoles))
		for _, role := range r.ExemptRoles {
			exempts[role] = struct{}{}
		}
		compiled = append(compiled, compiledRule{
			rule:    r,
			re:      re,
			points:  points,
			exempts: exempts,
		})
	}
	return &Engine{rules: compiled}, nil
}

// DefaultRules returns the MVP default rule set covering salary, citizen_id,
// bank_account, phone and email, matching both English and Thai column-name
// variants. All rules are active, enforced at preview+log+ai, with no exempt
// roles by default (security-reviewed defaults — Admin UI to tune them is P2).
func DefaultRules() []Rule {
	all := []Point{PointPreview, PointLog, PointAI}
	return []Rule{
		{
			Name:      "salary",
			MatchType: MatchColumnName,
			// salary / *salary* and Thai เงินเดือน
			Pattern:   `(?i)(salary|เงินเดือน)`,
			Style:     StyleFull,
			AppliesTo: all,
			Active:    true,
		},
		{
			Name:      "citizen_id",
			MatchType: MatchColumnName,
			// citizen_id / citizenid / citizen / national_id and Thai variants
			Pattern:   `(?i)(citizen[_]?id|citizen|national[_]?id|เลขบัตรประชาชน|เลขประจำตัวประชาชน|บัตรประชาชน)`,
			Style:     StylePartialLast4,
			AppliesTo: all,
			Active:    true,
		},
		{
			Name:      "bank_account",
			MatchType: MatchColumnName,
			Pattern:   `(?i)(bank[_]?account|account[_]?(no|number|num)|acct[_]?(no|number)|เลขที่บัญชี|เลขบัญชี|บัญชีธนาคาร)`,
			Style:     StylePartialLast4,
			AppliesTo: all,
			Active:    true,
		},
		{
			Name:      "phone",
			MatchType: MatchColumnName,
			Pattern:   `(?i)(phone|mobile|(^|_)tel(_|$|ephone)|เบอร์โทร|โทรศัพท์|เบอร์มือถือ)`,
			Style:     StylePartialLast4,
			AppliesTo: all,
			Active:    true,
		},
		{
			Name:      "email",
			MatchType: MatchColumnName,
			Pattern:   `(?i)(e[_-]?mail|อีเมล)`,
			Style:     StyleHash,
			AppliesTo: all,
			Active:    true,
		},
	}
}

// MaskItems returns a masked deep copy of items for the given point and viewer
// roles. A field is masked when some active rule matches its (possibly nested)
// key name, that rule applies at point, and none of the viewer's roles are in
// the rule's ExemptRoles. Nested maps and slices of maps are traversed. The
// input is never mutated.
//
// Hash-style masking is deterministic (equal cleartext -> equal digest) so
// values stay comparable across records; it is therefore NOT idempotent under
// re-masking: masking an already-hashed value yields hash(hash(x)). The full
// and partial_last4 styles are stable under re-masking.
func (e *Engine) MaskItems(items []map[string]any, point Point, viewerRoles []string) []map[string]any {
	if items == nil {
		return nil
	}
	roles := make(map[string]struct{}, len(viewerRoles))
	for _, r := range viewerRoles {
		roles[r] = struct{}{}
	}
	// point and roles are constant for the whole call, so the mask decision
	// depends only on the field name. Memoise it: column names repeat heavily
	// across rows, collapsing millions of regex evaluations to one per name.
	ctx := &maskCtx{
		point: point,
		roles: roles,
		cache: make(map[string]decision),
	}
	out := make([]map[string]any, len(items))
	for i, item := range items {
		out[i] = e.maskMap(item, ctx)
	}
	return out
}

// decision is a memoised styleFor result.
type decision struct {
	style Style
	mask  bool
}

// maskCtx carries the invariant inputs of a single MaskItems call plus the
// per-call field-name decision cache.
type maskCtx struct {
	point Point
	roles map[string]struct{}
	cache map[string]decision
}

// maskMap returns a deep-copied, masked version of m.
func (e *Engine) maskMap(m map[string]any, ctx *maskCtx) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if d := e.styleFor(k, ctx); d.mask {
			out[k] = e.maskValue(v, d.style)
			continue
		}
		out[k] = e.copyValue(v, ctx)
	}
	return out
}

// styleFor reports whether key must be masked at ctx.point for ctx.roles, and
// if so the Style of the first matching active rule. Results are memoised.
func (e *Engine) styleFor(key string, ctx *maskCtx) decision {
	if d, ok := ctx.cache[key]; ok {
		return d
	}
	d := e.evalStyle(key, ctx.point, ctx.roles)
	ctx.cache[key] = d
	return d
}

// evalStyle performs the uncached rule evaluation for key.
func (e *Engine) evalStyle(key string, point Point, roles map[string]struct{}) decision {
	for _, cr := range e.rules {
		if !cr.rule.Active {
			continue
		}
		if _, ok := cr.points[point]; !ok {
			continue
		}
		if !cr.re.MatchString(key) {
			continue
		}
		if roleExempt(cr.exempts, roles) {
			continue
		}
		return decision{style: cr.rule.Style, mask: true}
	}
	return decision{}
}

// roleExempt reports whether any of the viewer's roles is in exempts.
func roleExempt(exempts, roles map[string]struct{}) bool {
	for r := range roles {
		if _, ok := exempts[r]; ok {
			return true
		}
	}
	return false
}

// copyValue deep-copies a non-masked value, recursing into nested maps and
// slices so that their sensitive fields are still masked.
func (e *Engine) copyValue(v any, ctx *maskCtx) any {
	switch val := v.(type) {
	case map[string]any:
		return e.maskMap(val, ctx)
	case []map[string]any:
		cp := make([]map[string]any, len(val))
		for i := range val {
			cp[i] = e.maskMap(val[i], ctx)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i := range val {
			cp[i] = e.copyValue(val[i], ctx)
		}
		return cp
	default:
		return v
	}
}

// maskValue applies style to a scalar value. nil and non string/numeric values
// are returned unchanged (nothing to mask).
func (e *Engine) maskValue(v any, style Style) any {
	s, ok := stringify(v)
	if !ok {
		return v
	}
	switch style {
	case StyleFull:
		return maskToken
	case StylePartialLast4:
		return partialLast4(s)
	case StyleHash:
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	default:
		// Unknown style: leave the value untouched rather than risk leaking
		// or corrupting data.
		return v
	}
}

// partialLast4 masks all but the last four characters of s with maskChar. For
// values of four characters or fewer, everything is masked (never revealing
// more than the last four, and nothing at all when there are four or fewer).
func partialLast4(s string) string {
	r := []rune(s)
	n := len(r)
	if n <= 4 {
		return string(repeat(maskChar, n))
	}
	out := repeat(maskChar, n-4)
	return string(out) + string(r[n-4:])
}

// repeat returns a slice of n copies of c.
func repeat(c rune, n int) []rune {
	out := make([]rune, n)
	for i := range out {
		out[i] = c
	}
	return out
}

// stringify converts string and numeric values to their string form. It
// reports ok=false for nil and any non string/numeric type.
func stringify(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return n, true
	case int:
		return strconv.FormatInt(int64(n), 10), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64), true
	default:
		return "", false
	}
}
