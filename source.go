package viperr

import (
	"github.com/fufikcfg/viperr/internal/source"
)

// Source names used by Load.
// A field tag source selects one of them. An empty tag searches every source.
const (
	SourceProcess = source.Process
	SourceEnv     = source.Env
	SourceXML     = source.XML
)

// Source looks up one configuration key.
// ok is false when the source does not contain the key.
// A present empty string is ok true: it is a value, and it blocks lower-priority sources.
type Source = source.Source

// Map is a source backed by an explicit set of keys.
// A key mapped to "" is present.
type Map = source.Map

// ProcessEnv reads the environment of the current process.
// A variable set to an empty string is present.
func ProcessEnv() Source { return source.ProcessEnv() }

// Name labels a source so a field can select it with source:"xml".
// Load labels the process environment, the .env file, and the XML file
// with SourceProcess, SourceEnv, and SourceXML.
// An empty name returns src unchanged.
func Name(name string, src Source) Source { return source.Name(name, src) }
