package mutator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MutateJSON parses raw JSON, applies dot-path key=value modifications, and returns the modified JSON.
func MutateJSON(raw []byte, mutations []string) ([]byte, error) {
	if len(mutations) == 0 {
		return raw, nil
	}

	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parsing target JSON for mutation: %w", err)
	}

	rootMap, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root JSON must be an object")
	}

	for _, m := range mutations {
		parts := strings.SplitN(m, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid mutation expression (expected key.path=val): %s", m)
		}
		path := strings.TrimSpace(parts[0])
		rawVal := strings.TrimSpace(parts[1])
		val := parseValue(rawVal)

		if err := setPath(rootMap, path, val); err != nil {
			return nil, err
		}
	}

	return json.Marshal(rootMap)
}

func parseValue(v string) any {
	if v == "null" {
		return nil
	}
	if v == "true" {
		return true
	}
	if v == "false" {
		return false
	}
	if intVal, err := strconv.ParseInt(v, 10, 64); err == nil {
		return intVal
	}
	if floatVal, err := strconv.ParseFloat(v, 64); err == nil {
		return floatVal
	}
	// Check if it's already a JSON sub-structure
	if (strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")) ||
		(strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]")) {
		var sub any
		if err := json.Unmarshal([]byte(v), &sub); err == nil {
			return sub
		}
	}
	// Default to trimmed string (strip wrapping quotes if provided)
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	return v
}

func setPath(current map[string]any, path string, val any) error {
	keys := strings.Split(path, ".")
	if len(keys) == 0 || keys[0] == "" {
		return fmt.Errorf("empty key path")
	}

	curr := current
	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		next, exists := curr[k]
		if !exists || next == nil {
			newMap := make(map[string]any)
			curr[k] = newMap
			curr = newMap
			continue
		}

		nextMap, ok := next.(map[string]any)
		if !ok {
			// Replace non-map with new map
			newMap := make(map[string]any)
			curr[k] = newMap
			curr = newMap
			continue
		}
		curr = nextMap
	}

	leafKey := keys[len(keys)-1]
	curr[leafKey] = val
	return nil
}
