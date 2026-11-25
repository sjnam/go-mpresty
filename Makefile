# The Go sources are literate programs: each package directory holds one .gw
# file, and its .go and _test.go are tangled from it. Only the .gw files are
# committed; the Go files are generated here and in the Dockerfile.
#
# gtangle comes from go.mod's tool directive, so the version is pinned and
# nothing but Go is needed to build. gweave and LuaTeX are needed only for
# `make doc`.
#
#   make tangle   .gw -> .go and _test.go
#   make doc      typeset each .gw as a .pdf next to it
#   make build    the mpresty binary
#   make test     go test ./...
#   make tidy     go mod tidy, on tangled sources
#
# Never run a bare `go mod tidy` before `make tangle`: with no Go files to
# read, tidy drops every requirement from go.mod, and the Docker build fails.

BIN  := mpresty
ROOT := webapp/html

GTANGLE ?= go tool gtangle
GWEAVE  ?= gweave
LUATEX  ?= luatex --interaction=nonstopmode
WARNS   := 'Overfull\|Underfull\|Error\|Missing\|Undefined'

WEBS := cmd/mpresty/mpresty internal/cache/cache internal/htmldoc/htmldoc \
        internal/render/render internal/server/server
GOSRC := $(addsuffix .go,$(WEBS))

.PHONY: all tangle doc build test race vet tidy run clean docker

all: build

tangle: $(GOSRC)

# gtangle writes the _test.go beside the .go, from the same .gw. It runs in
# the web's directory so that the //line comments name the .gw alone.
%.go: %.gw
	cd $(@D) && $(GTANGLE) $(<F)

# A fatal TeX error leaves only a lowercase `error' in the log, which WARNS
# does not catch, so check that the PDF was made as well.
doc:
	@for w in $(WEBS); do d=$${w%/*}; n=$${w##*/}; \
	  (cd $$d && rm -f $$n.pdf && $(GWEAVE) $$n.gw >/dev/null && \
	   $(LUATEX) $$n.tex >/dev/null </dev/null; \
	   if [ -s $$n.pdf ]; then echo "$$d/$$n.pdf: `grep -ac $(WARNS) $$n.log` warnings"; \
	   else echo "$$d/$$n.pdf: NOT PRODUCED (see $$d/$$n.log)"; exit 1; fi) || exit 1; \
	done

build: tangle
	go build -o $(BIN) ./cmd/mpresty

test: tangle
	go test ./...

race: tangle
	go test -race ./...

vet: tangle
	go vet ./...

tidy: tangle
	go mod tidy

run: build
	./$(BIN) -root $(ROOT)

# Every Go file under cmd/ and internal/ is tangled, including the extra
# files a web writes with @(...@>=, such as render's confine_linux.go.
clean:
	rm -rf $(BIN) $(ROOT)/images
	find cmd internal -name '*.go' -delete
	@for w in $(WEBS); do \
	  rm -f $$w.tex $$w.log $$w.toc $$w.idx $$w.scn $$w.pdf; done

docker:
	docker build -t mpresty:latest .
