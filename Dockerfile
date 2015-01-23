FROM golang:1.4

ENV CONFIG_PATH /etc/docker/registry/config.yml
RUN mkdir -pv "$(dirname $CONFIG_PATH)"

ENV DISTRIBUTION_DIR /go/src/github.com/docker/distribution
WORKDIR $DISTRIBUTION_DIR
COPY . $DISTRIBUTION_DIR
ENV GOPATH $GOPATH:$DISTRIBUTION_DIR/Godeps/_workspace

RUN go install -v ./cmd/registry

RUN cp -lv ./cmd/registry/config.yml $CONFIG_PATH

EXPOSE 5000
CMD registry $CONFIG_PATH
