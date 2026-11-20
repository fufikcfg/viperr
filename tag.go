package viperr

import (
	"fmt"
	"strings"
)

type fieldSpec struct {
	key      string
	source   string
	required bool
	hasDef   bool
	def      string
	path     bool
	oneof    []string
	hasOneof bool
}

func parseTag(tag string) (fieldSpec, error) {
	var spec fieldSpec
	parts := strings.Split(tag, ",")
	spec.key = strings.TrimSpace(parts[0])
	if spec.key == "" {
		return fieldSpec{}, fmt.Errorf("empty key")
	}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, hasValue := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		switch name {
		case "required":
			if hasValue {
				return fieldSpec{}, fmt.Errorf("required does not take a value")
			}
			spec.required = true
		case "path":
			if hasValue {
				return fieldSpec{}, fmt.Errorf("path does not take a value")
			}
			spec.path = true
		case "default":
			if !hasValue {
				return fieldSpec{}, fmt.Errorf("default needs a value")
			}
			spec.hasDef = true
			spec.def = value
		case "oneof":
			if !hasValue || strings.TrimSpace(value) == "" {
				return fieldSpec{}, fmt.Errorf("oneof needs a value")
			}
			var items []string
			for _, item := range strings.Split(value, "|") {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				items = append(items, item)
			}
			if len(items) == 0 {
				return fieldSpec{}, fmt.Errorf("oneof needs a value")
			}
			spec.hasOneof = true
			spec.oneof = items
		default:
			return fieldSpec{}, fmt.Errorf("unknown option %q", part)
		}
	}
	return spec, nil
}
