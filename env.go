package viperr

import (
	"io"

	"github.com/fufikcfg/viperr/internal/dotenv"
)

// ParseEnv reads a flat KEY=VALUE file.
// Blank lines and lines whose first non-space character is '#' are skipped.
// Key and value are trimmed. An empty value is stored and counts as present.
// The parser does not handle quotes, export, inline comments, or multiline values.
// A repeated key keeps the last value. Nothing is written to the process environment.
func ParseEnv(r io.Reader) (Map, error) {
	return dotenv.Parse(r)
}

// ParseEnvFile reads path with ParseEnv.
func ParseEnvFile(path string) (Map, error) {
	return dotenv.ParseFile(path)
}
