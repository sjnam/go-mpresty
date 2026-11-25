// Package server exposes the rendering pipeline over HTTP.
//
//line server.gw:38
//line server.gw:39
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

// Server serves a document root, rendering graphics in HTML pages on the
// way out and passing everything else through as a static file.
//
//line server.gw:65
//line server.gw:66
//line server.gw:67
type Server struct {
	root    string
	proc    *render.Processor
	rawTags []string
	files   http.Handler
	log     *slog.Logger
}

// recorder notes the status so benchmark only appends to successful pages.
//
//line server.gw:223
//line server.gw:224
type recorder struct {
	http.ResponseWriter
	status int
}

// UpdateNodeByTag is the per-tag table /updatenode.html uses.
//
//line server.gw:309
//line server.gw:310
var UpdateNodeByTag = map[string]render.UpdateNodeFunc{
	"metapost": ExemplesUpdateNode,
}

// New builds a Server over the given document root.
//
//line server.gw:76
//line server.gw:77
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

// Handler returns the routing table.
//
//line server.gw:101
//line server.gw:102
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

//line server.gw:210
	inner := s.page(render.Updaters{})
	mux.HandleFunc("GET /benchmark.html", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		rec := &recorder{ResponseWriter: w}
		inner.ServeHTTP(rec, r)
		if rec.status == 0 || rec.status == http.StatusOK {
			fmt.Fprintf(w, "<p>elapsed seconds: %.6f</p>", time.Since(begin).Seconds())
		}
	})

//line server.gw:105
	mux.Handle("GET /exemples.html", s.page(render.Updaters{Default: ExemplesUpdateNode}))
	mux.Handle("GET /updatenode.html", s.page(render.Updaters{ByTag: UpdateNodeByTag}))

//line server.gw:246
	allowed := make(map[string]bool, len(s.rawTags))
	for _, tag := range s.rawTags {
		allowed[tag] = true
	}
	mux.HandleFunc("POST /preview.gxn", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "400 bad request", http.StatusBadRequest)
			return
		}
		gx := r.PostFormValue("gx")
		if !allowed[gx] {
			http.Error(w, "400 unknown graphics tag", http.StatusBadRequest)
			return
		}

//line server.gw:270
		var cmd string
		if c := r.PostFormValue("cmd"); c != "" {
			cmd = fmt.Sprintf(" cmd=%q", c)
		}
		body := []byte(strings.Join([]string{
			"<", gx, cmd, " cache='no' width='400'>\n",
			r.PostFormValue("code"),
			"\n</", gx, ">",
		}, ""))

//line server.gw:261
		uri := "/preview.gxn"
		up := render.Updaters{}

//line server.gw:151
		out, err := s.render(r, body, uri, up)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(out)

//line server.gw:264
	})

//line server.gw:108
	mux.Handle("/", s.page(render.Updaters{}))
	return mux
}

// page renders an HTML document with the given updaters, and serves
// anything else as a static file.
//
//line server.gw:116
//line server.gw:117
//line server.gw:118
func (s *Server) page(up render.Updaters) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path
		if strings.HasSuffix(uri, "/") {
			uri += "index.html"
		}
		if ext := strings.ToLower(path.Ext(uri)); ext != ".html" && ext != ".htm" {
			s.files.ServeHTTP(w, r)
			return
		}

//line server.gw:141
		name := filepath.Join(s.root, filepath.FromSlash(path.Clean("/"+strings.TrimPrefix(uri, "/"))))
		var body []byte
		var err error
		if strings.HasPrefix(name, s.root+string(os.PathSeparator)) {
			body, err = os.ReadFile(name)
		} else {
			err = fs.ErrPermission
		}

//line server.gw:129
		if err != nil {
			s.fail(w, r, err)
			return
		}

//line server.gw:151
		out, err := s.render(r, body, uri, up)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(out)

//line server.gw:134
	}
}

// render parses, renders and serializes one document.
//
//line server.gw:164
//line server.gw:165
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

//line server.gw:189
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, "404 page not found", http.StatusNotFound)
	case errors.Is(err, fs.ErrPermission):
		http.Error(w, "403 forbidden", http.StatusForbidden)
	case r.Context().Err() != nil && errors.Is(err, r.Context().Err()):
	default:
		s.log.Error("request failed", "uri", r.URL.Path, "err", err)
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
	}
}

//line server.gw:230
func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// ExemplesUpdateNode shows the source above the image.
//
//line server.gw:292
//line server.gw:293
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
