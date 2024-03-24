BINARY_NAME = compterm
GIT_TAG = $(shell git describe --tags --always)
LDFLAGS = -X 'main.GitTag=$(GIT_TAG)' -w -s
export CGO_ENABLED=0

all: js
	go build -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" .

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
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" .

clean: js-clean
	go clean
	rm -f $(BINARY_NAME)

.PHONY: all dev clean js js-dev js-clean js-deps

