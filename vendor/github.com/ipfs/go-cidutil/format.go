package cidutil

import (
	"bytes"
	"fmt"

	c "github.com/ipfs/go-cid"
	mb "github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
)

// FormatRef is a string documenting the format string for the Format function
const FormatRef = `
   %% literal %
   %b multibase name
   %B multibase code
   %v version string
   %V version number
   %c codec name
   %C codec code
   %h multihash name
   %H multihash code
   %L hash digest length
   %m multihash encoded in base %b (with multibase prefix)
   %M multihash encoded in base %b without multibase prefix
   %d hash digest encoded in base %b (with multibase prefix)
   %D hash digest encoded in base %b without multibase prefix
   %s cid string encoded in base %b (1)
   %S cid string encoded in base %b without multibase prefix
   %P cid prefix: %v-%c-%h-%L

(1) For CID version 0 the multibase must be base58btc and no prefix is
used.  For Cid version 1 the multibase prefix is included.
`

// Format formats a cid according to the format specificer as
// documented in the FormatRef constant
func Format(fmtStr string, base mb.Encoding, cid c.Cid) (string, error) {
	p := cid.Prefix()
	var out bytes.Buffer
	var err error
	encoder, err := mb.NewEncoder(base)
	if err != nil {
		return "", err
	}
	for i := 0; i < len(fmtStr); i++ {
		if fmtStr[i] != '%' {
			out.WriteByte(fmtStr[i])
			continue
		}
		i++
		if i >= len(fmtStr) {
			return "", FormatStringError{"premature end of format string", ""}
		}
		switch fmtStr[i] {
		case '%':
			out.WriteByte('%')
		case 'b': // base name
			out.WriteString(baseToString(base))
		case 'B': // base code
			out.WriteByte(byte(base))
		case 'v': // version string
			fmt.Fprintf(&out, "cidv%d", p.Version)
		case 'V': // version num
			fmt.Fprintf(&out, "%d", p.Version)
		case 'c': // codec name
			out.WriteString(codecToString(p.Codec))
		case 'C': // codec code
			fmt.Fprintf(&out, "%d", p.Codec)
		case 'h': // hash fun name
			out.WriteString(hashToString(p.MhType))
		case 'H': // hash fun code
			fmt.Fprintf(&out, "%d", p.MhType)
		case 'L': // hash length
			fmt.Fprintf(&out, "%d", p.MhLength)
		case 'm', 'M': // multihash encoded in base %b
			out.WriteString(encode(encoder, cid.Hash(), fmtStr[i] == 'M'))
		case 'd', 'D': // hash digest encoded in base %b
			dec, err := mh.Decode(cid.Hash())
			if err != nil {
				return "", err
			}
			out.WriteString(encode(encoder, dec.Digest, fmtStr[i] == 'D'))
		case 's': // cid string encoded in base %b
			str, err := cid.StringOfBase(base)
			if err != nil {
				return "", err
			}
			out.WriteString(str)
		case 'S': // cid string without base prefix
			out.WriteString(encode(encoder, cid.Bytes(), true))
		case 'P': // prefix
			fmt.Fprintf(&out, "cidv%d-%s-%s-%d",
				p.Version,
				codecToString(p.Codec),
				hashToString(p.MhType),
				p.MhLength,
			)
		default:
			return "", FormatStringError{"unrecognized specifier in format string", fmtStr[i-1 : i+1]}
		}

	}
	return out.String(), err
}

// FormatStringError is the error return from Format when the format
// string is ill formed
type FormatStringError struct {
	Message   string
	Specifier string
}

func (e FormatStringError) Error() string {
	if e.Specifier == "" {
		return e.Message
	} else {
		return fmt.Sprintf("%s: %s", e.Message, e.Specifier)
	}
}

func baseToString(base mb.Encoding) string {
	baseStr, ok := mb.EncodingToStr[base]
	if !ok {
		return fmt.Sprintf("base?%c", base)
	}
	return baseStr
}

func codecToString(num uint64) string {
	name, ok := c.CodecToStr[num]
	if !ok {
		return fmt.Sprintf("codec?%d", num)
	}
	return name
}

func hashToString(num uint64) string {
	name, ok := mh.Codes[num]
	if !ok {
		return fmt.Sprintf("hash?%d", num)
	}
	return name
}

func encode(base mb.Encoder, data []byte, strip bool) string {
	str := base.Encode(data)
	if strip {
		return str[1:]
	}
	return str
}

// ScanForCid scans bytes for anything resembling a CID. If one is
// found `i` will point to the begging of the cid and `j` to to the
// end and the cid will be returned, otherwise `i` and `j` will point
// the end of the buffer and the cid will be `Undef`.
func ScanForCid(buf []byte) (i, j int, cid c.Cid, cidStr string) {
	i = 0
	for {
		i = j
		for i < len(buf) && !asciiIsAlpha(buf[i]) {
			i++
		}
		j = i
		if i == len(buf) {
			return
		}
		for j < len(buf) && asciiIsAlpha(buf[j]) {
			j++
		}
		if j-i <= 1 || j-i > 128 || !supported[buf[i]] {
			continue
		}
		var err error
		cidStr = string(buf[i:j])
		cid, err = c.Decode(cidStr)
		if err == nil {
			return
		}
	}
}

var supported = make([]bool, 256)

func init() {
	// for now base64 encoding are not supported as they contain non
	// alhphanumeric characters
	supportedPrefixes := []byte("QfFbBcCvVtThzZ")
	for _, b := range supportedPrefixes {
		supported[b] = true
	}
}

func asciiIsAlpha(b byte) bool {
	return ('A' <= b && b <= 'Z') || ('a' <= b && b <= 'z') || ('0' <= b && b <= '9')
}
