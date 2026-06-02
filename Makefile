BINARY  = dropserve
VERSION = 0.1.0
CMD     = ./cmd/dropserve
DIST    = dist
LDFLAGS = -ldflags "-s -w -X 'main.version=$(VERSION)'"

.PHONY: build test dist clean

## build: compile for the current platform
build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

## test: run the test suite
test:
	go test ./...

## dist: cross-compile for all supported platforms into dist/
dist: clean
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-amd64       $(CMD)
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-arm64       $(CMD)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-amd64      $(CMD)
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-arm64      $(CMD)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-windows-amd64.exe $(CMD)
	@echo ""
	@echo "Built for 5 platforms:"
	@ls -lh $(DIST)/

## clean: remove build artifacts
clean:
	rm -rf $(DIST) $(BINARY) $(BINARY).exe

## help: show this message
help:
	@grep -E '^##' Makefile | sed 's/## /  /'
