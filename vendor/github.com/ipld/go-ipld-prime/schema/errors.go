package schema

import (
	"fmt"
	"strings"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// TODO: errors in this package remain somewhat slapdash.
//
//  - datamodel.ErrUnmatchable is used as a catch-all in some places, and contains who-knows-what values wrapped in the Reason field.
//    - sometimes this wraps things like strconv errors... and on the one hand, i'm kinda okay with that; on the other, maybe saying a bit more with types before getting to that kind of shrug would be nice.
//  - we probably want to use `Type` values, right?
//    - or do we: because then we probably need a `Repr bool` next to it, or lots of messages would be nonsensical.
//    - this is *currently* problematic because we don't actually generate type info consts yet.  Hopefully soon; but the pain, meanwhile, is... substantial.
//      - "substantial" is an understatement.  it makes incremental development almost impossible because stringifying error reports turn into nil pointer crashes!
//    - other ipld-wide errors like `datamodel.ErrWrongKind` *sometimes* refer to a TypeName... but don't *have* to, because they also arise at the merely-datamodel level; what would we do with these?
//      - it's undesirable (not to mention intensely forbidden for import cycle reasons) for those error types to refer to schema.Type.
//        - if we must have TypeName treated stringily in some cases, is it really useful to use full type info in other cases -- inconsistently?
//    - regardless of where we end up with this, some sort of an embed for helping deal with munging and printing this would probably be wise.
//  - generally, whether you should expect an "datamodel.Err*" or a "schema.Err*" from various methods is quite unclear.
//  - it's possible that we should wrap *all* schema-level errors in a single "datamodel.ErrSchemaNoMatch" error of some kind, to fix the above.  (and maybe that's what ErrUnmatchable really is.)  as yet undecided.

// ErrUnmatchable is the error raised when processing data with IPLD Schemas and
// finding data which cannot be matched into the schema.
// It will be returned by NodeAssemblers and NodeBuilders when they are fed unmatchable data.
// As a result, it will also often be seen returned from unmarshalling
// when unmarshalling into schema-constrained NodeAssemblers.
//
// ErrUnmatchable provides the name of the type in the schema that data couldn't be matched to,
// and wraps another error as the more detailed reason.
type ErrUnmatchable struct {
	// TypeName will indicate the named type of a node the function was called on.
	TypeName string

	// Reason must always be present.  ErrUnmatchable doesn't say much otherwise.
	Reason error
}

func (e ErrUnmatchable) Error() string {
	return fmt.Sprintf("matching data to schema of %s rejected: %s", e.TypeName, e.Reason)
}

// Reasonf returns a new ErrUnmatchable with a Reason field set to the Errorf of the arguments.
// It's a helper function for creating untyped error reasons without importing the fmt package.
func (e ErrUnmatchable) Reasonf(format string, a ...interface{}) ErrUnmatchable {
	return ErrUnmatchable{e.TypeName, fmt.Errorf(format, a...)}
}

// ErrMissingRequiredField is returned when calling 'Finish' on a NodeAssembler
// for a Struct that has not has all required fields set.
type ErrMissingRequiredField struct {
	Missing []string
}

func (e ErrMissingRequiredField) Error() string {
	return "missing required fields: " + strings.Join(e.Missing, ",")
}

// ErrInvalidKey indicates a key is invalid for some reason.
//
// This is only possible for typed nodes; specifically, it may show up when
// handling struct types, or maps with interesting key types.
// (Other kinds of key invalidity that happen for untyped maps
// fall under ErrRepeatedMapKey or ErrWrongKind.)
// (Union types use ErrInvalidUnionDiscriminant instead of ErrInvalidKey,
// even when their representation strategy is maplike.)
type ErrInvalidKey struct {
	// TypeName will indicate the named type of a node the function was called on.
	TypeName string

	// Key is the key that was rejected.
	Key datamodel.Node

	// Reason, if set, may provide details (for example, the reason a key couldn't be converted to a type).
	// If absent, it'll be presumed "no such field".
	// ErrUnmatchable may show up as a reason for typed maps with complex keys.
	Reason error
}

func (e ErrInvalidKey) Error() string {
	if e.Reason == nil {
		return fmt.Sprintf("invalid key for map %s: %q: no such field", e.TypeName, e.Key)
	} else {
		return fmt.Sprintf("invalid key for map %s: %q: %s", e.TypeName, e.Key, e.Reason)
	}
}

// ErrNoSuchField may be returned from lookup functions on the Node
// interface when a field is requested which doesn't exist,
// or from assigning data into on a MapAssembler for a struct
// when the key doesn't match a field name in the structure
// (or, when assigning data into a ListAssembler and the list size has
// reached out of bounds, in case of a struct with list-like representations!).
type ErrNoSuchField struct {
	Type Type

	Field datamodel.PathSegment
}

func (e ErrNoSuchField) Error() string {
	if e.Type == nil {
		return fmt.Sprintf("no such field: {typeinfomissing}.%s", e.Field)
	}
	return fmt.Sprintf("no such field: %s.%s", e.Type.Name(), e.Field)
}

// ErrNotUnionStructure means data was fed into a union assembler that can't match the union.
//
// This could have one of several reasons, which are explained in the detail text:
//
//   - there are too many entries in the map;
//   - the keys of critical entries aren't found;
//   - keys are found that aren't any of the expected critical keys;
//   - etc.
//
// TypeName is currently a string... see comments at the top of this file for
// remarks on the issues we need to address about these identifiers in errors in general.
type ErrNotUnionStructure struct {
	TypeName string

	Detail string
}

func (e ErrNotUnionStructure) Error() string {
	return fmt.Sprintf("cannot match schema: union structure constraints for %s caused rejection: %s", e.TypeName, e.Detail)
}
