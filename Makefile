.PHONY: all build clean test fmt tidy run

all: fmt build

build:
	go build -o gork ./cmd/gork

clean:
	rm -f gork

test:
	go test -v ./internal/htparser/...

fmt:
	go fmt ./...

tidy:
	go mod tidy

run: build
	./gork -url http://localhost/pixel.gif -c 10 -d 3s
