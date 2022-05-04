# syntax=docker/dockerfile:1

ARG GO_VERSION=1.17
ARG GOLANGCI_LINT_VERSION=v1.44

FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine AS golangci-lint

FROM golang:${GO_VERSION}-alpine AS base
RUN apk add --no-cache gcc musl-dev
WORKDIR /src

FROM base
RUN --mount=type=bind,target=. \
  --mount=type=cache,target=/root/.cache \
  --mount=from=golangci-lint,source=/usr/bin/golangci-lint,target=/usr/bin/golangci-lint \
  golangci-lint run
