BIN  := mpresty
ROOT := webapp/html

.PHONY: all build test race vet run clean docker

all: build

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

docker:
	docker build -t mpresty:latest .
