FROM debian:jessie

# compile and runtime deps
# https://github.com/docker/docker/blob/master/project/PACKAGERS.md#runtime-dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
        # For DIND
        ca-certificates \
        curl \
        iptables \
        procps \
        e2fsprogs \
        xz-utils \
        # For build
        build-essential \
        file \
        git \
        net-tools \ 
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Docker
ENV VERSION 1.7.0-rc1
RUN curl -L -o /usr/local/bin/docker https://test.docker.com/builds/Linux/x86_64/docker-${VERSION} \
    && chmod +x /usr/local/bin/docker

# Install DIND
RUN curl -L -o /dind https://raw.githubusercontent.com/docker/docker/master/hack/dind \
    && chmod +x /dind

# Install golang
ENV GO_VERSION 1.4.2
RUN curl -sSL https://golang.org/dl/go${GO_VERSION}.src.tar.gz | tar -v -C /usr/local -xz \
    && mkdir -p /go/bin
ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go
RUN cd /usr/local/go/src && ./make.bash --no-clean 2>&1

# Go dependencies
RUN go get github.com/tools/godep

# Install bats
RUN cd /usr/local/src/ \
    && git clone https://github.com/sstephenson/bats.git \
    && cd bats \
    && ./install.sh /usr/local

# Install docker-compose
RUN curl -L https://github.com/docker/compose/releases/download/1.2.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose \
    && chmod +x /usr/local/bin/docker-compose

RUN mkdir -p /go/src/github.com/docker/distribution
WORKDIR /go/src/github.com/docker/distribution/contrib/docker-integration

VOLUME /var/lib/docker

ENTRYPOINT ["/dind"]
