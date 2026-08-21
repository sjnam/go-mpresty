package render

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/sjnam/go-mpresty/internal/cache"
	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// Options configures a Processor.
type Options struct {
	Root      string        // document root; graphics sources are read under it
	WorkDir   string        // rendered SVGs, relative to Root (default "images")
	Timeout   time.Duration // wall clock budget for one engine run (default 30s)
	MaxJobs   int           // concurrent engine runs (default GOMAXPROCS)
	Cache     *cache.Cache  // caches remote sources; created if nil
	Client    *http.Client  // for remote src attributes
	Renderers []*Renderer   // default: Default()
	Logger    *slog.Logger
}

// Processor renders every graphics node in a document. It is safe for
// concurrent use and holds no per-request state.
type Processor struct {
	root    string
	workDir string
	tmpDir  string
	timeout time.Duration
	maxJobs int
	cache   *cache.Cache
	client  *http.Client
	log     *slog.Logger

	byTag map[string]*Renderer
	byExt map[string]*Renderer
	group singleflight.Group
}

// New prepares a Processor, creating the work directory if needed.
func New(opts Options) (*Processor, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	if opts.WorkDir == "" {
		opts.WorkDir = "images"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxJobs <= 0 {
		opts.MaxJobs = runtime.GOMAXPROCS(0)
	}
	if opts.Cache == nil {
		opts.Cache = cache.New(0)
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 15 * time.Second}
	}
	if opts.Renderers == nil {
		opts.Renderers = Default()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	p := &Processor{
		root:    root,
		workDir: opts.WorkDir,
		tmpDir:  filepath.Join(root, filepath.FromSlash(opts.WorkDir), ".tmp"),
		timeout: opts.Timeout,
		maxJobs: opts.MaxJobs,
		cache:   opts.Cache,
		client:  opts.Client,
		log:     opts.Logger,
		byTag:   make(map[string]*Renderer),
		byExt:   make(map[string]*Renderer),
	}
	// Engine jobs run in a scratch directory under the work directory so the
	// finished SVG can be moved into place with a same-filesystem rename.
	if err := os.MkdirAll(p.tmpDir, 0o755); err != nil {
		return nil, err
	}
	for _, r := range opts.Renderers {
		p.byTag[r.Tag] = r
		p.byExt[r.Ext] = r
		// An engine that is not installed still renders — as a red error on
		// the page. Say so at startup rather than at the first request.
		if _, err := exec.LookPath(r.Cmd); err != nil {
			p.log.Warn("engine not installed; its elements will render as errors",
				"tag", r.Tag, "cmd", r.Cmd)
		}
	}
	return p, nil
}

// RawTags lists the element names whose bodies must reach the engine verbatim.
// Pass them to htmldoc.Parse.
func (p *Processor) RawTags() []string {
	tags := make([]string, 0, len(p.byTag))
	for tag := range p.byTag {
		tags = append(tags, tag)
	}
	return tags
}

// Request carries everything that varies from one page render to the next.
type Request struct {
	// BaseURI is the URI of the page being rendered; relative src attributes
	// resolve against its directory.
	BaseURI string
	// NoCache renders every node afresh, as ?debug did in the Lua version.
	NoCache bool
	// Updaters chooses how each rendered node is rewritten.
	Updaters Updaters
}

type job struct {
	r    *Renderer
	node *htmldoc.Node
}

// Process renders every graphics node in doc, in parallel, and rewrites it in
// place. A node that fails to render is replaced by its error message rather
// than failing the whole page; only cancellation is reported as an error.
func (p *Processor) Process(ctx context.Context, doc *htmldoc.Node, req Request) error {
	jobs := p.collect(doc)
	if len(jobs) == 0 {
		return nil
	}
	base := path.Dir(req.BaseURI)
	if base != "/" {
		base += "/"
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.maxJobs)
	for _, j := range jobs {
		g.Go(func() error {
			uri, content, err := p.render(ctx, j.r, j.node, base, !req.NoCache)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.log.Error("render failed", "tag", j.r.Tag, "uri", req.BaseURI, "err", err)
				htmldoc.RemoveChildren(j.node)
				ErrorUpdateNode(doc, j.node, "", err.Error())
				return nil
			}
			htmldoc.RemoveChildren(j.node)
			req.Updaters.For(j.r.Tag)(doc, j.node, uri, content)
			return nil
		})
	}
	return g.Wait()
}

// collect finds every node a renderer claims: the custom elements by tag name,
// and <img> elements whose src carries a known extension.
func (p *Processor) collect(doc *htmldoc.Node) []job {
	var jobs []job
	for tag, r := range p.byTag {
		for _, n := range htmldoc.Elements(doc, tag) {
			jobs = append(jobs, job{r: r, node: n})
		}
	}
	for _, n := range htmldoc.Elements(doc, "img") {
		src, ok := htmldoc.Attr(n, "src")
		if !ok {
			continue
		}
		if r, ok := p.byExt[strings.TrimPrefix(path.Ext(src), ".")]; ok {
			jobs = append(jobs, job{r: r, node: n})
		}
	}
	return jobs
}

// render produces the SVG for one node and returns its URI and source.
func (p *Processor) render(ctx context.Context, r *Renderer, node *htmldoc.Node, base string, useCache bool) (uri, content string, err error) {
	if htmldoc.AttrOr(node, "cache", "") == "no" {
		useCache = false
	}
	htmldoc.RemoveAttr(node, "cache")

	content, err = p.contents(ctx, node, base, useCache)
	if err != nil {
		return "", "", err
	}

	cmd := htmldoc.AttrOr(node, "cmd", r.Cmd)
	htmldoc.RemoveAttr(node, "cmd")
	if !r.allows(cmd) {
		return "", content, fmt.Errorf("%s: command %q is not allowed", r.Tag, cmd)
	}

	uri, err = p.image(ctx, r, cmd, content, useCache)
	if err != nil {
		return "", content, err
	}
	return uri, content, nil
}
