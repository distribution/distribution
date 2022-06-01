package ipld

import (
	"github.com/ipld/go-ipld-prime/linking"
)

type (
	LinkSystem  = linking.LinkSystem
	LinkContext = linking.LinkContext
)

type (
	BlockReadOpener     = linking.BlockReadOpener
	BlockWriteOpener    = linking.BlockWriteOpener
	BlockWriteCommitter = linking.BlockWriteCommitter
	NodeReifier         = linking.NodeReifier
)
