package viperr

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// FieldLevel gives a validation rule access to the field being checked.
type FieldLevel interface {
	// Field is the field value with pointers dereferenced.
	// A nil pointer is the zero value of its element type.
	Field() reflect.Value
	// Param is the tag parameter, the part after "=" ("" when absent).
	Param() string
	// Key is the config key of the field, or its Go name when untagged.
	Key() string
	// Parent is the struct the field belongs to, for cross-field rules.
	Parent() reflect.Value
}

// Func reports whether a field value satisfies a rule.
type Func func(fl FieldLevel) bool

// Rule adds a validation rule or replaces a shipped one (notblank, mindur,
// maxdur, listen_addr, oneof_ci, writable_dir, pg_ident). Replacing a
// built-in tag (required, gt, port, ...) needs WithBuiltinOverride.
type Rule struct {
	Tag string
	Fn  Func
	// Message is the failure text; {field} and {param} are substituted.
	// Empty falls back to the tag default.
	Message string
	// EvenIfNull calls Fn for a nil pointer. By default a nil pointer fails
	// required and skips every other rule.
	EvenIfNull bool
}

// StructRule runs Fn for every struct of the Sample type reached during
// validation, including the root. Fn receives a pointer to the struct when it
// is addressable, a copy otherwise.
type StructRule struct {
	Sample any
	Fn     func(v any) error
}

type valSettings struct {
	rules                []Rule
	aliases              map[string]string
	structRules          []StructRule
	allowBuiltinOverride bool
}

// Option customizes tag validation: pass to Validate, Load,
// or Options.Validation of Decode.
type Option func(*valSettings)

// WithRules adds rules. A duplicate tag inside one call is an error.
// A rule named after a shipped rule replaces it; a rule named after a
// built-in tag is an error unless WithBuiltinOverride is set.
func WithRules(rules ...Rule) Option {
	return func(s *valSettings) { s.rules = append(s.rules, rules...) }
}

// WithAliases adds tag aliases. An alias expands inline where its name
// appears: "positive" -> "gt=0", "host" -> "hostname_rfc1123|ip|fqdn".
// An alias named after a shipped alias replaces it; an alias named after a
// built-in tag is an error unless WithBuiltinOverride is set.
func WithAliases(aliases map[string]string) Option {
	return func(s *valSettings) {
		if s.aliases == nil {
			s.aliases = make(map[string]string, len(aliases))
		}
		for k, v := range aliases {
			s.aliases[k] = v
		}
	}
}

// WithStructRules adds struct-level checks, for rules a single field
// cannot express.
func WithStructRules(rules ...StructRule) Option {
	return func(s *valSettings) { s.structRules = append(s.structRules, rules...) }
}

// WithBuiltinOverride allows WithRules and WithAliases to replace built-in
// tags such as gt or port. It changes the meaning of those tags for every
// struct validated with the same options.
func WithBuiltinOverride() Option {
	return func(s *valSettings) { s.allowBuiltinOverride = true }
}

// FieldError is one failed rule on one field.
type FieldError struct {
	Key   string // config key or Go field name
	Tag   string // the rule that failed, e.g. "gt"
	Param string // the rule parameter, e.g. "0"
	msg   string
}

func (e FieldError) Error() string { return e.msg }

// Errors is every rule failure found by one Validate call.
type Errors []FieldError

func (e Errors) Error() string {
	var b strings.Builder
	for i, fe := range e {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(fe.msg)
	}
	return b.String()
}

// Validate checks dst against its validate tags. dst must be a pointer to a
// struct; nested structs are validated recursively. A field named in an error
// is its config key (APP_SYNC_LOOP), or its Go name when untagged.
//
//	validate:"required,notblank"   non-empty, non-whitespace string
//	validate:"positive"            alias for gt=0
//	validate:"mindur=1s"           time.Duration >= 1s
//	validate:"host"                alias for hostname_rfc1123|ip|fqdn
//	validate:"omitempty,port"      empty skips the rest
//	validate:"required_without=Port,omitempty,listen_addr"
//
// Directives come first: required, required_without, omitempty.
func Validate(dst any, opts ...Option) error {
	var s valSettings
	for _, o := range opts {
		if o != nil {
			o(&s)
		}
	}
	eng, err := newTagEngine(s)
	if err != nil {
		return err
	}
	return eng.validate(dst)
}

