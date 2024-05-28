# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21.8
ARG ALPINE_VERSION=3.19

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS base
RUN apk add --no-cache git

FROM base AS hugo
ARG HUGO_VERSION=0.119.0
RUN --mount=type=cache,target=/go/mod/pkg \
    go install github.com/gohugoio/hugo@v${HUGO_VERSION}

FROM base AS build-base
COPY --from=hugo $GOPATH/bin/hugo /bin/hugo
WORKDIR /src

FROM build-base AS build
RUN --mount=type=bind,rw,source=docs,target=. \
    hugo --gc --minify --destination /out

FROM build-base AS server
COPY docs .
ENTRYPOINT [ "hugo", "server", "--bind", "0.0.0.0" ]
EXPOSE 1313

FROM scratch AS out
COPY --from=build /out /

FROM wjdp/htmltest:v0.17.0 AS test
# Copy the site to a public/distribution subdirectory
# This is a workaround for a limitation in htmltest, see:
# https://github.com/wjdp/htmltest/issues/45
WORKDIR /test/public/distribution
COPY --from=build /out .
WORKDIR /test
ADD docs/.htmltest.yml .htmltest.yml
RUN --mount=type=cache,target=tmp/.htmltest \
    htmltest
