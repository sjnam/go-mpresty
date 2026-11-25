// Package render turns graphics source embedded in an HTML document
// into SVG images.
//
//line render.gw:51
//line render.gw:52
//line render.gw:53
package render

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/sjnam/go-mpresty/internal/cache"
	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// Renderer describes one graphics engine. Every field is read-only once
// the Renderer is registered, so a single value is shared safely.
//
//line render.gw:94
//line render.gw:95
//line render.gw:96
type Renderer struct {
	Tag       string   // custom element name, e.g. "metapost"
	Ext       string   // source file extension, e.g. "mp"
	Cmd       string   // default command
	Commands  []string // commands a cmd attribute may select
	Preamble  string
	Postamble string

	// Command returns the argv to run inside a job directory that already
	// holds fname.Ext. It must leave fname.svg there.
	Command func(cmd, fname string) []string
}

// MetaPost renders metapost elements and .mp sources.
//
//line render.gw:117
//line render.gw:118
var MetaPost = &Renderer{
	Tag:       "metapost",
	Ext:       "mp",
	Cmd:       "mpost",
	Commands:  []string{"mpost", "mpost-lua"},
	Preamble:  "prologues:=3;\noutputtemplate:=\"%j.svg\";\noutputformat:=\"svg\";\n",
	Postamble: "\nend\n",
	Command: func(cmd, fname string) []string {
		return []string{cmd, "-interaction=nonstopmode", fname + ".mp"}
	},
}

// Graphviz renders graphviz elements and .gv sources.
//
//line render.gw:133
//line render.gw:134
var Graphviz = &Renderer{
	Tag: "graphviz",
	Ext: "gv",
	Cmd: "dot",
	Commands: []string{
		"dot", "neato", "fdp", "sfdp", "twopi", "circo", "osage", "patchwork",
	},
	Command: func(cmd, fname string) []string {
		return []string{cmd, "-Tsvg", fname + ".gv", "-o", fname + ".svg"}
	},
}

// UpdateNodeFunc rewrites one graphics node once its image is ready.
//
//line render.gw:156
//line render.gw:157
type UpdateNodeFunc func(doc, node *htmldoc.Node, uri, content string)

// Updaters selects an UpdateNodeFunc per tag: an entry of ByTag if there
// is one, else Default, else DefaultUpdateNode.
//
//line render.gw:159
//line render.gw:160
//line render.gw:161
type Updaters struct {
	Default UpdateNodeFunc
	ByTag   map[string]UpdateNodeFunc
}

// Options configures a Processor.
//
//line render.gw:184
//line render.gw:185
type Options struct {
	Root      string        // document root; sources are read under it
	WorkDir   string        // rendered SVGs, relative to Root
	Timeout   time.Duration // wall clock budget for one engine run
	MaxJobs   int           // concurrent engine runs
	Cache     *cache.Cache  // caches remote sources
	Client    *http.Client  // fetches remote sources
	Renderers []*Renderer   // the engines
	Logger    *slog.Logger
}

// Processor renders every graphics node in a document. It is safe for
// concurrent use and holds no per-request state.
//
//line render.gw:200
//line render.gw:201
//line render.gw:202
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

// Request carries everything that varies from one page render to the next.
//
//line render.gw:310
//line render.gw:311
type Request struct {
	// BaseURI is the URI of the page being rendered; relative src
	// attributes resolve against its directory.
	BaseURI string
	// NoCache renders every node afresh.
	NoCache bool
	// Updaters chooses how each rendered node is rewritten.
	Updaters Updaters
}

//line render.gw:353
type job struct {
	r    *Renderer
	node *htmldoc.Node
}

// DefaultUpdateNode replaces the node with an img pointing at the SVG.
//
//line render.gw:170
//line render.gw:171
func DefaultUpdateNode(doc, node *htmldoc.Node, uri, content string) {
	htmldoc.Rename(node, "img")
	htmldoc.SetAttr(node, "src", uri)
	if !htmldoc.HasAttr(node, "width") {
		htmldoc.SetAttr(node, "width", "300")
	}
}

// New prepares a Processor, creating the work directory if needed.
//
//line render.gw:220
//line render.gw:221
func New(opts Options) (*Processor, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}

//line render.gw:240
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
		opts.Renderers = []*Renderer{MetaPost, Graphviz}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

//line render.gw:227