type ruleEntry struct {
	fn         Func
	msg        string
	evenIfNull bool
}

type tagEngine struct {
	rules       map[string]ruleEntry
	aliases     map[string]string
	structRules map[reflect.Type][]StructRule
}

func newTagEngine(s valSettings) (*tagEngine, error) {
	e := &tagEngine{
		rules:       builtinRules(),
		aliases:     shippedAliases(),
		structRules: map[reflect.Type][]StructRule{},
	}
	for _, r := range shippedRules() {
		e.rules[r.Tag] = ruleEntry{fn: r.Fn, msg: r.Message, evenIfNull: r.EvenIfNull}
	}

	seen := map[string]bool{}
	for _, r := range s.rules {
		tag := strings.TrimSpace(r.Tag)
		if tag == "" || strings.ContainsAny(tag, ",|=") {
			return nil, fmt.Errorf("viperr: invalid rule tag %q", r.Tag)
		}
		if seen[tag] {
			return nil, fmt.Errorf("viperr: duplicate rule %q", tag)
		}
		seen[tag] = true
		if r.Fn == nil {
			return nil, fmt.Errorf("viperr: rule %q has no function", tag)
		}
		if builtinTagNames[tag] && !s.allowBuiltinOverride {
			return nil, fmt.Errorf("viperr: rule %q shadows a built-in tag (use WithBuiltinOverride to allow)", tag)
		}
		e.rules[tag] = ruleEntry{fn: r.Fn, msg: r.Message, evenIfNull: r.EvenIfNull}
	}

	for a, exp := range s.aliases {
		a = strings.TrimSpace(a)
		if a == "" || strings.ContainsAny(a, ",|=") {
			return nil, fmt.Errorf("viperr: invalid alias %q", a)
		}
		if strings.TrimSpace(exp) == "" {
			return nil, fmt.Errorf("viperr: alias %q expands to nothing", a)
		}
		if builtinTagNames[a] && !s.allowBuiltinOverride {
			return nil, fmt.Errorf("viperr: alias %q shadows a built-in tag (use WithBuiltinOverride to allow)", a)
		}
		e.aliases[a] = exp
	}

	for _, sr := range s.structRules {
		if sr.Fn == nil {
			return nil, fmt.Errorf("viperr: struct rule for %T has no function", sr.Sample)
		}
		t := reflect.TypeOf(sr.Sample)
		if t == nil {
			return nil, fmt.Errorf("viperr: struct rule sample is nil")
		}
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t == durationType {
			return nil, fmt.Errorf("viperr: struct rule sample %s is not a struct", t)
		}
		e.structRules[t] = append(e.structRules[t], sr)
	}
	return e, nil
}

