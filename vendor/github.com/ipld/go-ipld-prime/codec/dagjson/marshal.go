package dagjson

import (
	"encoding/base64"
	"fmt"
	"io"
	"sort"

	"github.com/polydawn/refmt/json"
	"github.com/polydawn/refmt/shared"
	"github.com/polydawn/refmt/tok"

	"github.com/ipld/go-ipld-prime/codec"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
)

// This should be identical to the general feature in the parent package,
// except for the `case datamodel.Kind_Link` block,
// which is dag-json's special sauce for schemafree links.

// EncodeOptions can be used to customize the behavior of an encoding function.
// The Encode method on this struct fits the codec.Encoder function interface.
type EncodeOptions struct {
	// If true, will encode nodes with a Link kind using the DAG-JSON
	// `{"/":"cid string"}` form.
	EncodeLinks bool

	// If true, will encode nodes with a Bytes kind using the DAG-JSON
	// `{"/":{"bytes":"base64 bytes..."}}` form.
	EncodeBytes bool

	// Control the sorting of map keys, using one of the `codec.MapSortMode_*` constants.
	MapSortMode codec.MapSortMode
}

// Encode walks the given datamodel.Node and serializes it to the given io.Writer.
// Encode fits the codec.Encoder function interface.
//
// The behavior of the encoder can be customized by setting fields in the EncodeOptions struct before calling this method.
func (cfg EncodeOptions) Encode(n datamodel.Node, w io.Writer) error {
	return Marshal(n, json.NewEncoder(w, json.EncodeOptions{}), cfg)
}

// Future work: we would like to remove the Marshal function,
// and in particular, stop seeing types from refmt (like shared.TokenSink) be visible.
// Right now, some kinds of configuration (e.g. for whitespace and prettyprint) are only available through interacting with the refmt types;
// we should improve our API so that this can be done with only our own types in this package.

// Marshal is a deprecated function.
// Please consider switching to EncodeOptions.Encode instead.
func Marshal(n datamodel.Node, sink shared.TokenSink, options EncodeOptions) error {
	var tk tok.Token
	switch n.Kind() {
	case datamodel.Kind_Invalid:
		return fmt.Errorf("cannot traverse a node that is absent")
	case datamodel.Kind_Null:
		tk.Type = tok.TNull
		_, err := sink.Step(&tk)
		return err
	case datamodel.Kind_Map:
		// Emit start of map.
		tk.Type = tok.TMapOpen
		expectedLength := int(n.Length())
		tk.Length = expectedLength // TODO: overflow check
		if _, err := sink.Step(&tk); err != nil {
			return err
		}
		if options.MapSortMode != codec.MapSortMode_None {
			// Collect map entries, then sort by key
			type entry struct {
				key   string
				value datamodel.Node
			}
			entries := []entry{}
			for itr := n.MapIterator(); !itr.Done(); {
				k, v, err := itr.Next()
				if err != nil {
					return err
				}
				keyStr, err := k.AsString()
				if err != nil {
					return err
				}
				entries = append(entries, entry{keyStr, v})
			}
			if len(entries) != expectedLength {
				return fmt.Errorf("map Length() does not match number of MapIterator() entries")
			}
			// Apply the desired sort function.
			switch options.MapSortMode {
			case codec.MapSortMode_Lexical:
				sort.Slice(entries, func(i, j int) bool {
					return entries[i].key < entries[j].key
				})
			case codec.MapSortMode_RFC7049:
				sort.Slice(entries, func(i, j int) bool {
					// RFC7049 style sort as per DAG-CBOR spec
					li, lj := len(entries[i].key), len(entries[j].key)
					if li == lj {
						return entries[i].key < entries[j].key
					}
					return li < lj
				})
			}
			// Emit map contents (and recurse).
			var entryCount int
			for _, e := range entries {
				tk.Type = tok.TString
				tk.Str = e.key
				entryCount++
				if _, err := sink.Step(&tk); err != nil {
					return err
				}
				if err := Marshal(e.value, sink, options); err != nil {
					return err
				}
			}
			if entryCount != expectedLength {
				return fmt.Errorf("map Length() does not match number of MapIterator() entries")
			}
		} else {
			// Don't sort map, emit map contents (and recurse).
			for itr := n.MapIterator(); !itr.Done(); {
				k, v, err := itr.Next()
				if err != nil {
					return err
				}
				tk.Type = tok.TString
				tk.Str, err = k.AsString()
				if err != nil {
					return err
				}
				if _, err := sink.Step(&tk); err != nil {
					return err
				}
				if err := Marshal(v, sink, options); err != nil {
					return err
				}
			}
		}
		// Emit map close.
		tk.Type = tok.TMapClose
		_, err := sink.Step(&tk)
		return err
	case datamodel.Kind_List:
		// Emit start of list.
		tk.Type = tok.TArrOpen
		l := n.Length()
		tk.Length = int(l) // TODO: overflow check
		if _, err := sink.Step(&tk); err != nil {
			return err
		}
		// Emit list contents (and recurse).
		for i := int64(0); i < l; i++ {
			v, err := n.LookupByIndex(i)
			if err != nil {
				return err
			}
			if err := Marshal(v, sink, options); err != nil {
				return err
			}
		}
		// Emit list close.
		tk.Type = tok.TArrClose
		_, err := sink.Step(&tk)
		return err
	case datamodel.Kind_Bool:
		v, err := n.AsBool()
		if err != nil {
			return err
		}
		tk.Type = tok.TBool
		tk.Bool = v
		_, err = sink.Step(&tk)
		return err
	case datamodel.Kind_Int:
		v, err := n.AsInt()
		if err != nil {
			return err
		}
		tk.Type = tok.TInt
		tk.Int = int64(v)
		_, err = sink.Step(&tk)
		return err
	case datamodel.Kind_Float:
		v, err := n.AsFloat()
		if err != nil {
			return err
		}
		tk.Type = tok.TFloat64
		tk.Float64 = v
		_, err = sink.Step(&tk)
		return err
	case datamodel.Kind_String:
		v, err := n.AsString()
		if err != nil {
			return err
		}
		tk.Type = tok.TString
		tk.Str = v
		_, err = sink.Step(&tk)
		return err
	case datamodel.Kind_Bytes:
		v, err := n.AsBytes()
		if err != nil {
			return err
		}
		if options.EncodeBytes {
			// Precisely seven tokens to emit:
			tk.Type = tok.TMapOpen
			tk.Length = 1
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TString
			tk.Str = "/"
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TMapOpen
			tk.Length = 1
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TString
			tk.Str = "bytes"
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Str = base64.RawStdEncoding.EncodeToString(v)
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TMapClose
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TMapClose
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			return nil
		} else {
			tk.Type = tok.TBytes
			tk.Bytes = v
			_, err = sink.Step(&tk)
			return err
		}
	case datamodel.Kind_Link:
		if !options.EncodeLinks {
			return fmt.Errorf("cannot Marshal ipld links to JSON")
		}
		v, err := n.AsLink()
		if err != nil {
			return err
		}
		switch lnk := v.(type) {
		case cidlink.Link:
			// Precisely four tokens to emit:
			tk.Type = tok.TMapOpen
			tk.Length = 1
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TString
			tk.Str = "/"
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Str = lnk.Cid.String()
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			tk.Type = tok.TMapClose
			if _, err = sink.Step(&tk); err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("schemafree link emission only supported by this codec for CID type links; got type %T", lnk)
		}
	default:
		panic("unreachable")
	}
}
