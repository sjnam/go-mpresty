package render

import (
	"os/exec"
	"slices"
)

// Renderer describes one graphics engine. Every field is read-only once the
// Renderer is registered, so a single value is shared safely across requests.
type Renderer struct {
	Tag       string   // custom element name, e.g. "metapost"
	Ext       string   // source file extension, e.g. "mp"
	Cmd       string   // default command
	Commands  []string // commands a cmd="..." attribute may select
	Preamble  string
	Postamble string

	// Steps returns the argv of each command to run inside a job directory
	// that already holds fname.Ext. The last step must leave fname.svg there.
	//
	// Unlike the Lua original these are argv slices run without a shell, so a
	// cmd attribute cannot inject anything; Commands bounds it further.
	Steps func(cmd, fname string) [][]string
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
	Steps: func(cmd, fname string) [][]string {
		return [][]string{{cmd, "-interaction=nonstopmode", fname + ".mp"}}
	},
}

// pdfConverters are the PDF-to-SVG tools the <tikz> pipeline can finish with,
// in preference order. The Lua version hard-coded pdf2svg; here the first one
// present on PATH is used, so a TeX Live install alone is often enough.
//
// dvisvgm is listed last because its --pdf mode delegates to MuPDF, and a
// dvisvgm built without it fails only at run time.
var pdfConverters = []struct {
	name string
	args func(fname string) []string
}{
	{"pdf2svg", func(f string) []string { return []string{f + ".pdf", f + ".svg"} }},
	{"pdftocairo", func(f string) []string { return []string{"-svg", f + ".pdf", f + ".svg"} }},
	{"mutool", func(f string) []string { return []string{"draw", "-o", f + ".svg", f + ".pdf", "1"} }},
	{"dvisvgm", func(f string) []string { return []string{"--pdf", f + ".pdf", "-o", f + ".svg"} }},
}

// PDFConverters names the tools Tikz can use, for diagnostics.
func PDFConverters() []string {
	names := make([]string, len(pdfConverters))
	for i, c := range pdfConverters {
		names[i] = c.name
	}
	return names
}

// Tikz builds the renderer for <tikz> elements and .tex sources.
//
// converter names the PDF-to-SVG tool to finish with; an empty string picks
// the first of PDFConverters found on PATH, falling back to the first name so
// that a missing tool surfaces as a clear error on the page.
func Tikz(converter string) *Renderer {
	conv := pdfConverters[0]
	if converter == "" {
		for _, c := range pdfConverters {
			if _, err := exec.LookPath(c.name); err == nil {
				conv = c
				break
			}
		}
	} else {
		for _, c := range pdfConverters {
			if c.name == converter {
				conv = c
				break
			}
		}
	}
	return &Renderer{
		Tag:      "tikz",
		Ext:      "tex",
		Cmd:      "pdflatex",
		Commands: []string{"pdflatex", "lualatex", "xelatex"},
		Preamble: `\documentclass[tikz]{standalone}
\usepackage{pgfplots}
\usepackage{xifthen}
\usetikzlibrary{arrows, backgrounds, calc, intersections}
\usetikzlibrary{matrix, mindmap, positioning, shapes}
\begin{document}
`,
		Postamble: "\n\\end{document}\n",
		Steps: func(cmd, fname string) [][]string {
			return [][]string{
				{cmd, "-interaction=nonstopmode", "-halt-on-error", fname + ".tex"},
				append([]string{conv.name}, conv.args(fname)...),
			}
		},
	}
}

// Graphviz renders <graphviz> elements and .gv sources.
var Graphviz = &Renderer{
	Tag:      "graphviz",
	Ext:      "gv",
	Cmd:      "dot",
	Commands: []string{"dot", "neato", "fdp", "sfdp", "twopi", "circo", "osage", "patchwork"},
	Steps: func(cmd, fname string) [][]string {
		return [][]string{{cmd, "-Tsvg", fname + ".gv", "-o", fname + ".svg"}}
	},
}

// Default is the set of renderers mpresty shipped with.
func Default() []*Renderer { return []*Renderer{MetaPost, Tikz(""), Graphviz} }
