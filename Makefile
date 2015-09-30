# Set an output prefix, which is the local directory if not specified
PREFIX?=$(shell pwd)


# Used to populate version variable in main package.
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always)

# Allow turning off function inlining and variable registerization
ifeq (${DISABLE_OPTIMIZATION},true)
	GO_GCFLAGS=-gcflags "-N -l"
	VERSION:="$(VERSION)-noopt"
endif

GO_LDFLAGS=-ldflags "-X `go list ./version`.Version $(VERSION)"

.PHONY: clean all fmt vet lint build test binaries
.DEFAULT: default
all: AUTHORS clean fmt vet fmt lint build test binaries

AUTHORS: .mailmap .git/HEAD
	 git log --format='%aN <%aE>' | sort -fu > $@

# This only needs to be generated by hand when cutting full releases.
version/version.go:
	./version/version.sh > $@

${PREFIX}/bin/registry: version/version.go $(shell find . -type f -name '*.go')
	@echo "+ $@"
	@go build -tags "${DOCKER_BUILDTAGS}" -o $@ ${GO_LDFLAGS}  ${GO_GCFLAGS} ./cmd/registry

${PREFIX}/bin/digest: version/version.go $(shell find . -type f -name '*.go')
	@echo "+ $@"
	@go build -tags "${DOCKER_BUILDTAGS}" -o $@ ${GO_LDFLAGS}  ${GO_GCFLAGS} ./cmd/digest

${PREFIX}/bin/registry-api-descriptor-template: version/version.go $(shell find . -type f -name '*.go')
	@echo "+ $@"
	@go build -o $@ ${GO_LDFLAGS} ${GO_GCFLAGS} ./cmd/registry-api-descriptor-template

${PREFIX}/bin/pruner: version/version.go $(shell find . -type f -name '*.go')
	@echo "+ $@"
	@go build -tags "${DOCKER_BUILDTAGS}" -o $@ ${GO_LDFLAGS}  ${GO_GCFLAGS} ./cmd/pruner

docs/spec/api.md: docs/spec/api.md.tmpl ${PREFIX}/bin/registry-api-descriptor-template
	./bin/registry-api-descriptor-template $< > $@

# Depends on binaries because vet will silently fail if it can't load compiled
# imports
vet: binaries
	@echo "+ $@"
	@go vet ./...

fmt:
	@echo "+ $@"
	@test -z "$$(gofmt -s -l . | grep -v Godeps/_workspace/src/ | tee /dev/stderr)" || \
		echo "+ please format Go code with 'gofmt -s'"

lint:
	@echo "+ $@"
	@test -z "$$(golint ./... | grep -v Godeps/_workspace/src/ | tee /dev/stderr)"

build:
	@echo "+ $@"
	@go build -tags "${DOCKER_BUILDTAGS}" -v ${GO_LDFLAGS} ./...

test:
	@echo "+ $@"
	@go test -test.short -tags "${DOCKER_BUILDTAGS}" ./...

test-full:
	@echo "+ $@"
	@go test ./...

binaries: ${PREFIX}/bin/registry ${PREFIX}/bin/digest ${PREFIX}/bin/registry-api-descriptor-template ${PREFIX}/bin/pruner
	@echo "+ $@"

clean:
	@echo "+ $@"
	@rm -rf "${PREFIX}/bin/registry" "${PREFIX}/bin/registry-api-descriptor-template"
