package render

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// image returns the URI of the SVG for this source, rendering it if needed.
//
// The file name is the command plus a CRC-32 of the source, exactly as the Lua
// version composed it, so an existing images directory stays valid across the
// port. The rendered file is itself the cache: it outlives the process, and
// asking the filesystem cannot go stale the way the Lua shared dict did when
// someone emptied the images directory. Identical concurrent requests collapse
// into a single engine run.
func (p *Processor) image(ctx context.Context, r *Renderer, cmd, content string, useCache bool) (string, error) {
	fname := cmd + strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(content))), 10)
	uri := path.Join("/", p.workDir, fname+".svg")

	if useCache && p.rendered(fname) {
		return uri, nil
	}
	_, err, _ := p.group.Do(r.Tag+fname, func() (any, error) {
		if useCache && p.rendered(fname) {
			return nil, nil
		}
		return nil, p.build(ctx, r, cmd, fname, content)
	})
	if err != nil {
		return "", err
	}
	return uri, nil
}

// rendered reports whether the SVG for this name is already on disk.
func (p *Processor) rendered(fname string) bool {
	_, err := os.Stat(p.svgPath(fname))
	return err == nil
}

func (p *Processor) svgPath(fname string) string {
	return filepath.Join(p.root, filepath.FromSlash(p.workDir), fname+".svg")
}

// inputs is the kpathsea search path for a job: its own directory first, then
// the document root recursively, then whatever the installation provides.
func (p *Processor) inputs(dir string) string {
	sep := string(filepath.ListSeparator)
	return strings.Join([]string{dir, p.root + string(filepath.Separator) + string(filepath.Separator), ""}, sep)
}

// build runs the engine in a scratch directory of its own and moves the
// finished SVG into the work directory.
//
// Isolation matters more here than it did in Lua: MetaPost's makempx writes
// fixed-name temporary files, and TeX writes .aux and .log beside the source,
// so genuinely parallel runs sharing one directory would collide.
func (p *Processor) build(ctx context.Context, r *Renderer, cmd, fname, content string) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	dir, err := os.MkdirTemp(p.tmpDir, "job-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	src := r.Preamble + content + r.Postamble
	if err := os.WriteFile(filepath.Join(dir, fname+"."+r.Ext), []byte(src), 0o644); err != nil {
		return err
	}

	for _, argv := range r.Steps(cmd, fname) {
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Dir = dir
		// Engines must never wait on a terminal, and TeX in particular starts
		// looking for input files wherever it pleases unless told otherwise.
		c.Stdin = nil
		c.Env = append(os.Environ(),
			"TEXMFOUTPUT="+dir,
			"openout_any=p",
			// A source may `input` a sibling, as benchmark.html's bird.mp
			// inputs courbes.mp. The Lua version got that for free by running
			// in the images directory; an isolated job needs a search path.
			// The trailing separator keeps the system defaults.
			"MPINPUTS="+p.inputs(dir),
			"TEXINPUTS="+p.inputs(dir),
		)
		var out bytes.Buffer
		c.Stdout, c.Stderr = &out, &out
		if err := c.Run(); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("%s: %w", argv[0], ctx.Err())
			}
			return fmt.Errorf("%s: %w\n%s", argv[0], err, diagnostics(dir, fname, out.String()))
		}
	}

	svg := filepath.Join(dir, fname+".svg")
	if _, err := os.Stat(svg); err != nil {
		return fmt.Errorf("%s produced no %s.svg", cmd, fname)
	}
	return os.Rename(svg, p.svgPath(fname))
}

// diagnostics trims engine chatter down to something worth putting on a page,
// preferring the tail where TeX and MetaPost report the actual error.
func diagnostics(dir, fname, out string) string {
	if strings.TrimSpace(out) == "" {
		if b, err := os.ReadFile(filepath.Join(dir, fname+".log")); err == nil {
			out = string(b)
		}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	const keep = 30
	if len(lines) > keep {
		lines = append([]string{"..."}, lines[len(lines)-keep:]...)
	}
	return strings.Join(lines, "\n")
}
