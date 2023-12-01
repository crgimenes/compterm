BINARY_NAME = compterm
GIT_TAG = $(shell git describe --tags --always)
LDFLAGS = -X 'main.GitTag=$(GIT_TAG)'
EXTLDFLAGS = -static -w -s

all:
	go build -o $(BINARY_NAME) -ldflags "$(LDFLAGS) -extldflags '$(EXTLDFLAGS)'" . 

dev:
	go run -tags dev . 

clean:
	go clean
	rm -f $(BINARY_NAME)

.PHONY: all dev clean
