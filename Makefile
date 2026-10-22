GO ?= go
TEMPL_VERSION ?= v0.3.1070
TEMPLUI_VERSION ?= v1.13.2
TAILWIND_VERSION ?= v4.3.3
TEMPL ?= $(shell command -v templ 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/templ)
TAILWIND ?= ./bin/tailwindcss
TAILWIND_OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
TAILWIND_ARCH ?= $(shell uname -m | sed 's/x86_64/x64/; s/aarch64/arm64/; s/arm64/arm64/')
TAILWIND_ASSET ?= tailwindcss-$(TAILWIND_OS)-$(TAILWIND_ARCH)
TAILWIND_URL ?= https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/$(TAILWIND_ASSET)

.PHONY: generate css build run test vet lint fmt tools templ templui tailwind clean

generate:
	$(TEMPL) generate

css: $(TAILWIND)
	$(TAILWIND) -i ./assets/css/input.css -o ./assets/css/output.css --minify

$(TAILWIND):
	mkdir -p $(dir $(TAILWIND))
	curl -fsSL "$(TAILWIND_URL)" -o $(TAILWIND)
	chmod +x $(TAILWIND)

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

tools: templ templui tailwind

templ:
	$(GO) install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)

templui:
	$(GO) install github.com/templui/templui/cmd/templui@$(TEMPLUI_VERSION)

tailwind: $(TAILWIND)

clean:
	rm -f readmecheck
