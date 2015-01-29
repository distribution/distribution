FROM golang:1.4

ENV CONFIG_PATH /etc/docker/registry/config.yml
ENV DISTRIBUTION_DIR /go/src/github.com/docker/distribution
ENV GOPATH $DISTRIBUTION_DIR/Godeps/_workspace:$GOPATH

WORKDIR $DISTRIBUTION_DIR
COPY . $DISTRIBUTION_DIR
RUN make PREFIX=/go clean binaries
RUN mkdir -pv "$(dirname $CONFIG_PATH)"
RUN cp -lv ./cmd/registry/config.yml $CONFIG_PATH

EXPOSE 5000
CMD registry $CONFIG_PATH
