//line render.gw:695
package render

import (
	"context"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

//line render.gw:717
func newProcessor(t *testing.T, root string) *Processor {
	t.Helper()
	fake := &Renderer{
		Tag:       "fake",
		Ext:       "txt",
		Cmd:       "cp",
		Commands:  []string{"cp"},
		Preamble:  "P:",
		Postamble: ":Q",
		Command: func(cmd, fname string) []string {
			return []string{"cp", fname + ".txt", fname + ".svg"}
		},
	}
	p, err := New(Options{Root: root, Renderers: []*Renderer{fake}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

//line render.gw:743
func process(t *testing.T, p *Processor, html string, req Request) *htmldoc.Node {
	t.Helper()
	doc, err := htmldoc.Parse(strings.NewReader(html), p.RawTags()...)
	if err != nil {
		t.Fatal(err)
	}
	if req.BaseURI == "" {
		req.BaseURI = "/page.html"
	}
	if err := p.Process(context.Background(), doc, req); err != nil {
		t.Fatal(err)
	}
	return doc
}

//line render.gw:761
func crcOf(s string) string {
	return strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(s))), 10)
}

//line render.gw:768
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

//line render.gw:1003
func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not installed", name)
	}
}

//line render.gw:791
func TestRendersInlineSource(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)
	doc := process(t, p, `<fake width="120">hello</fake>`, Request{})
	img := htmldoc.Elements(doc, "img")
	if len(img) != 1 {
		t.Fatalf("got %d img elements, want 1", len(img))
	}
	src, _ := htmldoc.Attr(img[0], "src")
	if want := "/images/cp" + crcOf("hello") + ".svg"; src != want {
		t.Errorf("src = %q, want %q", src, want)
	}
	if w, _ := htmldoc.Attr(img[0], "width"); w != "120" {
		t.Errorf("width = %q, want the author's 120", w)
	}
	got := readFile(t, filepath.Join(root, "images", "cp"+crcOf("hello")+".svg"))
	if got != "P:hello:Q" {
		t.Errorf("engine input = %q, want %q", got, "P:hello:Q")
	}
}

//line render.gw:814
func TestDefaultWidthWhenAuthorGivesNone(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	doc := process(t, p, `<fake>x</fake>`, Request{})
	if w, _ := htmldoc.Attr(htmldoc.Elements(doc, "img")[0], "width"); w != "300" {
		t.Errorf("width = %q, want 300", w)
	}
}

//line render.gw:825
func TestRendersImgBySourceExtension(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "source", "a.txt"), "from-file")
	p := newProcessor(t, root)
	doc := process(t, p, `<img src="/source/a.txt" width="90">`, Request{})
	src, _ := htmldoc.Attr(htmldoc.Elements(doc, "img")[0], "src")
	if want := "/images/cp" + crcOf("from-file") + ".svg"; src != want {
		t.Errorf("src = %q, want %q", src, want)
	}
}

func TestRelativeSourceResolvesAgainstPage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "b.txt"), "sibling")
	p := newProcessor(t, root)
	doc := process(t, p, `<img src="b.txt">`, Request{BaseURI: "/docs/index.html"})
	src, _ := htmldoc.Attr(htmldoc.Elements(doc, "img")[0], "src")
	if want := "/images/cp" + crcOf("sibling") + ".svg"; src != want {
		t.Errorf("src = %q, want %q", src, want)
	}
}

//line render.gw:852
func TestTraversingSourceStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(filepath.Dir(root), "secret.txt"), "outside")
	p := newProcessor(t, root)
	doc := process(t, p, `<img src="/../secret.txt">`, Request{})
	pre := htmldoc.Elements(doc, "pre")
	if len(pre) != 1 {
		t.Fatalf("got %d error nodes, want 1", len(pre))
	}
	if got := htmldoc.TextContent(pre[0]); strings.Contains(got, "outside") {
		t.Errorf("the file outside the root was read: %q", got)
	}
	if n := len(htmldoc.Elements(doc, "img")); n != 0 {
		t.Errorf("got %d rendered images, want none", n)
	}
}

//line render.gw:872
func TestResolveRefusesEscape(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	if _, err := p.resolve("../../etc/passwd", ""); err == nil {
		t.Error("resolve accepted a path that escapes the root")
	}
	if _, err := p.resolve("a.txt", "/docs/"); err != nil {
		t.Errorf("resolve rejected an ordinary path: %v", err)
	}
}

//line render.gw:885
func TestRejectsDisallowedCommand(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	doc := process(t, p, `<fake cmd="rm">x</fake>`, Request{})
	pre := htmldoc.Elements(doc, "pre")
	if len(pre) != 1 {
		t.Fatalf("got %d error nodes, want 1", len(pre))
	}
	if got := htmldoc.TextContent(pre[0]); !strings.Contains(got, "not allowed") {
		t.Errorf("error text = %q", got)
	}
}

