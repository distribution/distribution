package encode

// AssembleBlockResponse will generate a block response
// from the declaration and the encodings in the db
func AssembleBlockResponse(d Declaration, blob []byte) BlockResponse {
	var b BlockResponse

	// O in declaration implies client doesn't have the block
	// 1 implies client has block
	coveredIndex := 0
	startIndex := 0
	endIndex := 0
	for i, v := range d.Encodings {
		startIndex = i * ShiftOfWindow
		if v == true {
			b.AddBlock([]byte{})
		} else {
			endIndex = startIndex + SizeOfWindow
			if endIndex > len(blob) {
				endIndex = len(blob)
			}

			if coveredIndex < endIndex {
				b.AddBlock(blob[coveredIndex:endIndex])
			} else {
				b.AddBlock([]byte{})
			}
		}
		coveredIndex = startIndex + SizeOfWindow //Covered index cannot be greater than this value anyways
	}

	return b
}
