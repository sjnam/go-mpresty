package render

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sjnam/go-mpresty/internal/htmldoc"
)

// maxSourceBytes bounds a remote graphics source.
const maxSourceBytes = 4 << 20

// contents yields the graphics source for a node: the body of a src attribute
// if it has one, else its own text.
func (p *Processor) contents(ctx context.Context, node *htmldoc.Node, base string, useCache bool) (string, error) {
	uri, ok := htmldoc.Attr(node, "src")
	if !ok {
		return htmldoc.TextContent(node), nil
	}
	htmldoc.RemoveAttr(node, "src")

	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return p.fetch(ctx, uri, useCache)
	}
	// A local source is read straight from the document root. The Lua version
	// went back through nginx with ngx.location.capture, which confined the
	// path for it; reading the filesystem directly means doing that here.
	name, err := p.resolve(uri, base)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", uri, err)
	}
	return string(b), nil
}

// resolve maps a document-relative or absolute URI to a path inside the
// document root, refusing anything that would escape it.
func (p *Processor) resolve(uri, base string) (string, error) {
	if !strings.HasPrefix(uri, "/") {
		uri = base + uri
	}
	clean := path.Clean(uri)
	name := filepath.Join(p.root, filepath.FromSlash(clean))
	if name != p.root && !strings.HasPrefix(name, p.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the document root", uri)
	}
	return name, nil
}

// fetch retrieves a remote source, caching the body as the Lua version did.
func (p *Processor) fetch(ctx context.Context, uri string, useCache bool) (string, error) {
	key := "src:" + uri
	if useCache {
		if body, ok := p.cache.Get(key); ok {
			return body, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return "", err
	}
	res, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: %s", uri, res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxSourceBytes))
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", uri, err)
	}
	body := string(b)
	p.cache.Set(key, body)
	return body, nil
}
