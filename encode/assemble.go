package encode

// AssembleBlockResponse will generate a block response
// from the declaration and the encodings in the db
func AssembleBlockResponse(d Declaration, r Recipe, blob []byte) BlockResponse {
	var b BlockResponse

	if len(blob) == 0 {
		return GetNewBlockResponse(0)
	}
	// O in declaration implies client doesn't have the block
	// 1 implies client has block
	startIndex := 0
	endIndex := 0
	for i, v := range d.Encodings {
		startIndex, endIndex = BlockIndices(i, len(blob))
		if v == true {
			b.AddBlock(nil, r.Keys[i])
		} else {
			b.AddBlock(blob[startIndex:endIndex], r.Keys[i])
		}
	}

	return b
}
