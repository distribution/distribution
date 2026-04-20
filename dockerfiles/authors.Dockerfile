# syntax=docker/dockerfile:1

# ALPINE_VERSION sets the version of the alpine base image to use.
# It must be a supported tag in the docker.io/library/alpine image repository.
ARG ALPINE_VERSION=3.23

FROM alpine:${ALPINE_VERSION} AS gen
RUN apk add --no-cache git
WORKDIR /src
RUN --mount=type=bind,target=. <<EOT
  set -e
  mkdir /out
  # see also ".mailmap" for how email addresses and names are deduplicated
  {
    echo "# This file lists all individuals having contributed content to the repository."
    echo "# For how it is generated, see dockerfiles/authors.Dockerfile."
    echo
    git log --format='%aN <%aE>' | LC_ALL=C.UTF-8 sort -uf
  } > /out/AUTHORS
  cat /out/AUTHORS
EOT

FROM scratch AS update
COPY --from=gen /out /

FROM gen AS validate
RUN --mount=type=bind,target=.,rw <<EOT
  set -e
  git add -A
  cp -rf /out/* .
  if [ -n "$(git status --porcelain -- AUTHORS)" ]; then
    echo >&2 'ERROR: Authors result differs. Please update with "make authors"'
    git status --porcelain -- AUTHORS
    exit 1
  fi
EOT
