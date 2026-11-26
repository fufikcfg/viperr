package viperr

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fufikcfg/viperr/internal/bind"
	"github.com/fufikcfg/viperr/internal/resolve"
)

// Validator is implemented by a destination that has rules beyond the config tag.
// Decode calls it after every tagged field has been bound, normalized, and
// validated. Prefer validate tags; keep Validate for rules tags cannot express.
type Validator = bind.Validator

// Normalizer is implemented by a destination or a nested section that fixes
// up its own values — trim, lowercase, derive — before tag validation runs.
// Sections are normalized before the root.
type Normalizer = bind.Normalizer

// Options controls Decode.
type Options = bind.Options

// Entry describes how one tagged field was bound.
type Entry = bind.Entry

// Report is the provenance of every tagged field, in struct order.
// Secret values are already masked, so a Report is safe to print or log.
type Report = bind.Report

// Decode binds sources into dst, which must be a pointer to a struct.
// Sources are highest priority first. The first source that contains a key wins.
// A field with source:"xml" (or env, process, or any Name) reads only that source.
// A field default is used only when the selected sources do not contain the key.
// An absent key without a default leaves the zero value; validate tags decide
// whether that is acceptable. Presence is not the same as a non-zero Go value:
// "0" is a value and beats a default.
//
// After binding, Decode normalizes (see Normalizer), checks validate tags
// (see Validate), then calls the Validator hook when implemented.
func Decode(dst any, opt Options, sources ...Source) error {
	return bind.Decode(dst, opt, sources...)
}

// DecodeReport is Decode plus a Report of where every tagged field's value
// came from (source name, "default", or "-"). Secret values are masked.
func DecodeReport(dst any, opt Options, sources ...Source) (Report, error) {
	return bind.DecodeReport(dst, opt, sources...)
}

// Load reads configuration for dir into dst.
// An empty dir uses ResolveAppDir. The process environment wins over dir/.env,
// which wins over settings.xml or settings/settings.xml. A missing file is skipped.
// Path fields are resolved against dir.
// opts customize tag validation, see Validate.
func Load(dst any, dir string, opts ...Option) error {
	_, err := load(dst, dir, opts...)
	return err
}

// LoadReport is Load plus a Report of where every tagged field's value came
// from. Handy for a "config print" CLI command: load, then print the table.
func LoadReport(dst any, dir string, opts ...Option) (Report, error) {
	return load(dst, dir, opts...)
}

func load(dst any, dir string, opts ...Option) (Report, error) {
	if strings.TrimSpace(dir) == "" {
		resolved, err := ResolveAppDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	sources, err := chain(dir)
	if err != nil {
		return nil, err
	}
	return DecodeReport(dst, Options{AppDir: dir, Validation: opts}, sources...)
}

// chain builds the standard source chain for dir:
// process environment > dir/.env > dir settings.xml (or settings/settings.xml).
func chain(dir string) ([]Source, error) {
	sources := []Source{Name(SourceProcess, ProcessEnv())}

	env, err := ParseEnvFile(filepath.Join(dir, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err != nil {
		env = Map{}
	}
	sources = append(sources, Name(SourceEnv, env))

	settings, err := resolve.SettingsFile(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var xmlMap Map
	if err == nil {
		xmlMap, err = ParseXMLFile(settings)
		if err != nil {
			return nil, err
		}
	} else {
		xmlMap = Map{}
	}
	sources = append(sources, Name(SourceXML, xmlMap))
	return sources, nil
}

// Get reads one key along the standard source chain (process environment,
// then .env, then settings.xml) of the directory returned by ResolveAppDir.
// It needs no struct and no tag — useful for reading a lone secret such as
// GATEWAY_API_KEY without declaring a config section.
//
// ok reports whether the key was found in any source. When the key is absent
// and a default is given, Get returns the default and ok is false.
func Get(key string, def ...string) (string, bool) {
	dir, err := ResolveAppDir()
	if err != nil {
		// No app directory: the process environment is still a valid source.
		if v, ok := ProcessEnv().Lookup(key); ok {
			return v, true
		}
		if len(def) > 0 {
			return def[0], false
		}
		return "", false
	}
	return GetFrom(dir, key, def...)
}

// GetFrom is Get against an explicit directory instead of ResolveAppDir.
func GetFrom(dir, key string, def ...string) (string, bool) {
	sources, err := chain(dir)
	if err != nil {
		if len(def) > 0 {
			return def[0], false
		}
		return "", false
	}
	for _, src := range sources {
		if v, ok := src.Lookup(key); ok {
			return v, true
		}
	}
	if len(def) > 0 {
		return def[0], false
	}
	return "", false
}

// Explain loads dst from dir (ResolveAppDir when dir is empty or omitted) and
// writes a table of every tagged field: key, origin source, Go field, and
// value with secrets masked. It is the building block for a "config print"
// CLI command and for debugging which source won.
func Explain(w io.Writer, dst any, dir ...string) error {
	var base string
	if len(dir) > 0 {
		base = dir[0]
	}
	report, err := LoadReport(dst, base)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, report.String())
	return err
}
