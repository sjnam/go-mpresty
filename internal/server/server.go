// Package server exposes the rendering pipeline over HTTP, taking the place of
// mpresty.conf and lib/mpresty.lua.
package server

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjnam/go-mpresty/internal/htmldoc"
	"github.com/sjnam/go-mpresty/internal/render"
)

// Server serves a document root, rendering graphics in HTML pages on the way
// out and passing everything else through as a static file.
type Server struct {
	root    string
	proc    *render.Processor
	rawTags []string
	files   http.Handler
	log     *slog.Logger
}

// New builds a Server over the given document root.
func New(root string, proc *render.Processor, log *slog.Logger) (*Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		root:    abs,
		proc:    proc,
		rawTags: proc.RawTags(),
		files:   http.FileServer(http.Dir(abs)),
		log:     log,
	}, nil
}

// Handler returns the routing table. It mirrors the locations mpresty.conf
// declared; the per-location Lua files become the updaters passed here.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /benchmark.html", s.benchmark())
	mux.Handle("GET /exemples.html", s.page(render.Updaters{Default: ExemplesUpdateNode}))
	mux.Handle("GET /updatenode.html", s.page(render.Updaters{ByTag: UpdateNodeByTag}))
	mux.Handle("POST /preview.gxn", s.preview())
	mux.Handle("/", s.page(render.Updaters{}))
	return mux
}

// page renders an HTML document with the given updaters, and serves anything
// else as a static file.
func (s *Server) page(up render.Updaters) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path
		if strings.HasSuffix(uri, "/") {
			uri += "index.html"
		}
		if !isHTML(uri) {
			s.files.ServeHTTP(w, r)
			return
		}
		body, err := s.read(uri)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out, err := s.render(r, body, uri, up)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(out)
	}
}

// benchmark is the page handler with the elapsed time appended, as the
// content_by_lua_block for /benchmark.html did.
func (s *Server) benchmark() http.HandlerFunc {
	inner := s.page(render.Updaters{})
	return func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		rec := &recorder{ResponseWriter: w}
		inner.ServeHTTP(rec, r)
		if rec.status == 0 || rec.status == http.StatusOK {
			fmt.Fprintf(w, "<p>elapsed seconds: %.6f</p>", time.Since(begin).Seconds())
		}
	}
}

// render parses, renders and serializes one document.
func (s *Server) render(r *http.Request, body []byte, uri string, up render.Updaters) ([]byte, error) {
	doc, err := htmldoc.Parse(bytes.NewReader(body), s.rawTags...)
	if err != nil {
		return nil, err
	}
	req := render.Request{
		BaseURI:  uri,
		NoCache:  r.URL.Query().Has("debug"),
		Updaters: up,
	}
	if err := s.proc.Process(r.Context(), doc, req); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := htmldoc.Render(&out, doc); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// read loads a file from the document root, refusing paths that escape it.
func (s *Server) read(uri string) ([]byte, error) {
	name := filepath.Join(s.root, filepath.FromSlash(path.Clean("/"+strings.TrimPrefix(uri, "/"))))
	if !strings.HasPrefix(name, s.root+string(os.PathSeparator)) {
		return nil, fs.ErrPermission
	}
	return os.ReadFile(name)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, "404 page not found", http.StatusNotFound)
	case errors.Is(err, fs.ErrPermission):
		http.Error(w, "403 forbidden", http.StatusForbidden)
	case errors.Is(err, r.Context().Err()) && r.Context().Err() != nil:
		// The client went away; nothing left to write to.
	default:
		s.log.Error("request failed", "uri", r.URL.Path, "err", err)
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
	}
}

func isHTML(uri string) bool {
	ext := strings.ToLower(path.Ext(uri))
	return ext == ".html" || ext == ".htm"
}

// recorder notes the status so benchmark only appends to successful pages.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
