# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23.7
ARG ALPINE_VERSION=3.21
ARG GOLANGCI_LINT_VERSION=v1.64.8
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
      golangci-lint --timeout "${TIMEOUT}" --build-tags "${BUILDTAGS}" run
