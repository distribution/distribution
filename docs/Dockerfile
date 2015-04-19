FROM docs/base:latest
MAINTAINER Mary <mary@docker.com> (@moxiegirl)

# to get the git info for this repo
COPY . /src

# Reset the /docs dir so we can replace the theme meta with the new repo's git info
RUN git reset --hard

#
# RUN git describe --match 'v[0-9]*' --dirty='.m' --always > /docs/VERSION
# The above line causes a floating point error in our tools
#
RUN grep "VERSION =" /src/version/version.go  | sed 's/.*"\(.*\)".*/\1/' > /docs/VERSION
COPY docs/* /docs/sources/registry/
COPY docs/images/* /docs/sources/registry/images/
COPY docs/spec/* /docs/sources/registry/spec/
COPY docs/spec/auth/* /docs/sources/registry/spec/auth/
COPY docs/storage-drivers/* /docs/sources/registry/storage-drivers/
COPY docs/mkdocs.yml /docs/mkdocs-distribution.yml


# Then build everything together, ready for mkdocs
RUN /docs/build.sh
