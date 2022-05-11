package storage

import (
	"context"
	"io"
)

// --- basics --->

// Storage is one of the base interfaces in the storage APIs.
// This type is rarely seen by itself alone (and never useful to implement alone),
// but is included in both ReadableStorage and WritableStorage.
// Because it's included in both the of the other two useful base interfaces,
// you can define functions that work on either one of them
// by using this type to describe your function's parameters.
//
// Library functions that work with storage systems should take either
// ReadableStorage, or WritableStorage, or Storage, as a parameter,
// depending on whether the function deals with the reading of data,
// or the writing of data, or may be found on either, respectively.
//
// An implementation of Storage may also support many other methods.
// At the very least, it should also support one of either ReadableStorage or WritableStorage.
// It may support even more interfaces beyond that for additional feature detection.
// See the package-wide docs for more discussion of this design.
//
// The Storage interface does not include much of use in itself alone,
// because ReadableStorage and WritableStorage are meant to be the most used types in declarations.
// However, it does include the Has function, because that function is reasonable to require ubiquitously from all implementations,
// and it serves as a reasonable marker to make sure the Storage interface is not trivially satisfied.
type Storage interface {
	Has(ctx context.Context, key string) (bool, error)
}

// ReadableStorage is one of the base interfaces in the storage APIs;
// a storage system should implement at minimum either this, or WritableStorage,
// depending on whether it supports reading or writing.
// (One type may also implement both.)
//
// ReadableStorage implementations must at minimum provide
// a way to ask the store whether it contains a key,
// and a way to ask it to return the value.
//
// Library functions that work with storage systems should take either
// ReadableStorage, or WritableStorage, or Storage, as a parameter,
// depending on whether the function deals with the reading of data,
// or the writing of data, or may be found on either, respectively.
//
// An implementation of ReadableStorage may also support many other methods --
// for example, it may additionally match StreamingReadableStorage, or yet more interfaces.
// Usually, you should not need to check for this yourself; instead,
// you should use the storage package's functions to ask for the desired mode of interaction.
// Those functions will will accept any ReadableStorage as an argument,
// detect the additional interfaces automatically and use them if present,
// or, fall back to synthesizing equivalent behaviors from the basics.
// See the package-wide docs for more discussion of this design.
type ReadableStorage interface {
	Storage
	Get(ctx context.Context, key string) ([]byte, error)
}

// WritableStorage is one of the base interfaces in the storage APIs;
// a storage system should implement at minimum either this, or ReadableStorage,
// depending on whether it supports reading or writing.
// (One type may also implement both.)
//
// WritableStorage implementations must at minimum provide
// a way to ask the store whether it contains a key,
// and a way to put a value into storage indexed by some key.
//
// Library functions that work with storage systems should take either
// ReadableStorage, or WritableStorage, or Storage, as a parameter,
// depending on whether the function deals with the reading of data,
// or the writing of data, or may be found on either, respectively.
//
// An implementation of WritableStorage may also support many other methods --
// for example, it may additionally match StreamingWritableStorage, or yet more interfaces.
// Usually, you should not need to check for this yourself; instead,
// you should use the storage package's functions to ask for the desired mode of interaction.
// Those functions will will accept any WritableStorage as an argument,
// detect the additional interfaces automatically and use them if present,
// or, fall back to synthesizing equivalent behaviors from the basics.
// See the package-wide docs for more discussion of this design.
type WritableStorage interface {
	Storage
	Put(ctx context.Context, key string, content []byte) error
}

// --- streaming --->

type StreamingReadableStorage interface {
	GetStream(ctx context.Context, key string) (io.ReadCloser, error)
}

// StreamingWritableStorage is a feature-detection interface that advertises support for streaming writes.
// It is normal for APIs to use WritableStorage in their exported API surface,
// and then internally check if that value implements StreamingWritableStorage if they wish to use streaming operations.
//
// Streaming writes can be preferable to the all-in-one style of writing of WritableStorage.Put,
// because with streaming writes, the high water mark for memory usage can be kept lower.
// On the other hand, streaming writes can incur slightly higher allocation counts,
// which may cause some performance overhead when handling many small writes in sequence.
//
// The PutStream function returns three parameters: an io.Writer (as you'd expect), another function, and an error.
// The function returned is called a "WriteCommitter".
// The final error value is as usual: it will contain an error value if the write could not be begun.
// ("WriteCommitter" will be refered to as such throughout the docs, but we don't give it a named type --
// unfortunately, this is important, because we don't want to force implementers of storage systems to import this package just for a type name.)
//
// The WriteCommitter function should be called when you're done writing,
// at which time you give it the key you want to commit the data as.
// It will close and flush any streams, and commit the data to its final location under this key.
// (If the io.Writer is also an io.WriteCloser, it is not necessary to call Close on it,
// because using the WriteCommiter will do this for you.)
//
// Because these storage APIs are meant to work well for content-addressed systems,
// the key argument is not provided at the start of the write -- it's provided at the end.
// (This gives the opportunity to be computing a hash of the contents as they're written to the stream.)
//
// As a special case, giving a key of the zero string to the WriteCommiter will
// instead close and remove any temp files, and store nothing.
// An error may still be returned from the WriteCommitter if there is an error cleaning up
// any temporary storage buffers that were created.
//
// Continuing to write to the io.Writer after calling the WriteCommitter function will result in errors.
// Calling the WriteCommitter function more than once will result in errors.
type StreamingWritableStorage interface {
	PutStream(ctx context.Context) (io.Writer, func(key string) error, error)
}

// --- other specializations --->

// VectorWritableStorage is an API for writing several slices of bytes at once into storage.
// It's meant a feature-detection interface; not all storage implementations need to provide this feature.
// This kind of API can be useful for maximizing performance in scenarios where
// data is already loaded completely into memory, but scattered across several non-contiguous regions.
type VectorWritableStorage interface {
	PutVec(ctx context.Context, key string, blobVec [][]byte) error
}

// PeekableStorage is a feature-detection interface which a storage implementation can use to advertise
// the ability to look at a piece of data, and return it in shared memory.
// The PeekableStorage.Peek method is essentially the same as ReadableStorage.Get --
// but by contrast, ReadableStorage is expected to return a safe copy.
// PeekableStorage can be used when the caller knows they will not mutate the returned slice.
//
// An io.Closer is returned along with the byte slice.
// The Close method on the Closer must be called when the caller is done with the byte slice;
// otherwise, memory leaks may result.
// (Implementers of this interface may be expecting to reuse the byte slice after Close is called.)
//
// Note that Peek does not imply that the caller can use the byte slice freely;
// doing so may result in storage corruption or other undefined behavior.
type PeekableStorage interface {
	Peek(ctx context.Context, key string) ([]byte, io.Closer, error)
}

// the following are all hypothetical additional future interfaces (in varying degress of speculativeness):

// FUTURE: an EnumerableStorage API, that lets you list all keys present?

// FUTURE: a cleanup API (for getting rid of tmp files that might've been left behind on rough shutdown)?

// FUTURE: a sync-forcing API?

// FUTURE: a delete API?  sure.  (just document carefully what its consistency model is -- i.e. basically none.)
//   (hunch: if you do want some sort of consistency model -- consider offering a whole family of methods that have some sort of generation or sequencing number on them.)

// FUTURE: a force-overwrite API?  (not useful for a content-address system.  but maybe a gesture towards wider reusability is acceptable to have on offer.)

// FUTURE: a size estimation API?  (unclear if we need to standardize this, but we could.  an offer, anyway.)

// FUTURE: a GC API?  (dubious -- doing it well probably crosses logical domains, and should not be tied down here.)
