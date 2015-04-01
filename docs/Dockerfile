FROM docs/base:latest
MAINTAINER Mary <mary@docker.com> (@moxiegirl)

# to get the git info for this repo
COPY . /src

# Reset the /docs dir so we can replace the theme meta with the new repo's git info
RUN git reset --hard

# 
#
RUN git describe --match 'v[0-9]*' --dirty='.m' --always > /docs/VERSION
COPY docs/* /docs/sources/distribution/
COPY docs/mkdocs.yml /docs/mkdocs-distribution.yml

# Then build everything together, ready for mkdocs
RUN /docs/build.sh
