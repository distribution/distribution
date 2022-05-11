package linking

import (
	"bytes"
	"context"
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// This file contains all the functions on LinkSystem.
// These are the helpful, user-facing functions we expect folks to use "most of the time" when loading and storing data.

// Varations:
// - Load vs Store vs ComputeLink
// - Load vs LoadPlusRaw
// - With or without LinkContext?
//   - Brevity would be nice but I can't think of what to name the functions, so: everything takes LinkContext.  Zero value is fine though.
// - [for load direction only]: Prototype (and return Node|error) or Assembler (and just return error)?
//   - naming: Load vs Fill.
// - 'Must' variants.

// Can we get as far as a `QuickLoad(lnk Link) (Node, error)` function, which doesn't even ask you for a NodePrototype?
//  No, not quite.  (Alas.)  If we tried to do so, and make it use `basicnode.Prototype`, we'd have import cycles; ded.

// Load looks up some data identified by a Link, and does everything necessary to turn it into usable data.
// In detail, that means it:
// brings that data into memory,
// verifies the hash,
// parses it into the Data Model using a codec,
// and returns an IPLD Node.
//
// Where the data will be loaded from is determined by the configuration of the LinkSystem
// (namely, the StorageReadOpener callback, which can either be set directly,
// or configured via the SetReadStorage function).
//
// The in-memory form used for the returned Node is determined by the given NodePrototype parameter.
// A new builder and a new node will be allocated, via NodePrototype.NewBuilder.
// (If you'd like more control over memory allocation, you may wish to see the Fill function instead.)
//
// A schema may also be used, and apply additional data validation during loading,
// by using a schema.TypedNodePrototype as the NodePrototype argument.
//
// The LinkContext parameter may be used to pass contextual information down to the loading layer.
//
// Which hashing function is used to validate the loaded data is determined by LinkSystem.HasherChooser.
// Which codec is used to parse the loaded data into the Data Model is determined by LinkSystem.DecoderChooser.
//
// The LinkSystem.NodeReifier callback is also applied before returning the Node,
// and so Load may also thereby return an ADL.
func (lsys *LinkSystem) Load(lnkCtx LinkContext, lnk datamodel.Link, np datamodel.NodePrototype) (datamodel.Node, error) {
	nb := np.NewBuilder()
	if err := lsys.Fill(lnkCtx, lnk, nb); err != nil {
		return nil, err
	}
	nd := nb.Build()
	if lsys.NodeReifier == nil {
		return nd, nil
	}
	return lsys.NodeReifier(lnkCtx, nd, lsys)
}

// MustLoad is identical to Load, but panics in the case of errors.
//
// This function is meant for convenience of use in test and demo code, but should otherwise probably be avoided.
func (lsys *LinkSystem) MustLoad(lnkCtx LinkContext, lnk datamodel.Link, np datamodel.NodePrototype) datamodel.Node {
	if n, err := lsys.Load(lnkCtx, lnk, np); err != nil {
		panic(err)
	} else {
		return n
	}
}

// LoadPlusRaw is similar to Load, but additionally retains and returns the byte slice of the raw data parsed.
//
// Be wary of using this with large data, since it will hold all data in memory at once.
// For more control over streaming, you may want to construct a LinkSystem where you wrap the storage opener callbacks,
// and thus can access the streams (and tee them, or whatever you need to do) as they're opened.
// This function is meant for convenience when data sizes are small enough that fitting them into memory at once is not a problem.
func (lsys *LinkSystem) LoadPlusRaw(lnkCtx LinkContext, lnk datamodel.Link, np datamodel.NodePrototype) (datamodel.Node, []byte, error) {
	// Choose all the parts.
	decoder, err := lsys.DecoderChooser(lnk)
	if err != nil {
		return nil, nil, ErrLinkingSetup{"could not choose a decoder", err}
	}
	// Use LoadRaw to get the data.
	//  If we're going to have everything in memory at once, we might as well do that first, and then give the codec and the hasher the whole thing at once.
	block, err := lsys.LoadRaw(lnkCtx, lnk)
	if err != nil {
		return nil, block, err
	}
	// Create a NodeBuilder.
	// Deploy the codec.
	// Build the node.
	nb := np.NewBuilder()
	if err := decoder(nb, bytes.NewBuffer(block)); err != nil {
		return nil, block, err
	}
	nd := nb.Build()
	// Consider applying NodeReifier, if applicable.
	if lsys.NodeReifier == nil {
		return nd, block, nil
	}
	nd, err = lsys.NodeReifier(lnkCtx, nd, lsys)
	return nd, block, err
}

// LoadRaw looks up some data identified by a Link, brings that data into memory,
// verifies the hash, and returns it directly as a byte slice.
//
// LoadRaw does not return a data model view of the data,
// nor does it verify that a codec can parse the data at all!
// Use this function at your own risk; it does not provide the same guarantees as the Load or Fill functions do.
func (lsys *LinkSystem) LoadRaw(lnkCtx LinkContext, lnk datamodel.Link) ([]byte, error) {
	if lnkCtx.Ctx == nil {
		lnkCtx.Ctx = context.Background()
	}
	// Choose all the parts.
	hasher, err := lsys.HasherChooser(lnk.Prototype())
	if err != nil {
		return nil, ErrLinkingSetup{"could not choose a hasher", err}
	}
	if lsys.StorageReadOpener == nil {
		return nil, ErrLinkingSetup{"no storage configured for reading", io.ErrClosedPipe} // REVIEW: better cause?
	}
	// Open storage: get the data.
	// FUTURE: this could probably use storage.ReadableStorage.Get instead of streaming and a buffer, if we refactored LinkSystem to carry that interface through.
	reader, err := lsys.StorageReadOpener(lnkCtx, lnk)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, err
	}
	// Compute the hash.
	// (Then do a bit of a jig to build a link out of it -- because that's what we do the actual hash equality check on.)
	hasher.Write(buf.Bytes())
	hash := hasher.Sum(nil)
	lnk2 := lnk.Prototype().BuildLink(hash)
	if lnk2 != lnk {
		return nil, ErrHashMismatch{Actual: lnk2, Expected: lnk}
	}
	// No codec to deploy; this is the raw load function.
	// So we're done.
	return buf.Bytes(), nil
}

