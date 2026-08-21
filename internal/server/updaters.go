package server

import (
	"github.com/sjnam/go-mpresty/internal/htmldoc"
	"github.com/sjnam/go-mpresty/internal/render"
)

// The Lua version customised pages by dropping a .lua file next to the config;
// in Go the same customisation is an UpdateNodeFunc registered on a route.

// ExemplesUpdateNode shows the source above the image, as exemples.lua did.
func ExemplesUpdateNode(doc, node *htmldoc.Node, uri, content string) {
	htmldoc.Rename(node, "p")
	htmldoc.RemoveAttr(node, "width")

	pre := htmldoc.CreateElement("pre")
	htmldoc.SetTextContent(pre, content)
	node.AppendChild(pre)

	img := htmldoc.CreateElement("img")
	htmldoc.SetAttr(img, "src", uri)
	htmldoc.SetAttr(img, "width", "200")
	node.AppendChild(img)

	node.AppendChild(htmldoc.CreateElement("hr"))
}

// UpdateNodeByTag is the per-tag table /updatenode.html uses.
var UpdateNodeByTag = map[string]render.UpdateNodeFunc{
	"metapost": ExemplesUpdateNode,
}
