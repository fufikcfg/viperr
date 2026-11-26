// Package bind binds configuration sources into structs and records where
// every value came from. It is internal: only code inside the viperr module
// may import it.
package bind

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fufikcfg/viperr/internal/resolve"
	"github.com/fufikcfg/viperr/internal/source"
	"github.com/fufikcfg/viperr/internal/tag"
	"github.com/fufikcfg/viperr/internal/validate"
)

// Validator is implemented by a destination that has rules beyond the config tag.
// Decode calls it after every tagged field has been bound, normalized, and
// validated. Prefer validate tags; keep Validate for rules tags cannot express.
type Validator interface {
	Validate() error
}

// Normalizer is implemented by a destination or a nested section that fixes
// up its own values — trim, lowercase, derive — before tag validation runs.
// Sections are normalized before the root.
type Normalizer interface {
	Normalize() error
}

// Options controls Decode.
type Options struct {
	// AppDir is the base for fields tagged path.
	// An empty AppDir leaves a relative path relative.
	AppDir string
	// Validation customizes tag validation. Nil uses the shipped rules only.
	Validation []validate.Option
}

// SourceDefault is the reported origin of a value taken from a tag default.
const SourceDefault = "default"

// SourceUnset is the reported origin of a key no source contained.
const SourceUnset = "-"

// Entry describes how one tagged field was bound.
type Entry struct {
	Field  string // Go field name, dotted for nested sections (DB.Host)
	Key    string // config key (DB_HOST)
	Source string // winning source name, "default", or "-"
	Value  string // bound value; secret values are masked as "***"
	Secret bool   // the value is masked in this report
}

// Report is the provenance of every tagged field, in struct order.
// Secret values are already masked, so a Report is safe to print or log.
type Report []Entry

// String renders the report as an aligned table:
// key, origin source, Go field, value (secrets masked).
func (r Report) String() string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tSOURCE\tFIELD\tVALUE")
	for _, e := range r {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Key, e.Source, e.Field, e.Value)
	}
	_ = w.Flush()
	return b.String()
}

// Decode binds sources into dst. See DecodeReport.
func Decode(dst any, opt Options, sources ...source.Source) error {
	_, err := DecodeReport(dst, opt, sources...)
	return err
}

// DecodeReport binds sources into dst, which must be a pointer to a struct,
// and returns the provenance of every tagged field.
// Sources are highest priority first. The first source that contains a key wins.
// A field with source:"xml" (or env, process, or any Name) reads only that source.
// A field default is used only when the selected sources do not contain the key.
// An absent key without a default leaves the zero value; validate tags decide
// whether that is acceptable. Presence is not the same as a non-zero Go value:
// "0" is a value and beats a default.
//
// After binding, DecodeReport normalizes (see Normalizer), checks validate tags
// (see validate.Validate), then calls the Validator hook when implemented.
func DecodeReport(dst any, opt Options, sources ...source.Source) (Report, error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, fmt.Errorf("viperr: destination must be a non-nil pointer to a struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("viperr: destination must be a non-nil pointer to a struct")
	}
	var report Report
	if err := bindStruct(v, "", opt, sources, &report); err != nil {
		return nil, err
	}
	if err := normalizeValue(v); err != nil {
		return nil, fmt.Errorf("viperr: normalize: %w", err)
	}
	if err := validate.Validate(dst, opt.Validation...); err != nil {
		return nil, err
	}
	if validator, ok := dst.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return nil, fmt.Errorf("viperr: validate: %w", err)
		}
	}
	return report, nil
}

// normalizeValue calls Normalize on nested sections first, then on v itself,
// so the root sees sections already fixed up.
func normalizeValue(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		d := v.Field(i)
		for d.Kind() == reflect.Pointer {
			if d.IsNil() {
				break
			}
			d = d.Elem()
		}
		if d.Kind() == reflect.Struct && d.Type() != reflect.TypeOf(time.Time{}) {
			if err := normalizeValue(d); err != nil {
				return err
			}
		}
	}
	if v.CanAddr() {
		if n, ok := v.Addr().Interface().(Normalizer); ok {
			return n.Normalize()
		}
	}
	return nil
}

