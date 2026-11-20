package viperr

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var durationType = reflect.TypeOf(time.Duration(0))

// builtinTagNames are implemented by the engine itself. A user rule or alias
// named after one of them is rejected unless WithBuiltinOverride is set.
var builtinTagNames = map[string]bool{
	"required": true, "omitempty": true, "required_without": true,
	"gt": true, "gte": true, "lt": true, "lte": true,
	"min": true, "max": true, "eq": true, "ne": true,
	"oneof": true, "port": true,
	"ip": true, "ipv4": true, "ipv6": true,
	"fqdn": true, "hostname_rfc1123": true, "dns_rfc1035_label": true,
	"dirpath": true, "duration": true,
}

// builtinRules returns the engine tags. required, omitempty, and
// required_without are directives handled by the engine, not Func rules.
func builtinRules() map[string]ruleEntry {
	return map[string]ruleEntry{
		"gt":  {fn: makeCmp(func(c int) bool { return c > 0 })},
		"gte": {fn: makeCmp(func(c int) bool { return c >= 0 })},
		"lt":  {fn: makeCmp(func(c int) bool { return c < 0 })},
		"lte": {fn: makeCmp(func(c int) bool { return c <= 0 })},
		"min": {fn: makeCmp(func(c int) bool { return c >= 0 })},
		"max": {fn: makeCmp(func(c int) bool { return c <= 0 })},
		"eq":  {fn: makeCmp(func(c int) bool { return c == 0 })},
		"ne":  {fn: makeCmp(func(c int) bool { return c != 0 })},

		"oneof": {fn: oneofRule},
		"port":  {fn: portRule},

		"ip": {fn: func(fl FieldLevel) bool { return net.ParseIP(fieldString(fl.Field())) != nil }},
		"ipv4": {fn: func(fl FieldLevel) bool {
			ip := net.ParseIP(fieldString(fl.Field()))
			return ip != nil && ip.To4() != nil
		}},
		"ipv6": {fn: func(fl FieldLevel) bool {
			s := fieldString(fl.Field())
			return net.ParseIP(s) != nil && strings.Contains(s, ":")
		}},

		"fqdn":              {fn: func(fl FieldLevel) bool { return fqdnRe.MatchString(fieldString(fl.Field())) }},
		"hostname_rfc1123":  {fn: func(fl FieldLevel) bool { return isHostname(fieldString(fl.Field())) }},
		"dns_rfc1035_label": {fn: func(fl FieldLevel) bool { return dns1035Re.MatchString(fieldString(fl.Field())) }},

		"dirpath":  {fn: func(fl FieldLevel) bool { s := fieldString(fl.Field()); return s != "" && !strings.ContainsRune(s, 0) }},
		"duration": {fn: func(fl FieldLevel) bool { _, err := time.ParseDuration(fieldString(fl.Field())); return err == nil }},
	}
}

// shippedRules are the rules viperr ships on top of the engine tags.
// WithRules may replace any of them.
func shippedRules() []Rule {
	return []Rule{
		{Tag: "notblank", Fn: notBlankRule, Message: "{field} must not be blank"},
		{Tag: "mindur", Fn: minDurationRule, Message: "{field} must be >= {param}"},
		{Tag: "maxdur", Fn: maxDurationRule, Message: "{field} must be <= {param}"},
		{Tag: "listen_addr", Fn: listenAddrRule, Message: "{field} must be [host]:port"},
		{Tag: "oneof_ci", Fn: oneOfCIRule, Message: "{field} must be one of: {param}"},
		{Tag: "writable_dir", Fn: writableDirRule, Message: "{field} must be a writable directory"},
		{Tag: "pg_ident", Fn: pgIdentRule, Message: "{field} must be a valid PostgreSQL identifier (<=63 chars)"},
	}
}

// shippedAliases expand one tag into a tag chain. WithAliases may replace them.
func shippedAliases() map[string]string {
	return map[string]string{
		"positive":   "gt=0",
		"nonneg":     "gte=0",
		"host":       "hostname_rfc1123|ip|fqdn",
		"log_level":  "oneof=trace debug info warn error fatal panic",
		"pg_sslmode": "oneof=disable allow prefer require verify-ca verify-full",
	}
}

// defaultMessage renders the failure of a built-in tag. str is true when the
// field is a string, in which case comparisons apply to its length.
func defaultMessage(tag string, str bool) string {
	if str {
		switch tag {
		case "gt":
			return "{field} length must be > {param}"
		case "gte", "min":
			return "{field} length must be >= {param}"
		case "lt":
			return "{field} length must be < {param}"
		case "lte", "max":
			return "{field} length must be <= {param}"
		case "eq":
			return "{field} length must be == {param}"
		case "ne":
			return "{field} length must be != {param}"
		}
	}
	switch tag {
	case "required":
		return "{field} is required"
	case "required_without":
		return "{field} is required when {param} is not set"
	case "gt":
		return "{field} must be > {param}"
	case "gte", "min":
		return "{field} must be >= {param}"
	case "lt":
		return "{field} must be < {param}"
	case "lte", "max":
		return "{field} must be <= {param}"
	case "eq":
		return "{field} must be equal to {param}"
	case "ne":
		return "{field} must not be equal to {param}"
	case "oneof":
		return "{field} must be one of: {param}"
	case "port":
		return "{field} must be a valid port number"
	case "ip":
		return "{field} must be a valid IP address"
	case "ipv4":
		return "{field} must be a valid IPv4 address"
	case "ipv6":
		return "{field} must be a valid IPv6 address"
	case "fqdn":
		return "{field} must be a valid FQDN"
	case "hostname_rfc1123":
		return "{field} must be a valid hostname"
	case "dns_rfc1035_label":
		return "{field} must be a valid DNS label"
	case "dirpath":
		return "{field} must be a valid directory path"
	case "duration":
		return "{field} must be a valid duration"
	default:
		return "{field} failed validation"
	}
}