// Fill is similar to Load, but allows more control over memory allocations.
// Instead of taking a NodePrototype parameter, Fill takes a NodeAssembler parameter:
// this allows you to use your own NodeBuilder (and reset it, etc, thus controlling allocations),
// or, to fill in some part of a larger structure.
//
// Note that Fill does not regard NodeReifier, even if one has been configured.
// (This is in contrast to Load, which does regard a NodeReifier if one is configured, and thus may return an ADL node).
func (lsys *LinkSystem) Fill(lnkCtx LinkContext, lnk datamodel.Link, na datamodel.NodeAssembler) error {
	if lnkCtx.Ctx == nil {
		lnkCtx.Ctx = context.Background()
	}
	// Choose all the parts.
	decoder, err := lsys.DecoderChooser(lnk)
	if err != nil {
		return ErrLinkingSetup{"could not choose a decoder", err}
	}
	hasher, err := lsys.HasherChooser(lnk.Prototype())
	if err != nil {
		return ErrLinkingSetup{"could not choose a hasher", err}
	}
	if lsys.StorageReadOpener == nil {
		return ErrLinkingSetup{"no storage configured for reading", io.ErrClosedPipe} // REVIEW: better cause?
	}
	// Open storage; get a reader stream.
	reader, err := lsys.StorageReadOpener(lnkCtx, lnk)
	if err != nil {
		return err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	// TrustedStorage indicates the data coming out of this reader has already been hashed and verified earlier.
	// As a result, we can skip rehashing it
	if lsys.TrustedStorage {
		return decoder(na, reader)
	}
	// Tee the stream so that the hasher is fed as the unmarshal progresses through the stream.
	tee := io.TeeReader(reader, hasher)
	// The actual read is then dragged forward by the codec.
	decodeErr := decoder(na, tee)
	if decodeErr != nil {
		// It is important to security to check the hash before returning any other observation about the content,
		// so, if the decode process returns any error, we have several steps to take before potentially returning it.
		// First, we try to copy any data remaining that wasn't already pulled through the TeeReader by the decoder,
		// so that the hasher can reach the end of the stream.
		// If _that_ errors, return the I/O level error.
		// We hang onto decodeErr for a while: we can't return that until all the way after we check the hash equality.
		_, err := io.Copy(hasher, reader)
		if err != nil {
			return err
		}
	}
	// Compute the hash.
	// (Then do a bit of a jig to build a link out of it -- because that's what we do the actual hash equality check on.)
	hash := hasher.Sum(nil)
	lnk2 := lnk.Prototype().BuildLink(hash)
	if lnk2 != lnk {
		return ErrHashMismatch{Actual: lnk2, Expected: lnk}
	}
	// If we got all the way through IO and through the hash check:
	// now, finally, if we did get an error from the codec, we can admit to that.
	if decodeErr != nil {
		return decodeErr
	}
	return nil
}

// MustFill is identical to Fill, but panics in the case of errors.
//
// This function is meant for convenience of use in test and demo code, but should otherwise probably be avoided.
func (lsys *LinkSystem) MustFill(lnkCtx LinkContext, lnk datamodel.Link, na datamodel.NodeAssembler) {
	if err := lsys.Fill(lnkCtx, lnk, na); err != nil {
		panic(err)
	}
}

func (lsys *LinkSystem) Store(lnkCtx LinkContext, lp datamodel.LinkPrototype, n datamodel.Node) (datamodel.Link, error) {
	if lnkCtx.Ctx == nil {
		lnkCtx.Ctx = context.Background()
	}
	// Choose all the parts.
	encoder, err := lsys.EncoderChooser(lp)
	if err != nil {
		return nil, ErrLinkingSetup{"could not choose an encoder", err}
	}
	hasher, err := lsys.HasherChooser(lp)
	if err != nil {
		return nil, ErrLinkingSetup{"could not choose a hasher", err}
	}
	if lsys.StorageWriteOpener == nil {
		return nil, ErrLinkingSetup{"no storage configured for writing", io.ErrClosedPipe} // REVIEW: better cause?
	}
	// Open storage write stream, feed serial data to the storage and the hasher, and funnel the codec output into both.
	writer, commitFn, err := lsys.StorageWriteOpener(lnkCtx)
	if err != nil {
		return nil, err
	}
	tee := io.MultiWriter(writer, hasher)
	err = encoder(n, tee)
	if err != nil {
		return nil, err
	}
	lnk := lp.BuildLink(hasher.Sum(nil))
	return lnk, commitFn(lnk)
}

func (lsys *LinkSystem) MustStore(lnkCtx LinkContext, lp datamodel.LinkPrototype, n datamodel.Node) datamodel.Link {
	if lnk, err := lsys.Store(lnkCtx, lp, n); err != nil {
		panic(err)
	} else {
		return lnk
	}
}

// ComputeLink returns a Link for the given data, but doesn't do anything else
// (e.g. it doesn't try to store any of the serial-form data anywhere else).
func (lsys *LinkSystem) ComputeLink(lp datamodel.LinkPrototype, n datamodel.Node) (datamodel.Link, error) {
	encoder, err := lsys.EncoderChooser(lp)
	if err != nil {
		return nil, ErrLinkingSetup{"could not choose an encoder", err}
	}
	hasher, err := lsys.HasherChooser(lp)
	if err != nil {
		return nil, ErrLinkingSetup{"could not choose a hasher", err}
	}
	err = encoder(n, hasher)
	if err != nil {
		return nil, err
	}
	return lp.BuildLink(hasher.Sum(nil)), nil
}

func (lsys *LinkSystem) MustComputeLink(lp datamodel.LinkPrototype, n datamodel.Node) datamodel.Link {
	if lnk, err := lsys.ComputeLink(lp, n); err != nil {
		panic(err)
	} else {
		return lnk
	}
}
