// Package xmlconf flattens XML settings files into configuration keys.
// It is internal: only code inside the viperr module may import it.
package xmlconf

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fufikcfg/viperr/internal/source"
)

// Parse flattens an XML document to tag name → trimmed text.
// The key is the element local name, not a path. Nested layout is only a way
// to group tags. An element whose trimmed text is empty is omitted, so an empty
// tag does not count as present. Child elements are not part of the parent text.
// A repeated tag keeps the last non-empty value.
func Parse(r io.Reader) (source.Map, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return nil, fmt.Errorf("viperr: unsupported xml charset %q", charset)
	}
	out := source.Map{}
	var depth int
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("viperr: xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			b.Reset()
		case xml.CharData:
			if depth > 0 {
				b.Write(t)
			}
		case xml.EndElement:
			if depth == 0 {
				continue
			}
			text := strings.TrimSpace(b.String())
			b.Reset()
			depth--
			if text != "" {
				out[t.Name.Local] = text
			}
		}
	}
	return out, nil
}

// ParseFile reads path with Parse.
func ParseFile(path string) (source.Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%w (%s)", err, path)
	}
	return m, nil
}
