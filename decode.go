package viperr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Validator is implemented by a destination that has rules beyond the config tag.
// Decode calls it after every tagged field has been bound.
type Validator interface {
	Validate() error
}

// Options controls Decode.
type Options struct {
	// AppDir is the base for fields tagged path.
	// An empty AppDir leaves a relative path relative.
	AppDir string
}

// Decode binds sources into dst, which must be a pointer to a struct.
// Sources are highest priority first. The first source that contains a key wins.
// A field with source:"xml" (or env, process, or any Name) reads only that source.
// A field default is used only when the selected sources do not contain the key.
// required fails when the key is absent and the field has no default.
// Presence is not the same as a non-zero Go value: "0" satisfies required and beats a default.
func Decode(dst any, opt Options, sources ...Source) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("viperr: destination must be a non-nil pointer to a struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("viperr: destination must be a non-nil pointer to a struct")
	}
	if err := bindStruct(v, opt, sources); err != nil {
		return err
	}
	if validator, ok := dst.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("viperr: validate: %w", err)
		}
	}
	return nil
}

// Load reads configuration for dir into dst.
// An empty dir uses ResolveAppDir. The process environment wins over dir/.env,
// which wins over settings.xml or settings/settings.xml. A missing file is skipped.
// Path fields are resolved against dir.
func Load(dst any, dir string) error {
	if strings.TrimSpace(dir) == "" {
		resolved, err := ResolveAppDir()
		if err != nil {
			return err
		}
		dir = resolved
	}
	sources := []Source{Name(SourceProcess, ProcessEnv())}

	env, err := ParseEnvFile(filepath.Join(dir, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err != nil {
		env = Map{}
	}
	sources = append(sources, Name(SourceEnv, env))

	settings, err := settingsFile(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var xmlMap Map
	if err == nil {
		xmlMap, err = ParseXMLFile(settings)
		if err != nil {
			return err
		}
	} else {
		xmlMap = Map{}
	}
	sources = append(sources, Name(SourceXML, xmlMap))
	return Decode(dst, Options{AppDir: dir}, sources...)
}

func bindStruct(v reflect.Value, opt Options, sources []Source) error {
	var errs []error
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("config")
		if tag == "-" {
			continue
		}
		fv := v.Field(i)
		if tag == "" {
			if err := bindNested(fv, opt, sources); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		spec, err := parseTag(tag)
		if err != nil {
			errs = append(errs, fmt.Errorf("viperr: %s: %w", sf.Name, err))
			continue
		}
		spec.source = strings.TrimSpace(sf.Tag.Get("source"))
		if err := bindField(fv, spec, opt, sources); err != nil {
			errs = append(errs, err)
		}
	}
	return errorsJoin(errs)
}

func bindNested(fv reflect.Value, opt Options, sources []Source) error {
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
	return bindStruct(fv, opt, sources)
}

func bindField(fv reflect.Value, spec fieldSpec, opt Options, sources []Source) error {
	target := fv
	if target.Kind() == reflect.Pointer {
		elem := target.Type().Elem()
		if elem.Kind() == reflect.Struct && elem != reflect.TypeOf(time.Time{}) {
			return fmt.Errorf("viperr: %s: struct field cannot use a config key", spec.key)
		}
	}
	kind := fieldKind(target)
	if spec.path && kind != kindString {
		return fmt.Errorf("viperr: %s: path is only valid on a string", spec.key)
	}
	if kind == kindOther {
		return fmt.Errorf("viperr: %s: unsupported type %s", spec.key, target.Type())
	}

	raw, found, err := lookup(spec.key, spec.source, sources)
	if err != nil {
		return fmt.Errorf("viperr: %s: %w", spec.key, err)
	}
	switch {
	case found:
	case spec.hasDef:
		raw = spec.def
		found = true
	case spec.required:
		return fmt.Errorf("viperr: %s: required", spec.key)
	default:
		return nil
	}

	if spec.hasOneof && !oneOf(raw, spec.oneof) {
		return fmt.Errorf("viperr: %s: must be one of %s", spec.key, strings.Join(spec.oneof, ", "))
	}
	if spec.path {
		raw = Resolve(raw, opt.AppDir)
	}
	if err := assign(target, raw); err != nil {
		return fmt.Errorf("viperr: %s: %w", spec.key, err)
	}
	return nil
}

func lookup(key, want string, sources []Source) (string, bool, error) {
	if want != "" && !sourceLoaded(sources, want) {
		return "", false, fmt.Errorf("source %q is not loaded", want)
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		if want != "" && sourceName(src) != want {
			continue
		}
		if value, ok := src.Lookup(key); ok {
			return value, true, nil
		}
	}
	return "", false, nil
}

func sourceLoaded(sources []Source, name string) bool {
	for _, src := range sources {
		if src != nil && sourceName(src) == name {
			return true
		}
	}
	return false
}

func sourceName(src Source) string {
	named, ok := src.(interface{ SourceName() string })
	if !ok {
		return ""
	}
	return named.SourceName()
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

func errorsJoin(errs []error) error {
	return errors.Join(errs...)
}
