FROM docs/base:hugo-github-linking
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

ENV PROJECT=registry

# To get the git info for this repo
COPY . /src

COPY . /docs/content/$PROJECT/
