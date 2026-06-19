BINARY_NAME = compterm
GIT_TAG = $(shell git describe --tags --always)
LDFLAGS = -X 'main.GitTag=$(GIT_TAG)' -w -s
export CGO_ENABLED=0

all: js
	go build -trimpath -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" .

dev-race: js-dev
	go run -race -tags dev .

dev: js-dev
	go run -tags dev .

js-deps:
	npm clean-install --legacy-peer-deps

js-dev:
	npx esbuild assets/term.js --outfile=assets/term.min.js --bundle --sourcemap

js:
	bash -c '[[ -d node_modules ]] || make js-deps'
	npx esbuild assets/term.js --outfile=assets/term.min.js --bundle --minify

js-clean:
	rm -rf assets/term.min.js* node_modules


intel:
	GOOS=linux GOARCH=amd64 go build -trimpath -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" .

# Static-analysis and test gate (mirrors the eprojects verification flow).
# Note: go fix rewrites code in place to adopt new idioms — review its changes.
# G115 (integer-overflow conversions) is excluded: the only hits are safe fd and
# SGR color-byte conversions. The race detector needs cgo, so it overrides the
# CGO_ENABLED=0 set above.
check:
	go fix ./...
	go fix -inline ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needs to run on:"; gofmt -l .; exit 1; }
	go vet ./...
	gosec -quiet -exclude=G115 ./...
	CGO_ENABLED=1 go test -race ./...

clean: js-clean
	go clean
	rm -f $(BINARY_NAME)

.PHONY: all dev dev-race clean check js js-dev js-clean js-deps intel

