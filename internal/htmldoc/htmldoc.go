// Package htmldoc wraps golang.org/x/net/html with the small DOM helpers
// mpresty relied on gumbo for: textContent, attribute access, and a
// serializer. It also solves a problem gumbo never did — see Parse.
package htmldoc

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Node is an alias so callers need not import x/net/html directly.
type Node = html.Node

// Parse reads an HTML document, shielding the bodies of rawTags from the
// tokenizer.
//
// The HTML5 tokenizer treats "<" followed by a letter as a tag open, so a
// MetaPost line such as `if a<b:` loses everything from the "<" onward. The
// original mpresty inherited that from gumbo. Here each rawTag body is lifted
// out before parsing, replaced by an opaque alphanumeric token, and restored
// into the tree afterwards, so graphics source survives byte for byte.
func Parse(r io.Reader, rawTags ...string) (*Node, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	src, bodies := shield(string(b), rawTags)
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}
	if len(bodies) > 0 {
		restore(doc, bodies)
	}
	return doc, nil
}

// ParseFragment parses a document that a handler built itself, such as the
// single-element document /preview.gxn assembles from its form.
func ParseFragment(s string, rawTags ...string) (*Node, error) {
	return Parse(strings.NewReader(s), rawTags...)
}

// Render serializes the tree back to HTML.
func Render(w io.Writer, n *Node) error { return html.Render(w, n) }

// String serializes the tree to a string, mirroring gumbo's doc:serialize().
func String(n *Node) string {
	var b strings.Builder
	if err := Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}
