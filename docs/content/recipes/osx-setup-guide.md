---
description: Explains how to run a registry on macOS
keywords: registry, on-prem, images, tags, repository, distribution, macOS, recipe, advanced
title: macOS setup guide
---

## Use-case

This is useful if you intend to run a registry server natively on macOS.

### Alternatives

You can start a VM on macOS, and deploy your registry normally as a container using Docker inside that VM.

### Solution

Using the method described here, you install and compile your own from the git repository and run it as an macOS agent.

### Gotchas

Production services operation on macOS is out of scope of this document. Be sure you understand well these aspects before considering going to production with this.

## Setup golang on your machine

If you know, safely skip to the next section.

If you don't, the TLDR is:

    bash < <(curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer)
    source ~/.gvm/scripts/gvm
    gvm install go1.4.2
    gvm use go1.4.2

If you want to understand, you should read [How to Write Go Code](https://golang.org/doc/code.html).

## Checkout the source tree

    mkdir -p $GOPATH/src/github.com/distribution
    git clone https://github.com/distribution/distribution.git $GOPATH/src/github.com/distribution/distribution
    cd $GOPATH/src/github.com/distribution/distribution

## Build the binary

    GOPATH=$(PWD)/Godeps/_workspace:$GOPATH make binaries
    sudo mkdir -p /usr/local/libexec
    sudo cp bin/registry /usr/local/libexec/registry

## Setup

Copy the registry configuration file in place:

    mkdir /Users/Shared/Registry
    cp docs/osx/config.yml /Users/Shared/Registry/config.yml

## Run the registry under launchd

Copy the registry plist into place:

    plutil -lint docs/recipes/osx/com.docker.registry.plist
    cp docs/recipes/osx/com.docker.registry.plist ~/Library/LaunchAgents/
    chmod 644 ~/Library/LaunchAgents/com.docker.registry.plist

Start the registry:

    launchctl load ~/Library/LaunchAgents/com.docker.registry.plist

### Restart the registry service

    launchctl stop com.docker.registry
    launchctl start com.docker.registry

### Unload the registry service

    launchctl unload ~/Library/LaunchAgents/com.docker.registry.plist
