GO ?= go
TEMPL ?= $(shell command -v templ 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/templ)
TAILWIND ?= ./bin/tailwindcss

.PHONY: generate css build run test vet lint fmt tools clean

generate:
	$(TEMPL) generate

css:
	$(TAILWIND) -i ./assets/css/input.css -o ./assets/css/output.css --minify

build: generate css
	$(GO) build ./...

run: build
	$(GO) run ./cmd/readmecheck

test: generate
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint: vet

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './components/*' -not -path './utils/*')
	$(TEMPL) fmt .

tools:
	$(GO) install github.com/a-h/templ/cmd/templ@latest
	$(GO) install github.com/templui/templui/cmd/templui@latest

clean:
	rm -f readmecheck
