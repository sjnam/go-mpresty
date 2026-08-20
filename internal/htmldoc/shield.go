package htmldoc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// shield replaces the body of every rawTag element with a token, returning the
// rewritten source and the bodies keyed by token. The token is alphanumeric so
// that neither the tokenizer nor the entity decoder can alter it.
func shield(src string, rawTags []string) (string, map[string]string) {
	if len(rawTags) == 0 || !strings.Contains(src, "<") {
		return src, nil
	}
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return src, nil
	}
	prefix := "mprestyraw" + hex.EncodeToString(seed[:])

	bodies := make(map[string]string)
	for _, tag := range rawTags {
		re := regexp.MustCompile(`(?is)(<` + regexp.QuoteMeta(tag) + `\b[^<>]*>)(.*?)(</` + regexp.QuoteMeta(tag) + `\s*>)`)
		src = re.ReplaceAllStringFunc(src, func(m string) string {
			g := re.FindStringSubmatch(m)
			if g[2] == "" {
				return m
			}
			token := fmt.Sprintf("%s%d", prefix, len(bodies))
			bodies[token] = g[2]
			return g[1] + token + g[3]
		})
	}
	return src, bodies
}

// restore walks the parsed tree and swaps each token back for its raw body.
func restore(n *Node, bodies map[string]string) {
	Walk(n, func(n *Node) {
		if n.Type != TextNode {
			return
		}
		if body, ok := bodies[strings.TrimSpace(n.Data)]; ok {
			n.Data = body
		}
	})
}
