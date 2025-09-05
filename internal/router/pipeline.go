package router

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Đăng ký các filter có tên trùng với YAML.
// Bạn có thể thay thế bằng CEL sau này (cel-go).

func BuiltinFilters() map[string]FilterFn {
	return map[string]FilterFn{
		// ví dụ từ YAML:
		// expr: 'meta.size <= 1048576'
		"size_le_1mb": func(ctx context.Context, msg *Message, obj map[string]any) (bool, error) {
			sz, _ := getInt64(msg.Meta, "size")
			return sz <= 1024*1024, nil
		},
		// expr: 'meta.createdAtMs >= nowMs() - 3600 * 1000'
		"recent_1h": func(ctx context.Context, msg *Message, obj map[string]any) (bool, error) {
			created, _ := getInt64(msg.Meta, "createdAtMs")
			if created == 0 {
				// nếu thiếu trường, coi như fail (giống on_missing_field: drop)
				return false, nil
			}
			return time.Now().UnixMilli()-created <= 3600*1000, nil
		},
		// expr: 'has(payload.name) && (payload.age > 16 || payload.name.contains("A"))'
		"user_basic": func(ctx context.Context, msg *Message, obj map[string]any) (bool, error) {
			if obj == nil {
				return false, nil
			}

			if obj["name"] == nil || obj["age"] == nil {
				return false, nil
			}
			name := ""
			age := int64(0)
			if v, ok := obj["name"].(string); ok {
				name = v
			}
			if v, ok := strconv.ParseInt(obj["age"].(string), 10, 64); ok != nil {
				return false, nil
			} else {
				age = v
			}
			if name == "" {
				return false, nil
			}
			return age > 16 || strings.Contains(name, "A"), nil
		},
	}
}

func BuiltinProjections() map[string]ProjectFn {
	return map[string]ProjectFn{
		"keep_name_age_best_effort": func(ctx context.Context, msg *Message, obj map[string]any) (map[string]any, error) {
			if obj == nil {
				return map[string]any{}, nil
			}
			out := map[string]any{}
			if v, ok := obj["name"]; ok {
				out["name"] = v
			}
			if v, ok := obj["age"]; ok {
				out["age"] = v
			}
			return out, nil
		},
		"keep_name_age_strict": func(ctx context.Context, msg *Message, obj map[string]any) (map[string]any, error) {
			if obj == nil {
				return nil, nil // sẽ fail ở Encode -> drop
			}
			if _, ok := obj["name"]; !ok {
				return nil, nil
			}
			if _, ok := obj["age"]; !ok {
				return nil, nil
			}
			return map[string]any{
				"name": obj["name"],
				"age":  obj["age"],
			}, nil
		},
	}
}