func bindStruct(v reflect.Value, prefix string, opt Options, sources []source.Source, report *Report) error {
	var errs []error
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		tagText := sf.Tag.Get("config")
		if tagText == "-" {
			continue
		}
		fv := v.Field(i)
		fieldName := sf.Name
		if prefix != "" {
			fieldName = prefix + "." + sf.Name
		}
		if tagText == "" {
			if err := bindNested(fv, fieldName, opt, sources, report); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		spec, err := tag.Parse(tagText)
		if err != nil {
			errs = append(errs, fmt.Errorf("viperr: %s: %w", sf.Name, err))
			continue
		}
		spec.Source = strings.TrimSpace(sf.Tag.Get("source"))
		if err := bindField(fv, spec, fieldName, opt, sources, report); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func bindNested(fv reflect.Value, fieldName string, opt Options, sources []source.Source, report *Report) error {
	base := fv.Type()
	pointer := false
	if base.Kind() == reflect.Pointer {
		if base.Elem().Kind() != reflect.Struct {
			return nil
		}
		pointer = true
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct || base == reflect.TypeOf(time.Time{}) {
		return nil
	}
	if pointer {
		if fv.IsNil() {
			fv.Set(reflect.New(base))
		}
		fv = fv.Elem()
	}
	if !fv.CanSet() {
		return nil
	}
	return bindStruct(fv, fieldName, opt, sources, report)
}

func bindField(fv reflect.Value, spec tag.Spec, fieldName string, opt Options, sources []source.Source, report *Report) error {
	target := fv
	if target.Kind() == reflect.Pointer {
		elem := target.Type().Elem()
		if elem.Kind() == reflect.Struct && elem != reflect.TypeOf(time.Time{}) {
			return fmt.Errorf("viperr: %s: struct field cannot use a config key", spec.Key)
		}
	}
	kind := fieldKind(target)
	if spec.Path && kind != kindString {
		return fmt.Errorf("viperr: %s: path is only valid on a string", spec.Key)
	}
	if kind == kindOther {
		return fmt.Errorf("viperr: %s: unsupported type %s", spec.Key, target.Type())
	}

	raw, origin, found, err := lookup(spec.Key, spec.Source, sources)
	if err != nil {
		return fmt.Errorf("viperr: %s: %w", spec.Key, err)
	}
	switch {
	case found:
	case spec.HasDef:
		raw = spec.Def
		origin = SourceDefault
	default:
		record(report, fieldName, spec, "", SourceUnset)
		return nil // absent without a default: zero value, validate tags decide
	}

	if spec.HasOneof && !oneOf(raw, spec.Oneof) {
		return fmt.Errorf("viperr: %s: must be one of %s", spec.Key, strings.Join(spec.Oneof, ", "))
	}
	if spec.Path {
		if spec.NoAbs {
			raw = resolve.ResolveStrict(raw, opt.AppDir)
		} else {
			raw = resolve.Resolve(raw, opt.AppDir)
		}
	}
	if err := assign(target, raw); err != nil {
		return fmt.Errorf("viperr: %s: %w", spec.Key, err)
	}
	record(report, fieldName, spec, raw, origin)
	return nil
}

func record(report *Report, fieldName string, spec tag.Spec, value, origin string) {
	if report == nil {
		return
	}
	secret := spec.Secret || looksSecret(spec.Key)
	if secret && value != "" {
		value = "***"
	}
	*report = append(*report, Entry{
		Field:  fieldName,
		Key:    spec.Key,
		Source: origin,
		Value:  value,
		Secret: secret,
	})
}

// looksSecret reports whether a key name smells like a credential, so reports
// mask it even without an explicit secret option.
func looksSecret(key string) bool {
	u := strings.ToUpper(key)
	for _, marker := range []string{"SECRET", "PASSWORD", "PASSWD", "TOKEN", "CREDENTIAL", "PRIVATE"} {
		if strings.Contains(u, marker) {
			return true
		}
	}
	return u == "KEY" || strings.HasSuffix(u, "_KEY")
}

func lookup(key, want string, sources []source.Source) (string, string, bool, error) {
	if want != "" && !sourceLoaded(sources, want) {
		return "", "", false, fmt.Errorf("source %q is not loaded", want)
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		if want != "" && source.NameOf(src) != want {
			continue
		}
		if value, ok := src.Lookup(key); ok {
			origin := source.NameOf(src)
			if origin == "" {
				origin = SourceUnset
			}
			return value, origin, true, nil
		}
	}
	return "", "", false, nil
}

func sourceLoaded(sources []source.Source, name string) bool {
	for _, src := range sources {
		if src != nil && source.NameOf(src) == name {
			return true
		}
	}
	return false
}

func oneOf(raw string, allowed []string) bool {
	for _, item := range allowed {
		if raw == item {
			return true
		}
	}
	return false
}

type scalarKind int

const (
	kindOther scalarKind = iota
	kindString
	kindBool
	kindInt
	kindUint
	kindDuration
)

func fieldKind(fv reflect.Value) scalarKind {
	t := fv.Type()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == reflect.TypeOf(time.Duration(0)):
		return kindDuration
	case t.Kind() == reflect.String:
		return kindString
	case t.Kind() == reflect.Bool:
		return kindBool
	case isInt(t.Kind()):
		return kindInt
	case isUint(t.Kind()):
		return kindUint
	default:
		return kindOther
	}
}

func assign(fv reflect.Value, raw string) error {
	t := fv.Type()
	if t.Kind() == reflect.Pointer {
		elem := reflect.New(t.Elem())
		if err := assign(elem.Elem(), raw); err != nil {
			return err
		}
		fv.Set(elem)
		return nil
	}
	if !fv.CanSet() {
		return fmt.Errorf("cannot set field")
	}
	switch {
	case t == reflect.TypeOf(time.Duration(0)):
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("invalid duration %q", raw)
		}
		fv.SetInt(int64(d))
		return nil
	case t.Kind() == reflect.String:
		fv.SetString(raw)
		return nil
	case t.Kind() == reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid bool %q", raw)
		}
		fv.SetBool(b)
		return nil
	case isInt(t.Kind()):
		n, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return fmt.Errorf("invalid int %q", raw)
		}
		fv.SetInt(n)
		return nil
	case isUint(t.Kind()):
		n, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return fmt.Errorf("invalid uint %q", raw)
		}
		fv.SetUint(n)
		return nil
	default:
		return fmt.Errorf("unsupported type %s", t)
	}
}

func isInt(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

func isUint(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}
