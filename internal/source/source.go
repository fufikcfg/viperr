// Package source defines the configuration key source abstraction and the
// built-in sources (process environment, in-memory map). It is internal:
// only code inside the viperr module may import it.
package source

import (
	"os"
	"strings"
)

// Source names used by Load.
// A field tag source selects one of them. An empty tag searches every source.
const (
	Process = "process"
	Env     = "env"
	XML     = "xml"
)

// Source looks up one configuration key.
// ok is false when the source does not contain the key.
// A present empty string is ok true: it is a value, and it blocks lower-priority sources.
type Source interface {
	Lookup(key string) (value string, ok bool)
}

// Map is a source backed by an explicit set of keys.
// A key mapped to "" is present.
type Map map[string]string

// Lookup returns the value stored for key.
func (m Map) Lookup(key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	return v, ok
}

// ProcessEnv reads the environment of the current process.
// A variable set to an empty string is present.
func ProcessEnv() Source { return processEnv{} }

type processEnv struct{}

func (processEnv) Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Name labels a source so a field can select it with source:"xml".
// Load labels the process environment, the .env file, and the XML file
// with Process, Env, and XML.
// An empty name returns src unchanged.
func Name(name string, src Source) Source {
	name = strings.TrimSpace(name)
	if name == "" {
		return src
	}
	return named{name: name, src: src}
}

// NameOf returns the label attached by Name, or "" for an unnamed source.
func NameOf(src Source) string {
	n, ok := src.(interface{ SourceName() string })
	if !ok {
		return ""
	}
	return n.SourceName()
}

type named struct {
	name string
	src  Source
}

func (n named) Lookup(key string) (string, bool) {
	if n.src == nil {
		return "", false
	}
	return n.src.Lookup(key)
}

func (n named) SourceName() string { return n.name }