// fieldString reads a field as a string. Numbers are formatted, so port works
// on both string and int fields.
func fieldString(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	default:
		return ""
	}
}

// makeCmp builds a comparison rule. cmp receives the sign of field-vs-param:
// negative when the field is smaller, zero when equal, positive when greater.
// Strings compare by length, durations may use duration params ("1s").
func makeCmp(cmp func(c int) bool) Func {
	return func(fl FieldLevel) bool {
		c, ok := compareWithParam(fl)
		return ok && cmp(c)
	}
}

func compareWithParam(fl FieldLevel) (int, bool) {
	f := fl.Field()
	p := strings.TrimSpace(fl.Param())
	switch f.Kind() {
	case reflect.String:
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return 0, false
		}
		return compareInt(int64(len(f.String())), n), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		if f.Type() == durationType {
			if d, err := time.ParseDuration(p); err == nil {
				n = int64(d)
			} else if v, err := strconv.ParseInt(p, 10, 64); err == nil {
				n = v
			} else {
				return 0, false
			}
		} else {
			v, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return 0, false
			}
			n = v
		}
		return compareInt(f.Int(), n), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return 0, false
		}
		return compareUint(f.Uint(), n), true
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		return compareFloat(f.Float(), n), true
	case reflect.Bool:
		n, err := strconv.ParseBool(p)
		if err != nil {
			return 0, false
		}
		if f.Bool() == n {
			return 0, true
		}
		return 1, true
	default:
		return 0, false
	}
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func oneofRule(fl FieldLevel) bool {
	f := fl.Field()
	if f.Kind() != reflect.String {
		return false
	}
	return slices.Contains(strings.Fields(fl.Param()), f.String())
}

func portRule(fl FieldLevel) bool {
	n, err := strconv.Atoi(strings.TrimSpace(fieldString(fl.Field())))
	return err == nil && n >= 0 && n <= 65535
}

var (
	hostnameLabelRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	fqdnRe          = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	dns1035Re       = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`)
	pgIdentRe       = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]{0,62}$`)
)

// isHostname reports whether s is a valid RFC 1123 hostname: dot-separated
// labels of letters, digits, and hyphens, at most 253 characters in total.
// A single label ("localhost") is valid.
func isHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostnameLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// --- shipped rules ---

func notBlankRule(fl FieldLevel) bool {
	f := fl.Field()
	return f.Kind() == reflect.String && strings.TrimSpace(f.String()) != ""
}

// minDurationRule implements mindur=1s on time.Duration fields.
// A malformed parameter is a bug in the struct tag, so it panics.
func minDurationRule(fl FieldLevel) bool {
	min, err := time.ParseDuration(fl.Param())
	if err != nil {
		panic("viperr: bad mindur param " + fl.Param())
	}
	f := fl.Field()
	return f.Kind() == reflect.Int64 && f.Int() >= int64(min)
}

func maxDurationRule(fl FieldLevel) bool {
	max, err := time.ParseDuration(fl.Param())
	if err != nil {
		panic("viperr: bad maxdur param " + fl.Param())
	}
	f := fl.Field()
	return f.Kind() == reflect.Int64 && f.Int() <= int64(max)
}

// listenAddrRule accepts ":8080", "0.0.0.0:8080", "localhost:8080".
// A bare hostname_port would reject the empty host that net.Listen accepts.
func listenAddrRule(fl FieldLevel) bool {
	s := strings.TrimSpace(fieldString(fl.Field()))
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	if host != "" && net.ParseIP(host) == nil && !isHostname(host) {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

// oneOfCIRule implements oneof_ci: a case-insensitive oneof.
func oneOfCIRule(fl FieldLevel) bool {
	f := fl.Field()
	if f.Kind() != reflect.String {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(f.String()))
	for _, p := range strings.Fields(fl.Param()) {
		if v == strings.ToLower(p) {
			return true
		}
	}
	return false
}

// writableDirRule reports whether dir is a writable directory. When dir does
// not exist yet, the nearest existing ancestor is checked instead, so a log
// directory the application creates on startup still passes.
func writableDirRule(fl FieldLevel) bool {
	dir := strings.TrimSpace(fieldString(fl.Field()))
	if dir == "" {
		return false
	}
	for {
		info, err := os.Stat(dir)
		if err == nil {
			return info.IsDir() && dirWritable(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".viperr-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// pgIdentRule accepts unquoted PostgreSQL identifiers: a letter or underscore,
// then letters, digits, _, or $, at most 63 bytes (Postgres truncates longer
// names silently).
func pgIdentRule(fl FieldLevel) bool {
	f := fl.Field()
	return f.Kind() == reflect.String && pgIdentRe.MatchString(f.String())
}
