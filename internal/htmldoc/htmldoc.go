// Package htmldoc wraps golang.org/x/net/html with the DOM helpers
// mpresty needs, and parses graphics source without mangling it.
//
//line htmldoc.gw:32
//line htmldoc.gw:33
//line htmldoc.gw:34
package htmldoc

import (
	"crypto/rand"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Node is an alias so callers need not import x/net/html directly.
//
//line htmldoc.gw:55
//line htmldoc.gw:56
type Node = html.Node

// Node types re-exported for callers of Walk.
//
//line htmldoc.gw:58
//line htmldoc.gw:59
const (
	TextNode    = html.TextNode
	ElementNode = html.ElementNode

//line htmldoc.gw:62
)

// Parse reads an HTML document, shielding the bodies of rawTags from
// the tokenizer.
//
//line htmldoc.gw:84
//line htmldoc.gw:85
//line htmldoc.gw:86
func Parse(r io.Reader, rawTags ...string) (*Node, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	src, bodies := string(b), make(map[string]string)
	if len(rawTags) > 0 && strings.Contains(src, "<") {

//line htmldoc.gw:123
		prefix := "mprestyraw" + rand.Text()
		for _, tag := range rawTags {
			q := regexp.QuoteMeta(tag)
			re := regexp.MustCompile(`(?is)(<` + q + `\b[^<>]*>)(.*?)(</` + q + `\s*>)`)
			src = re.ReplaceAllStringFunc(src, func(m string) string {
				g := re.FindStringSubmatch(m)
				if g[2] == "" {
					return m
				}
				token := prefix + strconv.Itoa(len(bodies))
				bodies[token] = g[2]
				return g[1] + token + g[3]
			})
		}

//line htmldoc.gw:94
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}
	if len(bodies) > 0 {

//line htmldoc.gw:143
		Walk(doc, func(n *Node) {
			if n.Type == TextNode {
				if body, ok := bodies[strings.TrimSpace(n.Data)]; ok {
					n.Data = body
				}
			}
		})

//line htmldoc.gw:101
	}
	return doc, nil
}

// Render serializes the tree back to HTML.
//
//line htmldoc.gw:156
//line htmldoc.gw:157
func Render(w io.Writer, n *Node) error { return html.Render(w, n) }

// Walk calls fn for n and every descendant, in document order.
//
//line htmldoc.gw:165
//line htmldoc.gw:166
func Walk(n *Node, fn func(*Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}

// Elements collects every element with the given tag name. The result is
// a snapshot, so callers may rewrite the nodes it holds while iterating.
//
//line htmldoc.gw:178
//line htmldoc.gw:179
//line htmldoc.gw:180
func Elements(n *Node, name string) []*Node {
	var out []*Node
	Walk(n, func(n *Node) {
		if n.Type == ElementNode && n.Data == name {
			out = append(out, n)
		}
	})
	return out
}

// TextContent concatenates the text of n and its descendants.
//
//line htmldoc.gw:194
//line htmldoc.gw:195
func TextContent(n *Node) string {
	var b strings.Builder
	Walk(n, func(n *Node) {
		if n.Type == TextNode {
			b.WriteString(n.Data)
		}
	})
	return b.String()
}

// SetTextContent replaces the children of n with a single text node.
//
//line htmldoc.gw:211
//line htmldoc.gw:212
func SetTextContent(n *Node, s string) {
	RemoveChildren(n)
	n.AppendChild(&Node{Type: TextNode, Data: s})
}

// RemoveChildren detaches every child of n.
//
//line htmldoc.gw:220
//line htmldoc.gw:221
func RemoveChildren(n *Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		c = next
	}
}

// CreateElement builds a detached element.
//
//line htmldoc.gw:241
//line htmldoc.gw:242
func CreateElement(name string) *Node {
	return &Node{Type: ElementNode, Data: name, DataAtom: atom.Lookup([]byte(name))}
}

// Rename turns n into an element of another kind, keeping its attributes
// and children.
//
//line htmldoc.gw:246
//line htmldoc.gw:247
//line htmldoc.gw:248
func Rename(n *Node, name string) {
	n.Data, n.DataAtom = name, atom.Lookup([]byte(name))
}

// Attr returns the value of an attribute and whether it was present.
//
//line htmldoc.gw:256
//line htmldoc.gw:257
func Attr(n *Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// AttrOr returns an attribute value, falling back to def when absent.
//
//line htmldoc.gw:266
//line htmldoc.gw:267
func AttrOr(n *Node, key, def string) string {
	if v, ok := Attr(n, key); ok {
		return v
	}
	return def
}

// HasAttr reports whether the attribute is present.
//
//line htmldoc.gw:274
//line htmldoc.gw:275
func HasAttr(n *Node, key string) bool {
	_, ok := Attr(n, key)
	return ok
}

// SetAttr sets or replaces an attribute.
//
//line htmldoc.gw:283
//line htmldoc.gw:284
func SetAttr(n *Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// RemoveAttr drops an attribute if present.
//
//line htmldoc.gw:294
//line htmldoc.gw:295
func RemoveAttr(n *Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}
