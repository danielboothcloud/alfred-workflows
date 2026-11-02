# App Launcher - Alfred workflow

BINARY      := alfred-apps
WORKFLOW    := workflow
VERSION     := $(shell /usr/libexec/PlistBuddy -c 'Print :version' $(WORKFLOW)/info.plist)
PACKAGE     := app-launcher-$(VERSION).alfredworkflow
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath

# cgo is required: Foundation provides the localised app names Finder shows.
export CGO_ENABLED := 1

.PHONY: all build universal icons package install test vet list clean help

all: test package

## build: build the binary for this Mac
build:
	go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY) .

## universal: build an arm64 + amd64 binary
universal:
	GOARCH=arm64 CC='clang -arch arm64' go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-arm64 .
	GOARCH=amd64 CC='clang -arch x86_64' go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-amd64 .
	lipo -create -output $(WORKFLOW)/$(BINARY) $(WORKFLOW)/$(BINARY)-arm64 $(WORKFLOW)/$(BINARY)-amd64
	rm $(WORKFLOW)/$(BINARY)-arm64 $(WORKFLOW)/$(BINARY)-amd64

## icons: regenerate the workflow icon
icons:
	go run ./tools/genicons $(WORKFLOW)

## package: build and zip the .alfredworkflow
package: build
	plutil -lint $(WORKFLOW)/info.plist
	rm -f $(PACKAGE)
	cd $(WORKFLOW) && zip -qr ../$(PACKAGE) info.plist icon.png $(BINARY)
	@echo "Created $(PACKAGE)"

## install: package and open in Alfred
install: package
	open $(PACKAGE)

## test: run unit tests
test: vet
	go test ./...

vet:
	go vet ./...

## list: print every indexed app (rebuilds the index)
list: build
	$(WORKFLOW)/$(BINARY) reindex

clean:
	rm -f $(WORKFLOW)/$(BINARY) *.alfredworkflow

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