//line render.gw:267
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

//line render.gw:228
	if err := os.MkdirAll(p.tmpDir, 0o755); err != nil {
		return nil, err
	}

//line render.gw:283
	for _, r := range opts.Renderers {
		p.byTag[r.Tag] = r
		p.byExt[r.Ext] = r
		if _, err := exec.LookPath(r.Cmd); err != nil {
			p.log.Warn("engine not installed; its elements will render as errors",
				"tag", r.Tag, "cmd", r.Cmd)
		}
	}

//line render.gw:232
	return p, nil
}

// RawTags lists the element names whose bodies must reach the engine
// verbatim. Pass them to htmldoc.Parse.
//
//line render.gw:296
//line render.gw:297
//line render.gw:298
func (p *Processor) RawTags() []string {
	tags := make([]string, 0, len(p.byTag))
	for tag := range p.byTag {
		tags = append(tags, tag)
	}
	return tags
}

// Process renders every graphics node in doc, in parallel, and rewrites
// it in place. A node that fails to render shows its error instead; only
// cancellation is reported as an error.
//
//line render.gw:331
//line render.gw:332
//line render.gw:333
//line render.gw:334
func (p *Processor) Process(ctx context.Context, doc *htmldoc.Node, req Request) error {

//line render.gw:363
	var jobs []job
	for tag, r := range p.byTag {
		for _, n := range htmldoc.Elements(doc, tag) {
			jobs = append(jobs, job{r: r, node: n})
		}
	}
	for _, n := range htmldoc.Elements(doc, "img") {
		if src, ok := htmldoc.Attr(n, "src"); ok {
			if r, ok := p.byExt[strings.TrimPrefix(path.Ext(src), ".")]; ok {
				jobs = append(jobs, job{r: r, node: n})
			}
		}
	}

//line render.gw:336
	if len(jobs) == 0 {
		return nil
	}

//line render.gw:381
	base := path.Dir(req.BaseURI)
	if base != "/" {
		base += "/"
	}

//line render.gw:340
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.maxJobs)
	for _, j := range jobs {
		g.Go(func() error {

//line render.gw:396
			r, node := j.r, j.node
			var content, cmd, uri string
			var err error

//line render.gw:418
			useCache := !req.NoCache
			if htmldoc.AttrOr(node, "cache", "") == "no" {
				useCache = false
			}
			htmldoc.RemoveAttr(node, "cache")

//line render.gw:428
			if src, ok := htmldoc.Attr(node, "src"); !ok {
				content = htmldoc.TextContent(node)
			} else {
				htmldoc.RemoveAttr(node, "src")
				if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {

//line render.gw:480
					key := "src:" + src
					hit := false
					if useCache {
						content, hit = p.cache.Get(key)
					}
					if !hit {

//line render.gw:497
						var hreq *http.Request
						var res *http.Response
						hreq, err = http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
						if err == nil {
							res, err = p.client.Do(hreq)
						}
						if err == nil {
							if res.StatusCode != http.StatusOK {
								err = fmt.Errorf("fetch %s: %s", src, res.Status)
							} else if b, rerr := io.ReadAll(io.LimitReader(res.Body, 4<<20)); rerr != nil {
								err = fmt.Errorf("fetch %s: %w", src, rerr)
							} else {
								content = string(b)
							}
							res.Body.Close()
						}

//line render.gw:487
						if err == nil {
							p.cache.Set(key, content)
						}
					}

//line render.gw:434
				} else {

//line render.gw:445
					var name string
					if name, err = p.resolve(src, base); err == nil {
						var b []byte
						if b, err = os.ReadFile(name); err == nil {
							content = string(b)
						} else {
							err = fmt.Errorf("read %s: %w", src, err)
						}
					}

//line render.gw:436
				}
			}

//line render.gw:401
			if err == nil {

//line render.gw:517
				cmd = htmldoc.AttrOr(node, "cmd", r.Cmd)
				htmldoc.RemoveAttr(node, "cmd")
				if cmd != r.Cmd && !slices.Contains(r.Commands, cmd) {
					err = fmt.Errorf("%s: command %q is not allowed", r.Tag, cmd)
				}

//line render.gw:403
			}
			if err == nil {

//line render.gw:541
				fname := cmd + strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(content))), 10)
				uri = path.Join("/", p.workDir, fname+".svg")
				if !useCache || !p.rendered(fname) {
					_, err, _ = p.group.Do(r.Tag+fname, func() (any, error) {
						if useCache && p.rendered(fname) {
							return nil, nil
						}

//line render.gw:575
						ctx, cancel := context.WithTimeout(ctx, p.timeout)
						defer cancel()
						dir, err := os.MkdirTemp(p.tmpDir, "job-")
						if err != nil {
							return nil, err
						}
						defer os.RemoveAll(dir)
						text := r.Preamble + content + r.Postamble
						if err := os.WriteFile(filepath.Join(dir, fname+"."+r.Ext), []byte(text), 0o644); err != nil {
							return nil, err
						}

//line render.gw:602
						argv := r.Command(cmd, fname)
						c := exec.CommandContext(ctx, argv[0], argv[1:]...)
						c.Dir = dir
						c.Stdin = nil
						c.Env = append(os.Environ(),
							"TEXMFOUTPUT="+dir,
							"openout_any=p",
							"MPINPUTS="+p.inputs(dir),
							"TEXINPUTS="+p.inputs(dir),
						)
						var out bytes.Buffer
						c.Stdout, c.Stderr = &out, &out
						if err := c.Run(); err != nil {
							if ctx.Err() != nil {
								return nil, fmt.Errorf("%s: %w", argv[0], ctx.Err())
							}
							diag := out.String()

//line render.gw:640
							if strings.TrimSpace(diag) == "" {
								if b, err := os.ReadFile(filepath.Join(dir, fname+".log")); err == nil {
									diag = string(b)
								}
							}
							lines := strings.Split(strings.TrimRight(diag, "\n"), "\n")
							if len(lines) > 30 {
								lines = append([]string{"..."}, lines[len(lines)-30:]...)
							}
							diag = strings.Join(lines, "\n")

//line render.gw:620
							return nil, fmt.Errorf("%s: %w\n%s", argv[0], err, diag)
						}

//line render.gw:654
						svg := filepath.Join(dir, fname+".svg")
						if _, err := os.Stat(svg); err != nil {
							return nil, fmt.Errorf("%s produced no %s.svg", cmd, fname)
						}
						return nil, os.Rename(svg, p.svgPath(fname))

//line render.gw:549
					})
				}

//line render.gw:406
			}
			if err != nil {

//line render.gw:666
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.log.Error("render failed", "tag", r.Tag, "uri", req.BaseURI, "err", err)
				htmldoc.Rename(node, "pre")
				htmldoc.SetTextContent(node, err.Error())
				htmldoc.SetAttr(node, "style", "color:red")
				htmldoc.RemoveAttr(node, "width")

//line render.gw:409
			} else {

//line render.gw:679
				htmldoc.RemoveChildren(node)
				update := DefaultUpdateNode
				if fn := req.Updaters.ByTag[r.Tag]; fn != nil {
					update = fn
				} else if req.Updaters.Default != nil {
					update = req.Updaters.Default
				}
				update(doc, node, uri, content)

//line render.gw:411
			}

