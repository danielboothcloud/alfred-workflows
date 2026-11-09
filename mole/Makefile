# Mole - Alfred workflow

BINARY      := alfred-mole
WORKFLOW    := workflow
VERSION     := $(shell /usr/libexec/PlistBuddy -c 'Print :version' $(WORKFLOW)/info.plist)
PACKAGE     := mole-$(VERSION).alfredworkflow
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath

export CGO_ENABLED := 0

.PHONY: all build universal icons package install test vet list refresh clean help

all: test package

## build: build the binary for this Mac
build:
	go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY) .

## universal: build an arm64 + amd64 binary
universal:
	GOARCH=arm64 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-arm64 .
	GOARCH=amd64 go build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(WORKFLOW)/$(BINARY)-amd64 .
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

## refresh: rebuild the cached app list (runs mo uninstall --list)
refresh: build
	$(WORKFLOW)/$(BINARY) refresh

## list: print the cached app list and the name passed to mo uninstall
list: build
	$(WORKFLOW)/$(BINARY) list

clean:
	rm -f $(WORKFLOW)/$(BINARY) *.alfredworkflow

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
