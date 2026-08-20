package htmldoc

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Node types re-exported for callers of Walk.
const (
	TextNode    = html.TextNode
	ElementNode = html.ElementNode
)

// Walk calls fn for n and every descendant, in document order.
func Walk(n *Node, fn func(*Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}

// Elements collects every element with the given tag name, the equivalent of
// gumbo's doc:getElementsByTagName. The result is a snapshot, so callers may
// rewrite the nodes it holds while iterating.
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
func SetTextContent(n *Node, s string) {
	RemoveChildren(n)
	n.AppendChild(&Node{Type: TextNode, Data: s})
}

// RemoveChildren detaches every child of n.
func RemoveChildren(n *Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		c = next
	}
}

// CreateElement builds a detached element, as doc:createElement does.
func CreateElement(name string) *Node {
	return &Node{Type: ElementNode, Data: name, DataAtom: atom.Lookup([]byte(name))}
}

// Rename turns n into an element of another kind, keeping its attributes and
// children — the trick every fn_update_node opens with (node.localName = "img").
func Rename(n *Node, name string) {
	n.Data, n.DataAtom = name, atom.Lookup([]byte(name))
}

// Attr returns the value of an attribute and whether it was present.
func Attr(n *Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// AttrOr returns an attribute value, falling back to def when absent.
func AttrOr(n *Node, key, def string) string {
	if v, ok := Attr(n, key); ok {
		return v
	}
	return def
}

// HasAttr reports whether the attribute is present.
func HasAttr(n *Node, key string) bool {
	_, ok := Attr(n, key)
	return ok
}

// SetAttr sets or replaces an attribute.
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
func RemoveAttr(n *Node, key string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return
		}
	}
}
