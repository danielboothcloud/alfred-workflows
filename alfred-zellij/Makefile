# Zellij Sessions - Alfred workflow

BINARY      := alfred-zellij
WORKFLOW    := workflow
VERSION     := $(shell /usr/libexec/PlistBuddy -c 'Print :version' $(WORKFLOW)/info.plist)
PACKAGE     := zellij-sessions-$(VERSION).alfredworkflow
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath

.PHONY: all build universal icons package install test vet terminals clean help

all: test package

## build: build the binary for this Mac
build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY) .

## universal: build an arm64 + amd64 binary
universal:
	CGO_ENABLED=0 GOARCH=arm64 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-arm64 .
	CGO_ENABLED=0 GOARCH=amd64 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-amd64 .
	lipo -create -output $(WORKFLOW)/$(BINARY) $(WORKFLOW)/$(BINARY)-arm64 $(WORKFLOW)/$(BINARY)-amd64
	rm $(WORKFLOW)/$(BINARY)-arm64 $(WORKFLOW)/$(BINARY)-amd64

## icons: regenerate PNG icons
icons:
	go run ./tools/genicons $(WORKFLOW)

## package: build and zip the .alfredworkflow
package: build
	plutil -lint $(WORKFLOW)/info.plist
	rm -f $(PACKAGE)
	cd $(WORKFLOW) && zip -qr ../$(PACKAGE) info.plist icon.png icons $(BINARY)
	@echo "Created $(PACKAGE)"

## install: package and open in Alfred
install: package
	open $(PACKAGE)

## test: run unit tests
test: vet
	go test ./...

vet:
	go vet ./...

## terminals: debug Ghostty terminal → session mapping
terminals: build
	$(WORKFLOW)/$(BINARY) terminals

clean:
	rm -f $(WORKFLOW)/$(BINARY) *.alfredworkflow

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
