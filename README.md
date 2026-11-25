# go-mpresty

A web application that turns MetaPost and Graphviz source written *inside* an
HTML page into SVG images, on the way out.

This is a Go port of [mpresty](https://github.com/sjnam/mpresty), which did the
same thing with OpenResty and Lua. It keeps the page syntax, the routes and the
`images/` cache layout of the original, so existing pages work unchanged.

## Getting started

```bash
make build
./mpresty -root webapp/html
```

`make build` first tangles the Go sources out of the `.gw` files (see
[Literate source](#literate-source)); it needs nothing but Go.

Or with Docker, which brings the engines with it:

```bash
docker compose up -d
```

Then visit <http://localhost:8080/all.html>, `/tutorial.html`, `/sunflower.html`,
`/benchmark.html` or `/preview.html`.

## Writing a page

Put the graphics source in a custom element, or point an ordinary `<img>` at a
source file and let the extension choose the engine:

```html
<html>
<body>
<h1>Metapost</h1>
<metapost width="200">
beginfig(1)
  u:=1.3cm; transform T; z1=(0,2u); n:=5;
  for i=1 upto n-1: z[i+1]=z1 rotated (360*i/n); endfor;
  path p; p = for i=1 upto n: z[i]--endfor cycle;
  for i=0 upto 100:
    fill p withcolor 0.2*white; p:=p transformed T;
  endfor;
endfig;
</metapost>

<img src="tree.mp" width="200">
<img src="/source/dot.gv" cmd="neato" width="200">
<img src="https://example.org/source/rgb.mp" width="200">
</body>
</html>
```

| element | file extension | engine | `cmd` may select |
| --- | --- | --- | --- |
| `<metapost>` | `.mp` | `mpost` | `mpost`, `mpost-lua` |
| `<graphviz>` | `.gv` | `dot` | `dot`, `neato`, `fdp`, `sfdp`, `twopi`, `circo`, `osage`, `patchwork` |

Attributes the renderer consumes and strips: `src`, `cmd`, and `cache="no"` to
force a rebuild of that one image. Appending `?debug` to the page URL rebuilds
every image on it.

A source that fails to render does not take the page down with it — the engine
log appears in red where the image would have been.

## Routes

The routes `mpresty.conf` declared, and what replaced each one:

| route | Lua original | here |
| --- | --- | --- |
| `/*.html` | `content_by_lua_block` | default rewrite to `<img>` |
| `/benchmark.html` | timing wrapper | same, appends elapsed seconds |
| `/exemples.html` | `lua/exemples.lua` | `server.ExemplesUpdateNode` |
| `/updatenode.html` | `lua/upnode.lua` | `server.UpdateNodeByTag` |
| `/preview.gxn` | `lua/preview.lua` | the preview route in `internal/server` |
| everything else | `nginx` static | `http.FileServer` |

## Flags

```text
-addr string       address to listen on (default ":8080")
-root string       document root (default "webapp/html")
-workdir string    directory for rendered SVGs, relative to root (default "images")
-timeout duration  wall clock budget for one engine run (default 30s)
-jobs int          concurrent engine runs (0 = GOMAXPROCS)
-cache int         cache entries to keep (0 = default)
-v                 log at debug level
```

## Requirements

MetaPost and Graphviz, and nothing else. `btex ... etex` labels are typeset by
the plain eTeX that comes with MetaPost, so no LaTeX installation is needed;
the sample figures that once opened `verbatimtex %&latex` now say `%&plain`.

Since Graphviz 14 the `graphviz` package carries only `dot`; `neato`, `fdp`,
`sfdp`, `twopi` and `circo` need the neato-layout plugin
(`libgvplugin-neato-layout8` on Debian and Ubuntu), which nothing pulls in for
you. The [Dockerfile](Dockerfile) installs it, on Ubuntu 26.04 LTS, which
brings TeX Live 2025 and Graphviz 14 for a 334MB image.

mpresty names any engine it cannot find on `PATH` when it starts.

## How it differs from the Lua original

Four of these are fixes, one is a loss.

**Graphics source survives the HTML parser.** The HTML5 tokenizer reads `<`
followed by a letter as a tag open, so a line like `if a<b:` lost everything
from the `<` onward under gumbo. Each graphics element's body is now lifted out
before parsing and put back afterwards, exactly as written — the same treatment
HTML gives `<script>`. One consequence: entities inside a graphics element are
no longer decoded, so write `&`, not `&amp;`.

**No shared per-request state.** `lib/mpresty/base.lua` stored `cmd`, `doc` and
`use_cache` on the module singleton while several light threads walked the
document. That was safe only because `ngx.thread` coroutines are cooperative and
single-threaded. Goroutines are not, so a `Renderer` here is immutable
configuration and everything per-request travels as an argument.

**Each engine run gets its own directory.** The Lua version ran every engine in
`images/` and cleaned up with `rm F.log F.mp*`. MetaPost's `makempx` writes
fixed-name temporaries, so genuinely parallel runs would have collided. Jobs now run in a scratch directory
and only the finished SVG is moved into `images/`. Sources that `input` a
sibling still resolve: the document root is on the kpathsea search path.

**A `cmd` attribute cannot reach a shell.** It used to be pasted into a shell
string; commands now run as argv, and each renderer has an allow-list. Local
`src` paths are confined to the document root, which `ngx.location.capture` used
to do for free.

**Customisation is compiled, not scripted.** This is what the port gives up.
Dropping `exemples.lua` next to the config was enough before; now an
`UpdateNodeFunc` is registered on a route in `internal/server`. Embedding a Lua
VM would restore it if that flexibility is worth the dependency.

## Performance

Rendering the 32 images of `benchmark.html` from a cold cache, on a 10-core M-series Mac:

| `-jobs` | time |
| --- | --- |
| 1 | 8.45s |
| 4 | 2.52s |
| 0 (GOMAXPROCS) | 1.43s |

Warm, the same page is served in about a millisecond: a rendered file is its own
cache, so it survives a restart, and identical concurrent requests collapse into
one engine run.

## Layout

```text
cmd/mpresty/      flags, wiring, graceful shutdown
internal/render/  the engines, the job runner, the document walker
internal/htmldoc/ parsing with shielded raw bodies, DOM helpers, serialization
internal/cache/   LRU for remote sources
internal/server/  routes, static files, the per-route updaters
webapp/html/      the sample pages, carried over unchanged
```

## Literate source

Each Go directory above holds a single [GWEB](https://github.com/sjnam/gweb)
literate program, `<name>.gw`, from which `gtangle` produces `<name>.go` and,
for the packages that have tests, `<name>_test.go`. Only the `.gw` files are
in the repository; the Go files are generated, by `make` here and by the
Dockerfile in the image.

```bash
make tangle   # .gw -> .go and _test.go
make test     # tangles first, as build, vet and race do
make doc      # typeset each .gw as a PDF next to it (needs gweave and LuaTeX)
```

The `@s` directives that tell `gweave` which imported types to set in bold
are shared: they live in `types.gw` at the top of the repository, and every
web reads it with `@i ../../types.gw` (GWEB resolves `@i` against the
including file's directory).

`gtangle` is pinned by the `tool` directive in `go.mod` and runs as
`go tool gtangle`, so no separate installation is needed. After a fresh
clone, run `make tangle` before `go build`, `go test` or an editor can see
the code. The `//line` comments in the tangled files point compiler errors
and stack traces back at the `.gw` files.

## License

Public Domain, as the original.
