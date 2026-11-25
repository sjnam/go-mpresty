//line htmldoc.gw:312
package htmldoc

import (
	"strings"
	"testing"
)

func TestParseShieldsRawBodies(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<metapost>if a<b: draw z1--z2; fi</metapost>`, `if a<b: draw z1--z2; fi`},
		{`<metapost>label("a<b", origin);</metapost>`, `label("a<b", origin);`},
		{`<metapost>x:=a&b; y:=c&amp;d;</metapost>`, `x:=a&b; y:=c&amp;d;`},
		{`<metapost>if n<>2: fi</metapost>`, `if n<>2: fi`},
		{
			"<metapost width=\"200\">\nbeginfig(1)\nendfig;\n</metapost>",
			"\nbeginfig(1)\nendfig;\n",
		},
	}
	for _, c := range cases {

//line htmldoc.gw:336
		doc, err := Parse(strings.NewReader(c.in), "metapost")
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.in, err)
		}
		els := Elements(doc, "metapost")
		if len(els) != 1 {
			t.Fatalf("Parse(%q): got %d metapost elements, want 1", c.in, len(els))
		}
		if got := TextContent(els[0]); got != c.want {
			t.Errorf("TextContent(%q) = %q, want %q", c.in, got, c.want)
		}

//line htmldoc.gw:332
	}
}

//line htmldoc.gw:352
func TestParseLeavesOrdinaryMarkupAlone(t *testing.T) {
	const in = `<html><body><h1>t</h1><metapost src="tree.mp" width="200"></metapost>` +
		`<img src="/source/rgb.mp" width="180"></body></html>`
	doc, err := Parse(strings.NewReader(in), "metapost", "graphviz")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(Elements(doc, "metapost")); n != 1 {
		t.Errorf("metapost elements = %d, want 1", n)
	}
	if n := len(Elements(doc, "img")); n != 1 {
		t.Errorf("img elements = %d, want 1", n)
	}
	if v, _ := Attr(Elements(doc, "metapost")[0], "src"); v != "tree.mp" {
		t.Errorf("src = %q, want tree.mp", v)
	}
	if got := TextContent(Elements(doc, "h1")[0]); got != "t" {
		t.Errorf("h1 text = %q, want t", got)
	}
}

//line htmldoc.gw:377
func TestRenameAndAttrs(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<metapost width="200">code</metapost>`), "metapost")
	if err != nil {
		t.Fatal(err)
	}
	n := Elements(doc, "metapost")[0]
	RemoveChildren(n)
	Rename(n, "img")
	SetAttr(n, "src", "/images/x.svg")
	RemoveAttr(n, "width")
	if !HasAttr(n, "src") || HasAttr(n, "width") {
		t.Fatalf("attrs = %v", n.Attr)
	}
	var b strings.Builder
	if err := Render(&b, doc); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), `<img src="/images/x.svg"/>`; !strings.Contains(got, want) {
		t.Errorf("rendered %q, want it to contain %q", got, want)
	}
}
