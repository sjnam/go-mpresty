# The Go sources are literate programs: each package directory holds one .gw
# file, and its .go and _test.go are tangled from it. The tangled files are
# committed all the same, so that `go build`, `go install` and the Dockerfile
# work without GWEB. Edit the .gw, never the .go, and run `make tangle`.
#
#   make tangle   .gw -> .go and _test.go
#   make doc      typeset each .gw as a .pdf next to it
#   make build    the mpresty binary (does not tangle)
#   make test     go test ./...

BIN  := mpresty
ROOT := webapp/html

GTANGLE ?= gtangle
GWEAVE  ?= gweave
LUATEX  ?= luatex --interaction=nonstopmode
WARNS   := 'Overfull\|Underfull\|Error\|Missing\|Undefined'

# directory:web name
WEBS := cmd/mpresty:mpresty internal/cache:cache internal/htmldoc:htmldoc \
        internal/render:render internal/server:server

.PHONY: all tangle doc build test race vet run clean docker

all: build

tangle:
	@for w in $(WEBS); do d=$${w%%:*}; n=$${w##*:}; \
	  (cd $$d && $(GTANGLE) $$n.gw >/dev/null) || exit 1; done

# A fatal TeX error leaves only a lowercase `error' in the log, which WARNS
# does not catch, so check that the PDF was made as well.
doc:
	@for w in $(WEBS); do d=$${w%%:*}; n=$${w##*:}; \
	  (cd $$d && rm -f $$n.pdf && $(GWEAVE) $$n.gw >/dev/null && \
	   $(LUATEX) $$n.tex >/dev/null </dev/null; \
	   if [ -s $$n.pdf ]; then echo "$$d/$$n.pdf: `grep -ac $(WARNS) $$n.log` warnings"; \
	   else echo "$$d/$$n.pdf: NOT PRODUCED (see $$d/$$n.log)"; exit 1; fi) || exit 1; \
	done

build:
	go build -o $(BIN) ./cmd/mpresty

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

run: build
	./$(BIN) -root $(ROOT)

clean:
	rm -rf $(BIN) $(ROOT)/images
	@for w in $(WEBS); do d=$${w%%:*}; n=$${w##*:}; \
	  rm -f $$d/$$n.tex $$d/$$n.log $$d/$$n.toc $$d/$$n.idx $$d/$$n.scn $$d/$$n.pdf; done

docker:
	docker build -t mpresty:latest .
