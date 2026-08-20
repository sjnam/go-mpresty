package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sjnam/go-mpresty/internal/render"
)

// preview renders a snippet posted from preview.html. It builds a one-element
// document around the submitted code and runs it through the same pipeline,
// which is what preview.lua did.
func (s *Server) preview() http.HandlerFunc {
	allowed := make(map[string]bool, len(s.rawTags))
	for _, tag := range s.rawTags {
		allowed[tag] = true
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "400 bad request", http.StatusBadRequest)
			return
		}
		gx := r.PostFormValue("gx")
		if !allowed[gx] {
			http.Error(w, "400 unknown graphics tag", http.StatusBadRequest)
			return
		}
		var cmd string
		if c := r.PostFormValue("cmd"); c != "" {
			// The value reaches the engine as an argv element, never a shell
			// word, and the renderer's own allow-list has the final say.
			cmd = fmt.Sprintf(" cmd=%q", c)
		}
		html := strings.Join([]string{
			"<", gx, cmd, " cache='no' width='400'>\n",
			r.PostFormValue("code"),
			"\n</", gx, ">",
		}, "")

		out, err := s.render(r, []byte(html), "/preview.gxn", render.Updaters{})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(out)
	}
}
