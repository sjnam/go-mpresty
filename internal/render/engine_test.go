package render

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// The engines are real programs, so these run only where they are installed.
func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not installed", name)
	}
}

func TestMetaPostEndToEnd(t *testing.T) {
	requireCommand(t, "mpost")
	root := t.TempDir()
	p, err := New(Options{Root: root, Renderers: []*Renderer{MetaPost}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	// `a<b` and `&` are the characters the gumbo-based original lost.
	const src = `<metapost width="100">
beginfig(1)
  n:=3;
  if n<5: draw fullcircle scaled 2cm; fi
  label("n = "&decimal n, origin);
endfig;
</metapost>`

	doc, err := htmldoc.Parse(strings.NewReader(src), p.RawTags()...)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Process(context.Background(), doc, Request{BaseURI: "/p.html"}); err != nil {
		t.Fatal(err)
	}
	if pre := htmldoc.Elements(doc, "pre"); len(pre) > 0 {
		t.Fatalf("mpost failed: %s", htmldoc.TextContent(pre[0]))
	}
	img := htmldoc.Elements(doc, "img")
	if len(img) != 1 {
		t.Fatalf("got %d img elements, want 1", len(img))
	}
	src2, _ := htmldoc.Attr(img[0], "src")
	if !strings.HasPrefix(src2, "/images/mpost") || !strings.HasSuffix(src2, ".svg") {
		t.Errorf("src = %q", src2)
	}
	if got := readFile(t, p.svgPath(strings.TrimSuffix(strings.TrimPrefix(src2, "/images/"), ".svg"))); !strings.Contains(got, "<svg") {
		t.Errorf("output is not an SVG: %.60q", got)
	}
}

func TestGraphvizEndToEnd(t *testing.T) {
	requireCommand(t, "dot")
	root := t.TempDir()
	p, err := New(Options{Root: root, Renderers: []*Renderer{Graphviz}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := htmldoc.Parse(strings.NewReader(`<graphviz cmd="neato">digraph{a->b->c}</graphviz>`), p.RawTags()...)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Process(context.Background(), doc, Request{BaseURI: "/p.html"}); err != nil {
		t.Fatal(err)
	}
	if pre := htmldoc.Elements(doc, "pre"); len(pre) > 0 {
		t.Fatalf("dot failed: %s", htmldoc.TextContent(pre[0]))
	}
	if s, _ := htmldoc.Attr(htmldoc.Elements(doc, "img")[0], "src"); !strings.HasPrefix(s, "/images/neato") {
		t.Errorf("src = %q, want the cmd attribute to pick neato", s)
	}
}

// A failing engine must leave the page standing, with the log where the image
// would have been.
func TestEngineFailureBecomesVisibleError(t *testing.T) {
	requireCommand(t, "mpost")
	root := t.TempDir()
	p, err := New(Options{Root: root, Renderers: []*Renderer{MetaPost}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := htmldoc.Parse(strings.NewReader(`<metapost>this is not metapost at all (((</metapost>`), p.RawTags()...)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Process(context.Background(), doc, Request{BaseURI: "/p.html"}); err != nil {
		t.Fatal(err)
	}
	pre := htmldoc.Elements(doc, "pre")
	if len(pre) != 1 {
		t.Fatalf("got %d error nodes, want 1", len(pre))
	}
	if style, _ := htmldoc.Attr(pre[0], "style"); style != "color:red" {
		t.Errorf("style = %q", style)
	}
	if got := htmldoc.TextContent(pre[0]); !strings.Contains(got, "mpost") {
		t.Errorf("error text does not name the engine: %q", got)
	}
}
