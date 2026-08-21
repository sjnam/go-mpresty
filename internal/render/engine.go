package render

import "slices"

// Renderer describes one graphics engine. Every field is read-only once the
// Renderer is registered, so a single value is shared safely across requests.
type Renderer struct {
	Tag       string   // custom element name, e.g. "metapost"
	Ext       string   // source file extension, e.g. "mp"
	Cmd       string   // default command
	Commands  []string // commands a cmd="..." attribute may select
	Preamble  string
	Postamble string

	// Command returns the argv to run inside a job directory that already
	// holds fname.Ext. It must leave fname.svg there.
	//
	// Unlike the Lua original this is an argv slice run without a shell, so a
	// cmd attribute cannot inject anything; Commands bounds it further.
	Command func(cmd, fname string) []string
}

// allows reports whether cmd may be used with this renderer.
func (r *Renderer) allows(cmd string) bool {
	return cmd == r.Cmd || slices.Contains(r.Commands, cmd)
}

// MetaPost renders <metapost> elements and .mp sources.
var MetaPost = &Renderer{
	Tag:      "metapost",
	Ext:      "mp",
	Cmd:      "mpost",
	Commands: []string{"mpost", "mpost-lua"},
	Preamble: "prologues:=3;\noutputtemplate:=\"%j.svg\";\noutputformat:=\"svg\";\n",
	// The original appended "end"; keep it, on its own line so a source that
	// ends in a comment still terminates.
	Postamble: "\nend\n",
	Command: func(cmd, fname string) []string {
		return []string{cmd, "-interaction=nonstopmode", fname + ".mp"}
	},
}

// Graphviz renders <graphviz> elements and .gv sources.
var Graphviz = &Renderer{
	Tag:      "graphviz",
	Ext:      "gv",
	Cmd:      "dot",
	Commands: []string{"dot", "neato", "fdp", "sfdp", "twopi", "circo", "osage", "patchwork"},
	Command: func(cmd, fname string) []string {
		return []string{cmd, "-Tsvg", fname + ".gv", "-o", fname + ".svg"}
	},
}

// Default is the set of renderers used when none are configured.
func Default() []*Renderer { return []*Renderer{MetaPost, Graphviz} }
