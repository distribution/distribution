# syntax=docker/dockerfile:1

# GO_VERSION sets the version of the golang base image to use.
# It must be a supported tag in the docker.io/library/golang image repository.
ARG GO_VERSION=1.25.7

# ALPINE_VERSION sets the version of the alpine base image to use, including for the golang image.
# It must be a supported tag in the docker.io/library/alpine image repository
# that's also available as alpine image variant for the Golang version used.
ARG ALPINE_VERSION=3.23

ARG MODOUTDATED_VERSION=v0.8.0

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS base
RUN apk add --no-cache git rsync
WORKDIR /src

FROM base AS vendored
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs  \
    --mount=target=/go/pkg/mod,type=cache <<EOT
  set -e
  rsync -a /context/. .
  go mod tidy
  go mod vendor
  mkdir /out
  cp -r go.mod go.sum vendor /out
EOT

FROM scratch AS update
COPY --from=vendored /out /out

FROM vendored AS validate
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs <<EOT
  set -e
  rsync -a /context/. .
  git add -A
  rm -rf vendor
  cp -rf /out/* .
  if [ -n "$(git status --porcelain -- go.mod go.sum vendor)" ]; then
    echo >&2 'ERROR: Vendor result differs. Please vendor your package with "make vendor"'
    git status --porcelain -- go.mod go.sum vendor
    exit 1
  fi
EOT

FROM psampaz/go-mod-outdated:${MODOUTDATED_VERSION} AS go-mod-outdated
FROM base AS outdated
RUN --mount=target=.,ro \
    --mount=target=/go/pkg/mod,type=cache \
    --mount=from=go-mod-outdated,source=/home/go-mod-outdated,target=/usr/bin/go-mod-outdated \
      go list -mod=readonly -u -m -json all | go-mod-outdated -update -direct
