# syntax=docker/dockerfile:1

# GO_VERSION sets the version of the golang base image to use.
# It must be a supported tag in the docker.io/library/golang image repository.
ARG GO_VERSION=1.25.7

# ALPINE_VERSION sets the version of the alpine base image to use, including for the golang image.
# It must be a supported tag in the docker.io/library/alpine image repository
# that's also available as alpine image variant for the Golang version used.
ARG ALPINE_VERSION=3.23

# GOLANGCI_LINT_VERSION sets the version of the golangci/golangci-lint image to use.
ARG GOLANGCI_LINT_VERSION=v2.9
ARG BUILDTAGS=""

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine AS golangci-lint

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS base
RUN apk add --no-cache gcc musl-dev
WORKDIR /src

FROM base
ENV GOFLAGS="-buildvcs=false"
ARG TIMEOUT="5m"
ARG BUILDTAGS
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=from=golangci-lint,source=/usr/bin/golangci-lint,target=/usr/bin/golangci-lint \
    golangci-lint config verify && \
    golangci-lint --timeout "${TIMEOUT}" --build-tags "${BUILDTAGS}" run