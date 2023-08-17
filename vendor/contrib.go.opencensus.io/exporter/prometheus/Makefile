# TODO: Fix this on windows.
ALL_SRC := $(shell find . -name '*.go' \
								-not -path './vendor/*' \
								-not -path '*/gen-go/*' \
								-type f | sort)
ALL_PKGS := $(shell go list $(sort $(dir $(ALL_SRC))))

GOTEST_OPT?=-v -race -timeout 30s
GOTEST_OPT_WITH_COVERAGE = $(GOTEST_OPT) -coverprofile=coverage.txt -covermode=atomic
GOTEST=go test
LINT=golangci-lint
# TODO decide if we need to change these names.
README_FILES := $(shell find . -name '*README.md' | sort | tr '\n' ' ')

.DEFAULT_GOAL := lint-test

.PHONY: lint-test
lint-test: lint test

# TODO enable test-with-coverage in travis
.PHONY: travis-ci
travis-ci: lint test test-386

all-pkgs:
	@echo $(ALL_PKGS) | tr ' ' '\n' | sort

all-srcs:
	@echo $(ALL_SRC) | tr ' ' '\n' | sort

.PHONY: test
test:
	$(GOTEST) $(GOTEST_OPT) $(ALL_PKGS)

.PHONY: test-386
test-386:
	GOARCH=386 $(GOTEST) -v -timeout 30s $(ALL_PKGS)

.PHONY: test-with-coverage
test-with-coverage:
	$(GOTEST) $(GOTEST_OPT_WITH_COVERAGE) $(ALL_PKGS)

.PHONY: lint
lint:
	$(LINT) run --allow-parallel-runners

.PHONY: install-tools
install-tools:
	cd internal/tools && go install golang.org/x/tools/cmd/cover
	cd internal/tools && go install github.com/golangci/golangci-lint/cmd/golangci-lint

