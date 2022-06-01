// +build !amd64

package blake3

import "encoding/binary"

func compressNode(n node) (out [16]uint32) {
	compressNodeGeneric(&out, n)
	return
}

func compressBuffer(buf *[maxSIMD * chunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) node {
	return compressBufferGeneric(buf, buflen, key, counter, flags)
}

func compressChunk(chunk []byte, key *[8]uint32, counter uint64, flags uint32) node {
	n := node{
		cv:       *key,
		counter:  counter,
		blockLen: blockSize,
		flags:    flags | flagChunkStart,
	}
	var block [blockSize]byte
	for len(chunk) > blockSize {
		copy(block[:], chunk)
		chunk = chunk[blockSize:]
		bytesToWords(block, &n.block)
		n.cv = chainingValue(n)
		n.flags &^= flagChunkStart
	}
	// pad last block with zeros
	block = [blockSize]byte{}
	n.blockLen = uint32(len(chunk))
	copy(block[:], chunk)
	bytesToWords(block, &n.block)
	n.flags |= flagChunkEnd
	return n
}

func hashBlock(out *[64]byte, buf []byte) {
	var block [64]byte
	var words [16]uint32
	copy(block[:], buf)
	bytesToWords(block, &words)
	compressNodeGeneric(&words, node{
		cv:       iv,
		block:    words,
		blockLen: uint32(len(buf)),
		flags:    flagChunkStart | flagChunkEnd | flagRoot,
	})
	wordsToBytes(words, out)
}

func compressBlocks(out *[maxSIMD * blockSize]byte, n node) {
	var outs [maxSIMD][64]byte
	compressBlocksGeneric(&outs, n)
	for i := range outs {
		copy(out[i*64:], outs[i][:])
	}
}

func mergeSubtrees(cvs *[maxSIMD][8]uint32, numCVs uint64, key *[8]uint32, flags uint32) node {
	return mergeSubtreesGeneric(cvs, numCVs, key, flags)
}

func bytesToWords(bytes [64]byte, words *[16]uint32) {
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(bytes[4*i:])
	}
}

func wordsToBytes(words [16]uint32, block *[64]byte) {
	for i, w := range words {
		binary.LittleEndian.PutUint32(block[4*i:], w)
	}
}
