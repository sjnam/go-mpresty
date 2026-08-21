package htmldoc

import "testing"

// The tokenizer swallows `a<b` unless the body is shielded first. These are the
// exact cases that broke the original gumbo-based mpresty.
func TestParseShieldsRawBodies(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<metapost>if a<b: draw z1--z2; fi</metapost>`, `if a<b: draw z1--z2; fi`},
		{`<metapost>label("a<b", origin);</metapost>`, `label("a<b", origin);`},
		{`<metapost>x:=a&b; y:=c&amp;d;</metapost>`, `x:=a&b; y:=c&amp;d;`},
		{`<metapost>if n<>2: fi</metapost>`, `if n<>2: fi`},
		{"<metapost width=\"200\">\nbeginfig(1)\nendfig;\n</metapost>", "\nbeginfig(1)\nendfig;\n"},
	}
	for _, c := range cases {
		doc, err := Parse(stringReader(c.in), "metapost")
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
	}
}

// A shielded tag with no body, and ordinary markup around it, must survive.
func TestParseLeavesOrdinaryMarkupAlone(t *testing.T) {
	const in = `<html><body><h1>t</h1><metapost src="tree.mp" width="200"></metapost>` +
		`<img src="/source/rgb.mp" width="180"></body></html>`
	doc, err := Parse(stringReader(in), "metapost", "graphviz")
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

func TestRenameAndAttrs(t *testing.T) {
	doc, err := Parse(stringReader(`<metapost width="200">code</metapost>`), "metapost")
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
	if got, want := String(doc), `<img src="/images/x.svg"/>`; !contains(got, want) {
		t.Errorf("String(doc) = %q, want it to contain %q", got, want)
	}
}
