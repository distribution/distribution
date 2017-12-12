.PHONY: all fmt vet lint test test-short test-long
.DEFAULT: all
all: AUTHORS fmt vet lint test-short test-long
test: fmt vet test-short


AUTHORS: .mailmap .git/HEAD
	 git log --format='%aN <%aE>' | sort -fu > $@

# Required for go 1.5 to build
GO15VENDOREXPERIMENT := 1

# Package list
PKGS := $(shell go list -tags "${BUILDTAGS}" ./... | grep -v "/vendor/")

# Resolving binary dependencies for specific targets
GOLINT_BIN := $(GOPATH)/bin/golint
GOLINT := $(shell [ -x $(GOLINT_BIN) ] && echo $(GOLINT_BIN) || echo '')

GODEP_BIN := $(GOPATH)/bin/godep
GODEP := $(shell [ -x $(GODEP_BIN) ] && echo $(GODEP_BIN) || echo '')

vet:
	@echo "+ $@"
	go vet -tags "${BUILDTAGS}" $(PKGS)

fmt:
	@echo "+ $@"
	test -z "$$(gofmt -s -l . 2>&1 | grep -v vendor/ | tee /dev/stderr)" || \
		(echo >&2 "+ please format Go code with 'gofmt -s'" && false)

lint:
	@echo "+ $@"
	$(if $(GOLINT), , \
		$(error Please install golint: `go get -u github.com/golang/lint/golint`))
	test -z "$$($(GOLINT) ./... 2>&1 | grep -v vendor/ | tee /dev/stderr)"

test-short:
	@echo "+ $@"
	go test -v -test.short -tags "${BUILDTAGS}" ./iam/ ./s3/

test-long:
	@echo "+ $@"
	go test -tags "${BUILDTAGS}" ./autoscaling/ ./aws/ ./cloudfront/ ./cloudwatch/ ./dynamodb/ ./ec2/ ./elasticache/ ./elb/ ./iam/ ./kinesis/ ./rds/ ./route53/ ./s3/ ./sns/ ./sqs/ ./sts/ ./exp/mturk/ ./exp/sdb/ ./exp/ses/

dep-save:
	@echo "+ $@"
	$(if $(GODEP), , \
		$(error Please install godep: go get github.com/tools/godep))
	godep save $(PKGS)

dep-restore:
	@echo "+ $@"
	$(if $(GODEP), , \
		$(error Please install godep: go get github.com/tools/godep))
	godep restore $(PKGS)
