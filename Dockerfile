FROM golang:1.4

ENV DISTRIBUTION_DIR /go/src/github.com/docker/distribution
ENV GOPATH $DISTRIBUTION_DIR/Godeps/_workspace:$GOPATH

WORKDIR $DISTRIBUTION_DIR
COPY . $DISTRIBUTION_DIR
RUN make PREFIX=/go clean binaries

EXPOSE 5000
ENTRYPOINT ["./docker-entrypoint.sh"]
CMD ["registry", "cmd/registry/config.yml"]
