package expression

import (
	"fmt"
	"strconv"
	"time"
)

// coerceString renders any value as a string for EvalString and the string
// pipe functions. Times use RFC3339; whole floats drop the fractional part.
func coerceString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case time.Time:
		return t.Format(time.RFC3339)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case int:
		return strconv.Itoa(t)
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
	default:
		// Maps, slices and any other value: last-resort formatting. This is a
		// pure formatting call with no IO.
		return fmt.Sprintf("%v", v)
	}
}

// toFloat coerces a value to float64 for numeric pipes. Numeric strings are
// parsed; non-numeric values error.
func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", n)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("value of type %T is not numeric", v)
	}
}

// asTime coerces a value to time.Time. Strings are parsed as RFC3339.
func asTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return time.Time{}, fmt.Errorf("%q is not an RFC3339 time", t)
		}
		return parsed, nil
	default:
		return time.Time{}, fmt.Errorf("value of type %T is not a time", v)
	}
}
