BINARY  := claudius-maximus
ALIAS   := cmax
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test vet check clean install

build:
	go build $(LDFLAGS) -o $(BINARY) .
	ln -sf $(BINARY) $(ALIAS)

test:
	go test ./...

vet:
	go vet ./...

check: vet test

# Cross-compile smoke test: catches build-tag mistakes in the platform-specific
# process handling before they reach CI.
crosscompile:
	GOOS=linux   GOARCH=amd64 go build -o /dev/null .
	GOOS=darwin  GOARCH=arm64 go build -o /dev/null .
	GOOS=windows GOARCH=amd64 go build -o /dev/null .

clean:
	rm -f $(BINARY) $(ALIAS)
