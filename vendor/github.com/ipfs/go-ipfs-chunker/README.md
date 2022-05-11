# go-ipfs-chunker

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![](https://img.shields.io/badge/project-IPFS-blue.svg?style=flat-square)](http://ipfs.io/)
[![standard-readme compliant](https://img.shields.io/badge/standard--readme-OK-green.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![GoDoc](https://godoc.org/github.com/ipfs/go-ipfs-chunker?status.svg)](https://godoc.org/github.com/ipfs/go-ipfs-chunker)
[![Build Status](https://travis-ci.org/ipfs/go-ipfs-chunker.svg?branch=master)](https://travis-ci.org/ipfs/go-ipfs-chunker)

> go-ipfs-chunker implements data Splitters for go-ipfs.

`go-ipfs-chunker` provides the `Splitter` interface. IPFS splitters read data from a reader an create "chunks". These chunks are used to build the ipfs DAGs (Merkle Tree) and are the base unit to obtain the sums that ipfs uses to address content.

The package provides a `SizeSplitter` which creates chunks of equal size and it is used by default in most cases, and a `rabin` fingerprint chunker. This chunker will attempt to split data in a way that the resulting blocks are the same when the data has repetitive patterns, thus optimizing the resulting DAGs.

## Table of Contents

- [Install](#install)
- [Usage](#usage)
- [Contribute](#contribute)
- [License](#license)

## Install

`go-ipfs-chunker` works like a regular Go module:

```
> go get github.com/ipfs/go-ipfs-chunker
```

It uses [Gx](https://github.com/whyrusleeping/gx) to manage dependencies. You can use `make all` to build it with the `gx` dependencies.

## Usage

```
import "github.com/ipfs/go-ipfs-chunker"
```

Check the [GoDoc documentation](https://godoc.org/github.com/ipfs/go-ipfs-chunker)

## Contribute

PRs accepted.

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

MIT © Protocol Labs, Inc.
