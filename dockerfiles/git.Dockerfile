# syntax=docker/dockerfile:1

# GO_VERSION sets the version of the golang base image to use.
# It must be a supported tag in the docker.io/library/golang image repository.
ARG GO_VERSION=1.25.7

# ALPINE_VERSION sets the version of the alpine base image to use, including for the golang image.
# It must be a supported tag in the docker.io/library/alpine image repository
# that's also available as alpine image variant for the Golang version used.
ARG ALPINE_VERSION=3.23

FROM alpine:${ALPINE_VERSION} AS base
RUN apk add --no-cache git gpg

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS gitvalidation
ARG GIT_VALIDATION_VERSION=v1.1.0
RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
      GOBIN=/out go install "github.com/vbatts/git-validation@${GIT_VALIDATION_VERSION}"

FROM base AS validate
ARG COMMIT_RANGE
RUN if [ -z "$COMMIT_RANGE" ]; then echo "COMMIT_RANGE required" && exit 1; fi
ENV GIT_CHECK_EXCLUDE="./vendor"
WORKDIR /src
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache \
    --mount=from=gitvalidation,source=/out/git-validation,target=/usr/bin/git-validation \
      git-validation -q -range "${COMMIT_RANGE}" -run short-subject,dangling-whitespace
