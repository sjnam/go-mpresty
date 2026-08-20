package htmldoc

import (
	"io"
	"strings"
)

func stringReader(s string) io.Reader { return strings.NewReader(s) }

func contains(s, sub string) bool { return strings.Contains(s, sub) }
