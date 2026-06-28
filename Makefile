.PHONY: all build clean fmt fmt-check install lint test vet

GO ?= go
BINARY ?= treetop

all: build

build:
	$(GO) build -o $(BINARY) .

clean:
	rm -f $(BINARY)

fmt:
	gofmt -w .

fmt-check:
	@files="$$(gofmt -l .)"; [ -z "$$files" ] || { printf '%s\n' "$$files"; exit 1; }

install:
	$(GO) install .

lint: fmt-check vet

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
