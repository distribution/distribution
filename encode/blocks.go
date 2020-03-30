package encode

import (
	"fmt"
	"strconv"
	"strings"
)

//BlockResponse is a set of blocks of response
type BlockResponse struct {
	header         strings.Builder
	Blocks         [][]byte
	lengthOfBlocks int
}

//GetNewBlockResponse will generate a block response
func GetNewBlockResponse(length int) BlockResponse {
	var b BlockResponse
	b.Blocks = make([][]byte, length)
	return b
}

//HeaderLength gives the length of the header of the body
func (b *BlockResponse) HeaderLength() int {
	return b.header.Len()
}

//AddBlock will add a block to an array of blocks
func (b *BlockResponse) AddBlock(block []byte) {
	b.Blocks = append(b.Blocks, block)
	b.lengthOfBlocks += len(block)

	b.header.WriteString("-")
	if block == nil {
		b.header.WriteString("0")
	} else {
		b.header.WriteString(string(len(block)))
	}
}

// GetBlockResponseFromByteStream will generate
// a block response from a byte[]
func GetBlockResponseFromByteStream(headerlength int, byteStream []byte) BlockResponse {
	var b BlockResponse

	header := byteStream[:headerlength]
	blockLengths := strings.Split(string(header), "-")
	fmt.Println("Received header: ", header)
	fmt.Println("Block Lengths: ", blockLengths)

	b.Blocks = make([][]byte, len(blockLengths))
	blockCodeStream := byteStream[headerlength:]

	runningIndex := 0
	for _, lengthAsString := range blockLengths {
		length, _ := strconv.Atoi(lengthAsString)
		b.AddBlock(blockCodeStream[runningIndex : runningIndex+length])
		runningIndex += length
	}

	return b
}

// ConvertBlockResponseToByteStream will convert a
// block response object to appropriate binary stream
// Returns byte stream and length of header
func ConvertBlockResponseToByteStream(b BlockResponse) ([]byte, int) {
	byteStream := make([]byte, b.HeaderLength()+b.lengthOfBlocks)
	copy(byteStream[:b.HeaderLength()], []byte(b.header.String()))
	fmt.Println("Sending header:", b.header.String())
	startingIndex := 0
	for _, block := range b.Blocks {
		endingIndex := startingIndex + len(block)
		copy(byteStream[startingIndex:endingIndex], []byte(b.header.String()))
		startingIndex = endingIndex
	}

	return byteStream, b.HeaderLength()
}
