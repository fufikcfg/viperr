package viperr

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseEnv reads a flat KEY=VALUE file.
// Blank lines and lines whose first non-space character is '#' are skipped.
// Key and value are trimmed. An empty value is stored and counts as present.
// The parser does not handle quotes, export, inline comments, or multiline values.
// A repeated key keeps the last value. Nothing is written to the process environment.
func ParseEnv(r io.Reader) (Map, error) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	out := Map{}
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("viperr: .env line %d: expected KEY=VALUE", lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("viperr: .env line %d: empty key", lineNo)
		}
		out[key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("viperr: .env: %w", err)
	}
	return out, nil
}

// ParseEnvFile reads path with ParseEnv.
func ParseEnvFile(path string) (Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	m, err := ParseEnv(f)
	if err != nil {
		return nil, fmt.Errorf("%w (%s)", err, path)
	}
	return m, nil
}
