package encode

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const seperator string = "f"

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
	return hex.DecodedLen(len(b.header.String()))
}

//AddBlock will add a block to an array of blocks
func (b *BlockResponse) AddBlock(block []byte) {
	b.Blocks = append(b.Blocks, block)
	b.lengthOfBlocks += len(block)

	b.header.WriteString(seperator)
	if block == nil {
		b.header.WriteString("0")
	} else {
		b.header.WriteString(strconv.Itoa(len(block)))
	}
}

// GetBlockResponseFromByteStream will generate
// a block response from a byte[]
func GetBlockResponseFromByteStream(headerlength int, byteStream []byte) BlockResponse {
	var b BlockResponse

	header := hex.EncodeToString(byteStream[:headerlength])
	blockLengths := strings.Split(header, seperator)
	fmt.Println("Received byte stream: ", byteStream)
	fmt.Println("Received header: ", header)
	fmt.Println("Receive header Bytes:", byteStream[:headerlength])

	fmt.Println("Block Lengths: ", blockLengths)
	fmt.Println("Length of Block Lengths: ", len(blockLengths))
	// b.Blocks = make([][]byte, len(blockLengths))	//TODO: Can be optimized

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
	headerBytes, _ := hex.DecodeString(b.header.String())
	copy(byteStream[:b.HeaderLength()], headerBytes)

	fmt.Println("Sending header:", b.header.String())
	fmt.Println("Header bytes:", headerBytes)

	startingIndex := b.HeaderLength()
	for _, block := range b.Blocks {
		endingIndex := startingIndex + len(block)
		copy(byteStream[startingIndex:endingIndex], block)
		startingIndex = endingIndex
	}
	//fmt.Println("Sent byte stream: ", byteStream)
	return byteStream, b.HeaderLength()
}
