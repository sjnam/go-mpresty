//line server.gw:320
package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjnam/go-mpresty/internal/render"
)

//line server.gw:339
func newServer(t *testing.T, files map[string]string) http.Handler {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {

//line server.gw:355
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

//line server.gw:344
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

//line server.gw:364
	fake := &render.Renderer{
		Tag:      "fake",
		Ext:      "txt",
		Cmd:      "cp",
		Commands: []string{"cp"},
		Command: func(cmd, fname string) []string {
			return []string{"cp", fname + ".txt", fname + ".svg"}
		},
	}
	opts := render.Options{Root: root, Renderers: []*render.Renderer{fake}, Logger: log}
	proc, err := render.New(opts)
	if err != nil {
		t.Fatal(err)
	}

//line server.gw:347
	s, err := New(root, proc, log)
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

//line server.gw:381
func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

//line server.gw:391
func TestServesRenderedPage(t *testing.T) {
	h := newServer(t, map[string]string{
		"page.html": `<html><body><fake width="70">hi</fake></body></html>`,
	})
	rec := get(t, h, "/page.html")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<img width="70" src="/images/cp`) {
		t.Errorf("body = %q", body)
	}
	if strings.Contains(body, "<fake") {
		t.Error("the source element survived into the output")
	}
}

//line server.gw:414
func TestServesStaticFilesUntouched(t *testing.T) {
	h := newServer(t, map[string]string{"source/a.txt": "<fake>not html</fake>"})
	rec := get(t, h, "/source/a.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != "<fake>not html</fake>" {
		t.Errorf("body = %q, want it served verbatim", got)
	}
}

//line server.gw:427
func TestMissingPageIs404(t *testing.T) {
	h := newServer(t, nil)
	if rec := get(t, h, "/nope.html"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDirectoryServesIndex(t *testing.T) {
	h := newServer(t, map[string]string{"docs/index.html": `<p><fake>x</fake></p>`})
	rec := get(t, h, "/docs/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<img") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

//line server.gw:449
func TestBenchmarkAppendsElapsed(t *testing.T) {
	h := newServer(t, map[string]string{"benchmark.html": `<p><fake>x</fake></p>`})
	rec := get(t, h, "/benchmark.html")
	if !strings.Contains(rec.Body.String(), "<p>elapsed seconds: ") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestUpdatenodeUsesPerTagUpdaters(t *testing.T) {
	h := newServer(t, map[string]string{"updatenode.html": `<p><fake>x</fake></p>`})
	if rec := get(t, h, "/updatenode.html"); !strings.Contains(rec.Body.String(), "<img") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

//line server.gw:467
func TestPreviewRendersPostedCode(t *testing.T) {
	h := newServer(t, nil)
	form := url.Values{"gx": {"fake"}, "cmd": {"cp"}, "code": {"if a<b: draw x;"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview.gxn", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<img width="400" src="/images/cp`) {
		t.Errorf("body = %q", rec.Body.String())
	}
}

//line server.gw:484
func TestPreviewRejectsUnknownTag(t *testing.T) {
	h := newServer(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview.gxn", strings.NewReader("gx=evil&code=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
