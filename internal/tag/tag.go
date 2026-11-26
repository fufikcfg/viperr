// Package tag parses the config struct tag into a field specification.
// It is internal: only code inside the viperr module may import it.
package tag

import (
	"fmt"
	"strings"
)

// Spec is one parsed config tag.
type Spec struct {
	Key      string
	Source   string
	HasDef   bool
	Def      string
	Path     bool
	NoAbs    bool
	Secret   bool
	Oneof    []string
	HasOneof bool
}

// Parse reads a config tag: the first element is the key, the rest are
// options separated by commas. A backslash escapes the next character, so
// `\,` is a literal comma inside an option value (e.g. a default list) and
// `\\` is a literal backslash.
//
// Options: default=VALUE, path, noabs, secret, oneof=A|B.
func Parse(tag string) (Spec, error) {
	var spec Spec
	parts := split(tag)
	spec.Key = strings.TrimSpace(parts[0])
	if spec.Key == "" {
		return Spec{}, fmt.Errorf("empty key")
	}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, hasValue := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		switch name {
		case "path":
			if hasValue {
				return Spec{}, fmt.Errorf("path does not take a value")
			}
			spec.Path = true
		case "noabs":
			if hasValue {
				return Spec{}, fmt.Errorf("noabs does not take a value")
			}
			spec.NoAbs = true
		case "secret":
			if hasValue {
				return Spec{}, fmt.Errorf("secret does not take a value")
			}
			spec.Secret = true
		case "default":
			if !hasValue {
				return Spec{}, fmt.Errorf("default needs a value")
			}
			spec.HasDef = true
			spec.Def = value
		case "oneof":
			if !hasValue || strings.TrimSpace(value) == "" {
				return Spec{}, fmt.Errorf("oneof needs a value")
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
				return Spec{}, fmt.Errorf("oneof needs a value")
			}
			spec.HasOneof = true
			spec.Oneof = items
		default:
			return Spec{}, fmt.Errorf("unknown option %q", part)
		}
	}
	return spec, nil
}

// split divides tag on commas. A backslash escapes the next character:
// `\,` is a literal comma, `\\` is a literal backslash.
func split(tag string) []string {
	var parts []string
	var b strings.Builder
	b.Grow(len(tag))
	escaped := false
	for _, r := range tag {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ',':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteByte('\\')
	}
	parts = append(parts, b.String())
	return parts
}