func (e *tagEngine) validate(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("viperr: validate: destination must be a non-nil pointer to a struct")
	}
	var errs Errors
	if err := e.walkStruct(v.Elem(), &errs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (e *tagEngine) walkStruct(v reflect.Value, errs *Errors) error {
	t := v.Type()
	if srs, ok := e.structRules[t]; ok {
		target := v
		if v.CanAddr() {
			target = v.Addr()
		}
		for _, sr := range srs {
			if err := sr.Fn(target.Interface()); err != nil {
				return fmt.Errorf("viperr: %s: %w", t.Name(), err)
			}
		}
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := v.Field(i)
		d := fv
		for d.Kind() == reflect.Pointer && !d.IsNil() {
			d = d.Elem()
		}
		if d.Kind() == reflect.Struct && d.Type() != timeType {
			if err := e.walkStruct(d, errs); err != nil {
				return err
			}
		}
		if err := e.checkField(v, fv, sf, errs); err != nil {
			return err
		}
	}
	return nil
}

type term struct{ name, param string }

// parse expands aliases and splits a validate tag into validations.
// Commas separate validations; | separates alternatives inside one.
func (e *tagEngine) parse(tag string, depth int) ([][]term, error) {
	if depth > 8 {
		return nil, fmt.Errorf("alias expansion too deep (cycle?)")
	}
	var out [][]term
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, param, _ := strings.Cut(part, "=")
		if exp, ok := e.aliases[name]; ok {
			if param != "" {
				return nil, fmt.Errorf("alias %q does not take a parameter", name)
			}
			sub, err := e.parse(exp, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
			continue
		}
		var terms []term
		for _, alt := range strings.Split(part, "|") {
			n, p, _ := strings.Cut(strings.TrimSpace(alt), "=")
			if n == "" {
				return nil, fmt.Errorf("empty rule in tag %q", tag)
			}
			terms = append(terms, term{name: n, param: p})
		}
		out = append(out, terms)
	}
	return out, nil
}

type fieldLevel struct {
	field  reflect.Value
	param  string
	key    string
	parent reflect.Value
}

func (f fieldLevel) Field() reflect.Value  { return f.field }
func (f fieldLevel) Param() string         { return f.param }
func (f fieldLevel) Key() string           { return f.key }
func (f fieldLevel) Parent() reflect.Value { return f.parent }

func (e *tagEngine) checkField(parent, fv reflect.Value, sf reflect.StructField, errs *Errors) error {
	tag := strings.TrimSpace(sf.Tag.Get("validate"))
	if tag == "" || tag == "-" {
		return nil
	}
	vals, err := e.parse(tag, 0)
	if err != nil {
		return fmt.Errorf("viperr: %s: %w", sf.Name, err)
	}
	key := fieldKey(sf)

	raw := fv
	nilPtr := false
	for raw.Kind() == reflect.Pointer {
		if raw.IsNil() {
			nilPtr = true
			raw = reflect.Zero(raw.Type().Elem())
			break
		}
		raw = raw.Elem()
	}
	zero := nilPtr || raw.IsZero()

	for _, val := range vals {
		if len(val) == 1 {
			switch val[0].name {
			case "omitempty":
				if zero {
					return nil
				}
				continue
			case "required":
				if zero {
					*errs = append(*errs, e.fieldError(key, "required", "", raw, ""))
					return nil // first failure stops the field
				}
				continue
			case "required_without":
				other := parent.FieldByName(val[0].param)
				if !other.IsValid() {
					return fmt.Errorf("viperr: %s: required_without: no field %q", sf.Name, val[0].param)
				}
				if zero && isZeroValue(other) {
					*errs = append(*errs, e.fieldError(key, "required_without", val[0].param, raw, ""))
					return nil
				}
				continue
			}
		}
		if nilPtr && !e.anyEvenIfNull(val) {
			continue
		}
		ok := false
		for _, t := range val {
			if t.name == "omitempty" && zero {
				ok = true
				break
			}
			rule, exists := e.rules[t.name]
			if !exists {
				return fmt.Errorf("viperr: %s: unknown rule %q", sf.Name, t.name)
			}
			if rule.fn(fieldLevel{field: raw, param: t.param, key: key, parent: parent}) {
				ok = true
				break
			}
		}
		if !ok {
			t := val[0]
			*errs = append(*errs, e.fieldError(key, t.name, t.param, raw, ""))
			return nil // first failure stops the field
		}
	}
	return nil
}

func (e *tagEngine) anyEvenIfNull(val []term) bool {
	for _, t := range val {
		if r, ok := e.rules[t.name]; ok && r.evenIfNull {
			return true
		}
	}
	return false
}

func (e *tagEngine) fieldError(key, tag, param string, raw reflect.Value, msg string) FieldError {
	if msg == "" {
		msg = e.rules[tag].msg
	}
	if msg == "" {
		msg = defaultMessage(tag, raw.Kind() == reflect.String)
	}
	msg = strings.ReplaceAll(msg, "{field}", key)
	msg = strings.ReplaceAll(msg, "{param}", param)
	if !strings.HasPrefix(msg, "viperr: ") {
		msg = "viperr: " + msg
	}
	return FieldError{Key: key, Tag: tag, Param: param, msg: msg}
}

func isZeroValue(v reflect.Value) bool {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	return v.IsZero()
}

// fieldKey names a field in errors: the config key when tagged,
// the Go field name otherwise.
func fieldKey(sf reflect.StructField) string {
	if tag := sf.Tag.Get("config"); tag != "" && tag != "-" {
		if key, _, _ := strings.Cut(tag, ","); strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key)
		}
	}
	return sf.Name
}

var timeType = reflect.TypeOf(time.Time{})
