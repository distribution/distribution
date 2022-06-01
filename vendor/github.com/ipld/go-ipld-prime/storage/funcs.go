package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

/*
	This file contains equivalents of every method that can be feature-detected on a storage system.
	You can always call these functions, and give them the most basic storage interface,
	and they'll attempt to feature-detect their way to the best possible implementation of the behavior,
	or they'll fall back to synthesizing the same behavior from more basic interfaces.

	Long story short: you can always use these functions as an end user, and get the behavior you want --
	regardless of how much explicit support the storage implementation has for the exact behavior you requested.
*/

func Has(ctx context.Context, store Storage, key string) (bool, error) {
	// Okay, not much going on here -- this function is only here for consistency of style.
	return store.Has(ctx, key)
}

func Get(ctx context.Context, store ReadableStorage, key string) ([]byte, error) {
	// Okay, not much going on here -- this function is only here for consistency of style.
	return store.Get(ctx, key)
}

func Put(ctx context.Context, store WritableStorage, key string, content []byte) error {
	// Okay, not much going on here -- this function is only here for consistency of style.
	return store.Put(ctx, key, content)
}

// GetStream returns a streaming reader.
// This function will feature-detect the StreamingReadableStorage interface, and use that if possible;
// otherwise it will fall back to using basic ReadableStorage methods transparently
// (at the cost of loading all the data into memory at once and up front).
func GetStream(ctx context.Context, store ReadableStorage, key string) (io.ReadCloser, error) {
	// Prefer the feature itself, first.
	if streamable, ok := store.(StreamingReadableStorage); ok {
		return streamable.GetStream(ctx, key)
	}
	// Fallback to basic.
	blob, err := store.Get(ctx, key)
	return noopCloser{bytes.NewReader(blob)}, err
}

// PutStream returns an io.Writer and a WriteCommitter callback.
// (See the docs on StreamingWritableStorage.PutStream for details on what that means.)
// This function will feature-detect the StreamingWritableStorage interface, and use that if possible;
// otherwise it will fall back to using basic WritableStorage methods transparently
// (at the cost of needing to buffer all of the content in memory while the write is in progress).
func PutStream(ctx context.Context, store WritableStorage) (io.Writer, func(key string) error, error) {
	// Prefer the feature itself, first.
	if streamable, ok := store.(StreamingWritableStorage); ok {
		return streamable.PutStream(ctx)
	}
	// Fallback to basic.
	var buf bytes.Buffer
	var written bool
	return &buf, func(key string) error {
		if written {
			return fmt.Errorf("WriteCommitter already used")
		}
		written = true
		return store.Put(ctx, key, buf.Bytes())
	}, nil
}

// PutVec is an API for writing several slices of bytes at once into storage.
// This kind of API can be useful for maximizing performance in scenarios where
// data is already loaded completely into memory, but scattered across several non-contiguous regions.
// This function will feature-detect the VectorWritableStorage interface, and use that if possible;
// otherwise it will fall back to using StreamingWritableStorage,
// or failing that, fall further back to basic WritableStorage methods, transparently.
func PutVec(ctx context.Context, store WritableStorage, key string, blobVec [][]byte) error {
	// Prefer the feature itself, first.
	if putvable, ok := store.(VectorWritableStorage); ok {
		return putvable.PutVec(ctx, key, blobVec)
	}
	// Fallback to streaming mode.
	// ... or, fallback to basic, and use emulated streaming.  Still presumably preferable to doing a big giant memcopy.
	// Conveniently, the PutStream function makes that transparent for our implementation, too.
	wr, wrcommit, err := PutStream(ctx, store)
	if err != nil {
		return err
	}
	for _, blob := range blobVec {
		_, err := wr.Write(blob)
		if err != nil {
			return err
		}
	}
	return wrcommit(key)
}

// Peek accessess the same data as Get, but indicates that the caller promises not to mutate the returned byte slice.
// (By contrast, Get is expected to return a safe copy.)
// This function will feature-detect the PeekableStorage interface, and use that if possible;
// otherwise it will fall back to using basic ReadableStorage methods transparently
// (meaning that a no-copy fastpath simply wasn't available).
//
// An io.Closer is returned along with the byte slice.
// The Close method on the Closer must be called when the caller is done with the byte slice;
// otherwise, memory leaks may result.
// (Implementers of this interface may be expecting to reuse the byte slice after Close is called.)
func Peek(ctx context.Context, store ReadableStorage, key string) ([]byte, io.Closer, error) {
	// Prefer the feature itself, first.
	if peekable, ok := store.(PeekableStorage); ok {
		return peekable.Peek(ctx, key)
	}
	// Fallback to basic.
	bs, err := store.Get(ctx, key)
	return bs, noopCloser{nil}, err
}

type noopCloser struct {
	io.Reader
}

func (noopCloser) Close() error { return nil }
