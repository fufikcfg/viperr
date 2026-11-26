package viperr

import (
	"io"

	"github.com/fufikcfg/viperr/internal/xmlconf"
)

// ParseXML flattens an XML document to tag name → trimmed text.
// The key is the element local name, not a path. Nested layout is only a way
// to group tags. An element whose trimmed text is empty is omitted, so an empty
// tag does not count as present. Child elements are not part of the parent text.
// A repeated tag keeps the last non-empty value.
func ParseXML(r io.Reader) (Map, error) {
	return xmlconf.Parse(r)
}

// ParseXMLFile reads path with ParseXML.
func ParseXMLFile(path string) (Map, error) {
	return xmlconf.ParseFile(path)
}
