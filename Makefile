BINARY_NAME = compterm
VERSION ?= $(shell git describe --tags --always)
DIST_DIR ?= dist
LDFLAGS = -X 'main.Version=$(VERSION)' -w -s
export CGO_ENABLED=0

all: js
	go build -trimpath -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" .

dev-race: js-dev
	go run -race -tags dev .

dev: js-dev
	go run -tags dev .

# Reinstall whenever package.json changes, so a dependency bump (e.g. a new
# xterm version) actually reaches the bundle instead of using stale node_modules.
node_modules: package.json
	npm install --legacy-peer-deps
	@touch node_modules

js-deps:
	npm clean-install --legacy-peer-deps

js-dev: node_modules
	npx esbuild assets/term.js --outfile=assets/term.min.js --bundle --sourcemap

js: node_modules
	npx esbuild assets/term.js --outfile=assets/term.min.js --bundle --minify

js-clean:
	rm -rf assets/term.min.js* node_modules

# Release artifacts, invoked by release.sh as `make dist VERSION=... DIST_DIR=...`.
# The bundle must be built first because the binary embeds it. Windows is not a
# target: creack/pty and SIGWINCH are Unix-only.
dist: js
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 .
	gzip -f $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(DIST_DIR)/$(BINARY_NAME)-linux-arm64

# Static-analysis and test gate (the house QA flow).
# Note: go fix rewrites code in place to adopt new idioms — review its changes.
# G115 (integer-overflow conversions) is excluded: the only hits are safe fd and
# SGR color-byte conversions. The race detector needs cgo, so it overrides the
# CGO_ENABLED=0 set above.
check:
	go fix ./...
	go fix -inline ./...
	modernize -fix ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needs to run on:"; gofmt -l .; exit 1; }
	go vet ./...
	golangci-lint run ./...
	gosec -quiet -exclude=G115 ./...
	staticcheck ./...
	golangci-lint run --no-config --default=none --enable=gocognit --tests=false ./...
	CGO_ENABLED=1 go test -race -count 1 -timeout 30s ./...

clean: js-clean
	go clean
	rm -f $(BINARY_NAME)
	rm -rf $(DIST_DIR)

.PHONY: all dev dev-race clean check js js-dev js-clean js-deps dist
