FROM golang:1.4

ENV DISTRIBUTION_DIR /go/src/github.com/docker/distribution
ENV GOPATH $DISTRIBUTION_DIR/Godeps/_workspace:$GOPATH
ENV HTTP_PROXY ""
ENV HTTPS_PROXY ""

WORKDIR $DISTRIBUTION_DIR
COPY . $DISTRIBUTION_DIR
RUN make PREFIX=/go clean binaries

EXPOSE 5000
ENTRYPOINT ["registry"]
CMD ["cmd/registry/config.yml"]
