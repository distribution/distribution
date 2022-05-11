package linking

import (
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/storage"
)

// SetReadStorage configures how the LinkSystem will look for information to load,
// setting it to look at the given storage.ReadableStorage.
//
// This will overwrite the LinkSystem.StorageReadOpener field.
//
// This mechanism only supports setting exactly one ReadableStorage.
// If you would like to make a more complex configuration
// (for example, perhaps using information from a LinkContext to decide which storage area to use?)
// then you should set LinkSystem.StorageReadOpener to a custom callback of your own creation instead.
func (lsys *LinkSystem) SetReadStorage(store storage.ReadableStorage) {
	lsys.StorageReadOpener = func(lctx LinkContext, lnk datamodel.Link) (io.Reader, error) {
		return storage.GetStream(lctx.Ctx, store, lnk.Binary())
	}
}

// SetWriteStorage configures how the LinkSystem will store information,
// setting it to write into the given storage.WritableStorage.
//
// This will overwrite the LinkSystem.StorageWriteOpener field.
//
// This mechanism only supports setting exactly one WritableStorage.
// If you would like to make a more complex configuration
// (for example, perhaps using information from a LinkContext to decide which storage area to use?)
// then you should set LinkSystem.StorageWriteOpener to a custom callback of your own creation instead.
func (lsys *LinkSystem) SetWriteStorage(store storage.WritableStorage) {
	lsys.StorageWriteOpener = func(lctx LinkContext) (io.Writer, BlockWriteCommitter, error) {
		wr, wrcommit, err := storage.PutStream(lctx.Ctx, store)
		return wr, func(lnk datamodel.Link) error {
			return wrcommit(lnk.Binary())
		}, err
	}
}
