package encode

import (
	"fmt"
	"strings"
)

const seperator string = "-"

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
	return len(b.header.String())
}

//AddBlock will add a block to an array of blocks
func (b *BlockResponse) AddBlock(block []byte, digest string) {
	b.Blocks = append(b.Blocks, block)
	b.lengthOfBlocks += len(block)

	b.header.WriteString(seperator)
	if block == nil {
		b.header.WriteString(digest)
	} else {
		b.header.WriteString("0")
	}
}

// GetBlockResponseFromByteStream will generate
// a block response from a byte[]
func GetBlockResponseFromByteStream(headerlength int, byteStream []byte) (BlockResponse, []string) {
	var b BlockResponse

	header := string(byteStream[:headerlength])
	blockKeys := strings.Split(header, seperator)[1:] //We have to get rid of empty character at beginning introduced by split
	if Debug == true {
		//fmt.Println("Received byte stream: ", byteStream)
		fmt.Println("Received header: ", header)
		fmt.Println("Receive header Bytes:", byteStream[:headerlength])

		fmt.Println("Block Keys: ", blockKeys)
		fmt.Println("Length of Block Keys: ", len(blockKeys))
		// b.Blocks = make([][]byte, len(blockLengths))	//TODO: Can be optimized
	}

	blockCodeStream := byteStream[headerlength:]

	counter := 0
	for _, blockKey := range blockKeys {
		if blockKey == "0" {
			startIndex, endIndex := BlockIndices(counter, len(blockCodeStream))
			b.AddBlock(blockCodeStream[startIndex:endIndex], "0")
			counter++
		} else {
			b.AddBlock(nil, blockKey)
		}
	}

	return b, blockKeys
}

// ConvertBlockResponseToByteStream will convert a
// block response object to appropriate binary stream
// Returns byte stream and length of header
func ConvertBlockResponseToByteStream(b BlockResponse) ([]byte, int) {
	byteStream := make([]byte, b.HeaderLength()+b.lengthOfBlocks)
	headerBytes := []byte(b.header.String())
	copy(byteStream[:b.HeaderLength()], headerBytes)

	if Debug == true {
		fmt.Println("Sending header:", b.header.String())
		fmt.Println("Header bytes:", headerBytes)
	}

	startingIndex := b.HeaderLength()
	for _, block := range b.Blocks {
		if block != nil {
			endingIndex := startingIndex + len(block)
			copy(byteStream[startingIndex:endingIndex], block)
			startingIndex = endingIndex
		}
	}

	return byteStream, b.HeaderLength()
}
