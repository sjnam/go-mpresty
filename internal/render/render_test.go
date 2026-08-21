package render

import (
	"context"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// fake is an engine that needs no TeX installed: it copies its source file to
// the SVG the pipeline expects, so tests exercise the plumbing, not MetaPost.
func fake() *Renderer {
	return &Renderer{
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
}

func newProcessor(t *testing.T, root string) *Processor {
	t.Helper()
	p, err := New(Options{Root: root, Renderers: []*Renderer{fake()}, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

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

func TestRendersInlineSource(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)

	doc := process(t, p, `<fake width="120">hello</fake>`, Request{})
	img := htmldoc.Elements(doc, "img")
	if len(img) != 1 {
		t.Fatalf("got %d img elements, want 1", len(img))
	}
	src, _ := htmldoc.Attr(img[0], "src")
	want := "/images/cp" + crcOf("hello") + ".svg"
	if src != want {
		t.Errorf("src = %q, want %q", src, want)
	}
	if w, _ := htmldoc.Attr(img[0], "width"); w != "120" {
		t.Errorf("width = %q, want the author's 120", w)
	}
	// The engine saw preamble and postamble wrapped around the source.
	b, err := os.ReadFile(filepath.Join(root, "images", "cp"+crcOf("hello")+".svg"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "P:hello:Q" {
		t.Errorf("engine input = %q, want %q", got, "P:hello:Q")
	}
}

func TestDefaultWidthWhenAuthorGivesNone(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	doc := process(t, p, `<fake>x</fake>`, Request{})
	if w, _ := htmldoc.Attr(htmldoc.Elements(doc, "img")[0], "width"); w != "300" {
		t.Errorf("width = %q, want 300", w)
	}
}

// The <img src="....txt"> form must render just like the custom element.
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

// A relative src resolves against the directory of the page, as it did in Lua.
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

// A traversing src must never reach outside the document root. Cleaning the
// URI as an absolute path neutralises it, so the file simply is not there.
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

// resolve keeps the guard for callers that pass a path not rooted by Clean.
func TestResolveRefusesEscape(t *testing.T) {
	p := newProcessor(t, t.TempDir())
	if _, err := p.resolve("../../etc/passwd", ""); err == nil {
		t.Error("resolve accepted a path that escapes the root")
	}
	if _, err := p.resolve("a.txt", "/docs/"); err != nil {
		t.Errorf("resolve rejected an ordinary path: %v", err)
	}
}

// A cmd attribute picks the engine binary, so it must be bounded by the
// renderer's allow-list — the Lua version pasted it into a shell string.
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

// An already rendered file is reused untouched; ?debug and cache="no" rebuild.
func TestCacheReusesRenderedFile(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)
	svg := filepath.Join(root, "images", "cp"+crcOf("keep")+".svg")
	writeFile(t, svg, "SENTINEL")

	process(t, p, `<fake>keep</fake>`, Request{})
	if got := readFile(t, svg); got != "SENTINEL" {
		t.Errorf("cached file was rebuilt: %q", got)
	}

	process(t, p, `<fake>keep</fake>`, Request{NoCache: true})
	if got := readFile(t, svg); got != "P:keep:Q" {
		t.Errorf("NoCache did not rebuild: %q", got)
	}

	writeFile(t, svg, "SENTINEL")
	process(t, p, `<fake cache="no">keep</fake>`, Request{})
	if got := readFile(t, svg); got != "P:keep:Q" {
		t.Errorf(`cache="no" did not rebuild: %q`, got)
	}
}

// The attributes the pipeline consumes must not survive into the output.
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

// Many nodes at once is the whole point of the port; run this under -race.
func TestConcurrentNodes(t *testing.T) {
	root := t.TempDir()
	p := newProcessor(t, root)
	var b strings.Builder
	for i := range 40 {
		b.WriteString("<fake>n")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("</fake>")
	}
	doc := process(t, p, b.String(), Request{})
	if n := len(htmldoc.Elements(doc, "img")); n != 40 {
		t.Fatalf("got %d img elements, want 40", n)
	}
	if n := len(htmldoc.Elements(doc, "pre")); n != 0 {
		t.Errorf("got %d error nodes, want none", n)
	}
}

// Two documents racing on the same source must not fight over the same file.
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
	if got := readFile(t, filepath.Join(root, "images", "cp"+crcOf("same")+".svg")); got != "P:same:Q" {
		t.Errorf("svg = %q", got)
	}
}

// crcOf mirrors how the pipeline names a file: the CRC-32 of the source
// itself, before preamble and postamble are wrapped around it.
func crcOf(s string) string {
	return strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(s))), 10)
}
