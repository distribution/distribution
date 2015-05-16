// +build !noresumabledigest

package storage

import "github.com/docker/distribution/digest"

func (bw *blobWriter) setupResumableDigester() {
	bw.resumableDigester = digest.NewCanonicalResumableDigester()
}
