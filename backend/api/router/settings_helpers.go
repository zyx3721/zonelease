package router

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func parseSettingsResourcePath(path, prefix string) (id string, action string, ok bool) {
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func normalizeRequestStrings(values []string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func isAllowedValue(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case json.Number:
		number, _ := typed.Float64()
		return number
	case string:
		number, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number
	default:
		return 0
	}
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func removeEmptyConfigValues(config map[string]any) map[string]any {
	cleaned := make(map[string]any, len(config))
	for key, value := range config {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				cleaned[key] = strings.TrimSpace(typed)
			}
		case []string:
			items := make([]string, 0, len(typed))
			for _, item := range typed {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					items = append(items, trimmed)
				}
			}
			if len(items) > 0 {
				cleaned[key] = items
			}
		case []any:
			items := make([]any, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok {
					if trimmed := strings.TrimSpace(text); trimmed != "" {
						items = append(items, trimmed)
					}
					continue
				}
				if item != nil {
					items = append(items, item)
				}
			}
			if len(items) > 0 {
				cleaned[key] = items
			}
		case map[string]any:
			if nested := removeEmptyConfigValues(typed); len(nested) > 0 {
				cleaned[key] = nested
			}
		case map[string]string:
			items := make(map[string]string, len(typed))
			for itemKey, itemValue := range typed {
				if trimmed := strings.TrimSpace(itemValue); trimmed != "" {
					items[itemKey] = trimmed
				}
			}
			if len(items) > 0 {
				cleaned[key] = items
			}
		case nil:
			continue
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || typed <= 0 {
				continue
			}
			cleaned[key] = typed
		case float32:
			if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) || typed <= 0 {
				continue
			}
			cleaned[key] = typed
		case int:
			if typed > 0 {
				cleaned[key] = typed
			}
		case int8:
			if typed > 0 {
				cleaned[key] = typed
			}
		case int16:
			if typed > 0 {
				cleaned[key] = typed
			}
		case int32:
			if typed > 0 {
				cleaned[key] = typed
			}
		case int64:
			if typed > 0 {
				cleaned[key] = typed
			}
		case uint:
			if typed > 0 {
				cleaned[key] = typed
			}
		case uint8:
			if typed > 0 {
				cleaned[key] = typed
			}
		case uint16:
			if typed > 0 {
				cleaned[key] = typed
			}
		case uint32:
			if typed > 0 {
				cleaned[key] = typed
			}
		case uint64:
			if typed > 0 {
				cleaned[key] = typed
			}
		default:
			cleaned[key] = value
		}
	}
	return cleaned
}
