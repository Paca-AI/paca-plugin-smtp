package main

import (
	"fmt"
	"strconv"
)

// toString converts a DB row value (as decoded from the host's JSON bridge)
// to a string. Handles nil (NULL) and non-string scalar types defensively.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// toInt converts a DB row value to an int. Numeric columns come back as
// float64 through the host's JSON bridge.
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

// toBool converts a DB row value to a bool.
func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "t" || b == "1"
	default:
		return false
	}
}

// containsString reports whether want is present in list.
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
