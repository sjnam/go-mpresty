// Package render turns graphics source embedded in an HTML document into SVG
// images, the job mpresty's lib/mpresty/*.lua did.
//
// The Lua original kept per-request state (cmd, doc, use_cache) on the module
// singleton. That was safe only because ngx.thread light threads are
// cooperative and single-threaded; goroutines are not. Here a Renderer holds
// immutable configuration only, and everything that varies per node travels as
// arguments.
package render

import (
	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// UpdateNodeFunc rewrites one graphics node once its image is ready. It is the
// Go form of mpresty's fn_update_node: doc is the document, node the element
// that carried the source, uri the URI of the rendered SVG, and content the
// graphics source itself.
type UpdateNodeFunc func(doc, node *htmldoc.Node, uri, content string)

// Updaters selects an UpdateNodeFunc per tag, matching mpresty's convention
// that fn_update_node may be a single function or a table keyed by tag name.
type Updaters struct {
	Default UpdateNodeFunc
	ByTag   map[string]UpdateNodeFunc
}

// For returns the updater for a tag, falling back to Default and then to
// DefaultUpdateNode.
func (u Updaters) For(tag string) UpdateNodeFunc {
	if fn, ok := u.ByTag[tag]; ok && fn != nil {
		return fn
	}
	if u.Default != nil {
		return u.Default
	}
	return DefaultUpdateNode
}

// DefaultUpdateNode replaces the node with an <img> pointing at the SVG,
// defaulting the width to 300 as base.lua did.
func DefaultUpdateNode(doc, node *htmldoc.Node, uri, content string) {
	htmldoc.Rename(node, "img")
	htmldoc.SetAttr(node, "src", uri)
	if !htmldoc.HasAttr(node, "width") {
		htmldoc.SetAttr(node, "width", "300")
	}
}

// ErrorUpdateNode shows the failure in red where the image would have been.
func ErrorUpdateNode(doc, node *htmldoc.Node, uri, content string) {
	htmldoc.Rename(node, "pre")
	htmldoc.SetTextContent(node, content)
	htmldoc.SetAttr(node, "style", "color:red")
	htmldoc.RemoveAttr(node, "width")
}
