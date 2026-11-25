FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The Go sources are tangled from the .gw literate programs, by the gtangle
# that go.mod pins as a tool.
RUN for w in $(find cmd internal -name '*.gw'); do \
      (cd "$(dirname "$w")" && go tool gtangle "$(basename "$w")") || exit 1; \
    done
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mpresty ./cmd/mpresty

FROM ubuntu:resolute

LABEL maintainer="Soojin Nam <jsunam@gmail.com>"

# The graphics engines, from Ubuntu 26.04 LTS: MetaPost from TeX Live 2025 and
# Graphviz 14. No LaTeX: btex labels are typeset by the plain eTeX that
# texlive-metapost already carries, so the samples say `%&plain` where they
# once said `%&latex`.
#
# Graphviz 14 ships only dot in the graphviz package; neato, fdp, sfdp, twopi
# and circo live in the neato-layout plugin, which nothing depends on.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get --no-install-recommends -y install \
       texlive-metapost \
       graphviz libgvplugin-neato-layout8 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/mpresty /usr/local/bin/mpresty
COPY webapp/html /var/www/html

EXPOSE 8080
ENTRYPOINT ["mpresty"]
CMD ["-addr", ":8080", "-root", "/var/www/html"]
