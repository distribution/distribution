// +build !noresumabledigest

package storage

import "github.com/docker/distribution/digest"

func (lw *layerWriter) setupResumableDigester() {
	lw.resumableDigester = digest.NewCanonicalResumableDigester()
}
