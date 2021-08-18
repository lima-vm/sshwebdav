# Files are installed under $(DESTDIR)/$(PREFIX)
PREFIX ?= /usr/local
DEST := $(shell echo "$(DESTDIR)/$(PREFIX)" | sed 's:///*:/:g; s://*$$::')

GO ?= go

TAR ?= tar

PACKAGE := github.com/lima-vm/sshwebdav

VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
VERSION_TRIMMED := $(VERSION:v%=%)

GO_BUILD := CGO_ENABLED=0 $(GO) build -ldflags="-s -w -X $(PACKAGE)/pkg/version.Version=$(VERSION)"

.PHONY: all
all: binaries

.PHONY: binaries
binaries: _output/bin/sshwebdav

.PHONY: _output/bin/sshwebdav
_output/bin/sshwebdav:
	$(GO_BUILD) -o $@ ./cmd/sshwebdav

.PHONY: install
install:
	mkdir -p "$(DEST)"
	cp -av _output/* "$(DEST)"

.PHONY: uninstall
uninstall:
	rm -rf "$(DEST)/bin/sshwebdav"

.PHONY: clean
clean:
	rm -rf _output

.PHONY: artifacts
artifacts:
	mkdir -p _artifacts
	GOOS=darwin GOARCH=amd64 make clean binaries
	$(TAR) -C _output/ -czvf _artifacts/sshwebdav-$(VERSION_TRIMMED)-Darwin-x86_64.tar.gz ./
	GOOS=darwin GOARCH=arm64 make clean binaries
	$(TAR) -C _output -czvf _artifacts/sshwebdav-$(VERSION_TRIMMED)-Darwin-arm64.tar.gz ./
	GOOS=linux GOARCH=amd64 make clean binaries
	$(TAR) -C _output/ -czvf _artifacts/sshwebdav-$(VERSION_TRIMMED)-Linux-x86_64.tar.gz ./
	GOOS=linux GOARCH=arm64 make clean binaries
	$(TAR) -C _output/ -czvf _artifacts/sshwebdav-$(VERSION_TRIMMED)-Linux-aarch64.tar.gz ./