//line render.gw:345
			return nil
		})
	}
	return g.Wait()
}

// resolve maps a document-relative or absolute URI to a path inside the
// document root, refusing anything that would escape it.
//
//line render.gw:462
//line render.gw:463
//line render.gw:464
func (p *Processor) resolve(uri, base string) (string, error) {
	if !strings.HasPrefix(uri, "/") {
		uri = base + uri
	}
	name := filepath.Join(p.root, filepath.FromSlash(path.Clean(uri)))
	if name != p.root && !strings.HasPrefix(name, p.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the document root", uri)
	}
	return name, nil
}

// rendered reports whether the SVG for this name is already on disk.
//
//line render.gw:554
//line render.gw:555
func (p *Processor) rendered(fname string) bool {
	_, err := os.Stat(p.svgPath(fname))
	return err == nil
}

func (p *Processor) svgPath(fname string) string {
	return filepath.Join(p.root, filepath.FromSlash(p.workDir), fname+".svg")
}

// inputs is the kpathsea search path for a job.
//
//line render.gw:628
//line render.gw:629
func (p *Processor) inputs(dir string) string {
	sep := string(filepath.ListSeparator)
	sub := p.root + string(filepath.Separator) + string(filepath.Separator)
	return strings.Join([]string{dir, sub, ""}, sep)
}
