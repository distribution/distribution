FROM golang

COPY . /go/src/github.com/docker/distribution

# Fetch any dependencies to run the registry
RUN go get github.com/docker/distribution/...
RUN go install github.com/docker/distribution/cmd/registry

ENV CONFIG_PATH /etc/docker/registry/config.yml
COPY ./cmd/registry/config.yml $CONFIG_PATH

EXPOSE 5000
ENV PATH /go/bin
CMD registry $CONFIG_PATH