//line render.gw:901
func TestCacheReusesRenderedFile(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)
	svg := filepath.Join(root, "images", "cp"+crcOf("keep")+".svg")
	writeFile(t, svg, "SENTINEL")
	process(t, p, `<fake>keep</fake>`, Request{})
	if got := readFile(t, svg); got != "SENTINEL" {
		t.Errorf("cached file was rebuilt: %q", got)
	}

//line render.gw:914
	process(t, p, `<fake>keep</fake>`, Request{NoCache: true})
	if got := readFile(t, svg); got != "P:keep:Q" {
		t.Errorf("NoCache did not rebuild: %q", got)
	}
	writeFile(t, svg, "SENTINEL")
	process(t, p, `<fake cache="no">keep</fake>`, Request{})
	if got := readFile(t, svg); got != "P:keep:Q" {
		t.Errorf(`cache="no" did not rebuild: %q`, got)
	}

//line render.gw:911
}

//line render.gw:926
func TestConsumedAttributesAreRemoved(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	doc := process(t, p, `<fake cmd="cp" cache="no" width="50">x</fake>`, Request{})
	img := htmldoc.Elements(doc, "img")[0]
	for _, k := range []string{"cmd", "cache"} {
		if htmldoc.HasAttr(img, k) {
			t.Errorf("attribute %q leaked into the output", k)
		}
	}
}

//line render.gw:940
func TestUpdatersSelectPerTag(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	req := Request{Updaters: Updaters{ByTag: map[string]UpdateNodeFunc{
		"fake": func(doc, node *htmldoc.Node, uri, content string) {
			htmldoc.Rename(node, "figure")
			htmldoc.SetTextContent(node, content+"|"+uri)
		},
	}}}
	doc := process(t, p, `<fake>src</fake>`, req)
	fig := htmldoc.Elements(doc, "figure")
	if len(fig) != 1 {
		t.Fatalf("got %d figure elements, want 1", len(fig))
	}
	if got := htmldoc.TextContent(fig[0]); !strings.HasPrefix(got, "src|/images/") {
		t.Errorf("figure text = %q", got)
	}
}

//line render.gw:963
func TestConcurrentNodes(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	var b strings.Builder
	for i := range 40 {
		b.WriteString("<fake>n" + strconv.Itoa(i) + "</fake>")
	}
	doc := process(t, p, b.String(), Request{})
	if n := len(htmldoc.Elements(doc, "img")); n != 40 {
		t.Fatalf("got %d img elements, want 40", n)
	}
	if n := len(htmldoc.Elements(doc, "pre")); n != 0 {
		t.Errorf("got %d error nodes, want none", n)
	}
}

//line render.gw:979
func TestConcurrentIdenticalSources(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			doc, _ := htmldoc.Parse(strings.NewReader(`<fake>same</fake>`), p.RawTags()...)
			p.Process(context.Background(), doc, Request{BaseURI: "/p.html"})
		}()
	}
	for range 8 {
		<-done
	}
	svg := filepath.Join(root, "images", "cp"+crcOf("same")+".svg")
	if got := readFile(t, svg); got != "P:same:Q" {
		t.Errorf("svg = %q", got)
	}
}

//line render.gw:1013
func TestMetaPostEndToEnd(t *testing.T) {
	requireCommand(t, "mpost")
	p, err := New(Options{Root: t.TempDir(), Renderers: []*Renderer{MetaPost}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
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

//line render.gw:1034
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
	uri, _ := htmldoc.Attr(img[0], "src")
	if !strings.HasPrefix(uri, "/images/mpost") || !strings.HasSuffix(uri, ".svg") {
		t.Errorf("src = %q", uri)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(uri, "/images/"), ".svg")
	if got := readFile(t, p.svgPath(name)); !strings.Contains(got, "<svg") {
		t.Errorf("output is not an SVG: %.60q", got)
	}

//line render.gw:1031
}

//line render.gw:1055
func TestGraphvizEndToEnd(t *testing.T) {
	requireCommand(t, "dot")
	p, err := New(Options{Root: t.TempDir(), Renderers: []*Renderer{Graphviz}, Logger: quietLogger()})
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

//line render.gw:1079
func TestEngineFailureBecomesVisibleError(t *testing.T) {
	requireCommand(t, "mpost")
	p, err := New(Options{Root: t.TempDir(), Renderers: []*Renderer{MetaPost}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	const src = `<metapost>this is not metapost at all (((</metapost>`
	doc, err := htmldoc.Parse(strings.NewReader(src), p.RawTags()...)
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
