package schemadsl

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	dmt "github.com/ipld/go-ipld-prime/schema/dmt"
)

var globalTrue = true

// TODO: fuzz testing

func ParseBytes(src []byte) (*dmt.Schema, error) {
	return Parse("", bytes.NewReader(src))
}

func ParseFile(path string) (*dmt.Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(path, f)
}

func Parse(name string, r io.Reader) (*dmt.Schema, error) {
	p := &parser{
		path: name,
		br:   bufio.NewReader(r),
		line: 1,
		col:  1,
	}

	sch := &dmt.Schema{}
	sch.Types.Values = make(map[string]dmt.TypeDefn)

	for {
		tok, err := p.consumeToken()
		if err == io.EOF {
			break
		}

		switch tok {
		case "type":
			name, err := p.consumeName()
			if err != nil {
				return nil, err
			}
			defn, err := p.typeDefn()
			if err != nil {
				return nil, err
			}
			mapAppend(&sch.Types, name, defn)
		case "advanced":
			return nil, p.errf("TODO: advanced")
		default:
			return nil, p.errf("unexpected token: %q", tok)
		}
	}
	return sch, nil
}

func mapAppend(mapPtr, k, v interface{}) {
	// TODO: delete with generics
	// TODO: error on dupes

	mval := reflect.ValueOf(mapPtr).Elem()
	kval := reflect.ValueOf(k)
	vval := reflect.ValueOf(v)

	keys := mval.FieldByName("Keys")
	keys.Set(reflect.Append(keys, kval))

	values := mval.FieldByName("Values")
	if values.IsNil() {
		values.Set(reflect.MakeMap(values.Type()))
	}
	values.SetMapIndex(kval, vval)
}

type parser struct {
	path string
	br   *bufio.Reader

	peekedToken string

	line, col int
}

func (p *parser) forwardError(err error) error {
	var prefix string
	if p.path != "" {
		prefix = p.path + ":"
	}
	return fmt.Errorf("%s%d:%d: %s", prefix, p.line, p.col, err)
}

func (p *parser) errf(format string, args ...interface{}) error {
	return p.forwardError(fmt.Errorf(format, args...))
}

func (p *parser) consumeToken() (string, error) {
	if tok := p.peekedToken; tok != "" {
		p.peekedToken = ""
		return tok, nil
	}
	for {
		// TODO: use runes for better unicode support
		b, err := p.br.ReadByte()
		if err == io.EOF {
			return "", err // TODO: ErrUnexpectedEOF?
		}
		if err != nil {
			return "", p.forwardError(err)
		}
		p.col++
		switch b {
		case ' ', '\t', '\r': // skip whitespace
			continue
		case '\n': // skip newline
			// TODO: should we require a newline after each type def, struct field, etc?
			p.line++
			p.col = 1
			continue
		case '"': // quoted string
			quoted, err := p.br.ReadString('"')
			if err != nil {
				return "", p.forwardError(err)
			}
			return "\"" + quoted, nil
		case '{', '}', '[', ']', '(', ')', ':', '&': // simple token
			return string(b), nil
		case '#': // comment
			_, err := p.br.ReadString('\n')
			if err != nil {
				return "", p.forwardError(err)
			}
			// tokenize the newline
			if err := p.br.UnreadByte(); err != nil {
				panic(err) // should never happen
			}
			continue
		default: // string token or name
			var sb strings.Builder
			sb.WriteByte(b)
			for {
				b, err := p.br.ReadByte()
				if err == io.EOF {
					// Token ends at the end of the whole input.
					return sb.String(), nil
				}
				if err != nil {
					return "", p.forwardError(err)
				}
				// TODO: should probably allow unicode letters and numbers, like Go?
				switch {
				case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
				case b >= '0' && b <= '9':
				case b == '_':
				default:
					if err := p.br.UnreadByte(); err != nil {
						panic(err) // should never happen
					}
					return sb.String(), nil
				}
				sb.WriteByte(b)
			}
		}
	}
}

func (p *parser) consumePeeked() {
	if p.peekedToken == "" {
		panic("consumePeeked requires a peeked token to be present")
	}
	p.peekedToken = ""
}

func (p *parser) peekToken() (string, error) {
	if tok := p.peekedToken; tok != "" {
		return tok, nil
	}
	tok, err := p.consumeToken()
	if err != nil {
		if err == io.EOF {
			// peekToken is often used when a token is optional.
			// If we hit io.EOF, that's not an error.
			// TODO: consider making peekToken just not return an error?
			return "", nil
		}
		return "", err
	}
	p.peekedToken = tok
	return tok, nil
}

func (p *parser) consumeName() (string, error) {
	tok, err := p.consumeToken()
	if err != nil {
		return "", err
	}
	switch tok {
	case "\"", "{", "}", "[", "]", "(", ")", ":":
		return "", p.errf("expected a name, got %q", tok)
	}
	if tok[0] == '"' {
		return "", p.errf("expected a name, got string %s", tok)
	}
	return tok, nil
}

func (p *parser) consumeString() (string, error) {
	tok, err := p.consumeToken()
	if err != nil {
		return "", err
	}
	if tok[0] != '"' {
		return "", p.errf("expected a string, got %q", tok)
	}
	// Unquote, too.
	return tok[1 : len(tok)-1], nil
}

func (p *parser) consumeRequired(tok string) error {
	got, err := p.consumeToken()
	if err != nil {
		return err
	}
	if got != tok {
		return p.errf("expected %q, got %q", tok, got)
	}
	return nil
}

func (p *parser) typeDefn() (dmt.TypeDefn, error) {
	var defn dmt.TypeDefn
	kind, err := p.consumeToken()
	if err != nil {
		return defn, err
	}

	switch kind {
	case "struct":
		if err := p.consumeRequired("{"); err != nil {
			return defn, err
		}
		defn.TypeDefnStruct, err = p.typeStruct()
	case "union":
		if err := p.consumeRequired("{"); err != nil {
			return defn, err
		}
		defn.TypeDefnUnion, err = p.typeUnion()
	case "enum":
		if err := p.consumeRequired("{"); err != nil {
			return defn, err
		}
		defn.TypeDefnEnum, err = p.typeEnum()
	case "bool":
		defn.TypeDefnBool = &dmt.TypeDefnBool{}
	case "bytes":
		defn.TypeDefnBytes = &dmt.TypeDefnBytes{}
	case "float":
		defn.TypeDefnFloat = &dmt.TypeDefnFloat{}
	case "int":
		defn.TypeDefnInt = &dmt.TypeDefnInt{}
	case "link":
		defn.TypeDefnLink = &dmt.TypeDefnLink{}
	case "any":
		defn.TypeDefnAny = &dmt.TypeDefnAny{}
	case "&":
		target, err := p.consumeName()
		if err != nil {
			return defn, err
		}
		defn.TypeDefnLink = &dmt.TypeDefnLink{ExpectedType: &target}
	case "string":
		defn.TypeDefnString = &dmt.TypeDefnString{}
	case "{":
		defn.TypeDefnMap, err = p.typeMap()
	case "[":
		defn.TypeDefnList, err = p.typeList()
	case "=":
		from, err := p.consumeName()
		if err != nil {
			return defn, err
		}
		defn.TypeDefnCopy = &dmt.TypeDefnCopy{FromType: from}
	default:
		err = p.errf("unknown type keyword: %q", kind)
	}

	return defn, err
}

func (p *parser) typeStruct() (*dmt.TypeDefnStruct, error) {
	repr := &dmt.StructRepresentation_Map{}
	repr.Fields = &dmt.Map__FieldName__StructRepresentation_Map_FieldDetails{}

	defn := &dmt.TypeDefnStruct{}
	for {
		tok, err := p.consumeToken()
		if err != nil {
			return nil, err
		}

		if tok == "}" {
			break
		}
		name := tok

		var field dmt.StructField
	loop:
		for {
			tok, err := p.peekToken()
			if err != nil {
				return nil, err
			}
			switch tok {
			case "optional":
				if field.Optional != nil {
					return nil, p.errf("multiple optional keywords")
				}
				field.Optional = &globalTrue
				p.consumePeeked()
			case "nullable":
				if field.Nullable != nil {
					return nil, p.errf("multiple nullable keywords")
				}
				field.Nullable = &globalTrue
				p.consumePeeked()
			default:
				var err error
				field.Type, err = p.typeNameOrInlineDefn()
				if err != nil {
					return nil, err
				}
				break loop
			}
		}
		tok, err = p.peekToken()
		if err != nil {
			return nil, err
		}
		if tok == "(" {
			details := dmt.StructRepresentation_Map_FieldDetails{}
			p.consumePeeked()
		parenLoop:
			for {
				tok, err = p.consumeToken()
				if err != nil {
					return nil, err
				}
				switch tok {
				case ")":
					break parenLoop
				case "rename":
					str, err := p.consumeString()
					if err != nil {
						return nil, err
					}
					details.Rename = &str
				case "implicit":
					scalar, err := p.consumeToken()
					if err != nil {
						return nil, err
					}
					var anyScalar dmt.AnyScalar
					switch {
					case scalar[0] == '"': // string
						s, err := strconv.Unquote(scalar)
						if err != nil {
							return nil, p.forwardError(err)
						}
						anyScalar.String = &s
					case scalar == "true", scalar == "false": // bool
						t := scalar == "true"
						anyScalar.Bool = &t
					case scalar[0] >= '0' && scalar[0] <= '0':
						n, err := strconv.Atoi(scalar)
						if err != nil {
							return nil, p.forwardError(err)
						}
						anyScalar.Int = &n
					default:
						return nil, p.errf("unsupported implicit scalar: %s", scalar)
					}

					details.Implicit = &anyScalar
				}
			}
			mapAppend(repr.Fields, name, details)
		}

		mapAppend(&defn.Fields, name, field)
	}

	reprName := "map" // default repr
	if tok, err := p.peekToken(); err == nil && tok == "representation" {
		p.consumePeeked()
		name, err := p.consumeName()
		if err != nil {
			return nil, err
		}
		reprName = name
	}
	if reprName != "map" && len(repr.Fields.Keys) > 0 {
		return nil, p.errf("rename and implicit are only supported for struct map representations")
	}
	switch reprName {
	case "map":
		if len(repr.Fields.Keys) == 0 {
			// Fields is optional; omit it if empty.
			repr.Fields = nil
		}
		defn.Representation.StructRepresentation_Map = repr
		return defn, nil
	case "tuple":
		defn.Representation.StructRepresentation_Tuple = &dmt.StructRepresentation_Tuple{}
		return defn, nil
		// TODO: support custom fieldorder
	default:
		return nil, p.errf("unknown struct repr: %q", reprName)
	}
}

func (p *parser) typeNameOrInlineDefn() (dmt.TypeNameOrInlineDefn, error) {
	var typ dmt.TypeNameOrInlineDefn
	tok, err := p.consumeToken()
	if err != nil {
		return typ, err
	}
	if tok == "&" {
		return typ, p.errf("TODO: links")
	}

	switch tok {
	case "[":
		tlist, err := p.typeList()
		if err != nil {
			return typ, err
		}
		typ.InlineDefn = &dmt.InlineDefn{TypeDefnList: tlist}
	case "{":
		tmap, err := p.typeMap()
		if err != nil {
			return typ, err
		}
		typ.InlineDefn = &dmt.InlineDefn{TypeDefnMap: tmap}
	default:
		typ.TypeName = &tok
	}
	return typ, nil
}

func (p *parser) typeList() (*dmt.TypeDefnList, error) {
	defn := &dmt.TypeDefnList{}
	tok, err := p.peekToken()
	if err != nil {
		return nil, err
	}
	if tok == "nullable" {
		defn.ValueNullable = &globalTrue
		p.consumePeeked()
	}

	defn.ValueType, err = p.typeNameOrInlineDefn()
	if err != nil {
		return nil, err
	}

	if err := p.consumeRequired("]"); err != nil {
		return defn, err
	}

	// TODO: repr
	return defn, nil
}

func (p *parser) typeMap() (*dmt.TypeDefnMap, error) {
	defn := &dmt.TypeDefnMap{}

	var err error
	defn.KeyType, err = p.consumeName()
	if err != nil {
		return nil, err
	}
	if err := p.consumeRequired(":"); err != nil {
		return defn, err
	}

	tok, err := p.peekToken()
	if err != nil {
		return nil, err
	}
	if tok == "nullable" {
		defn.ValueNullable = &globalTrue
		p.consumePeeked()
	}

	defn.ValueType, err = p.typeNameOrInlineDefn()
	if err != nil {
		return nil, err
	}

	if err := p.consumeRequired("}"); err != nil {
		return defn, err
	}

	// TODO: repr
	return defn, nil
}

func (p *parser) typeUnion() (*dmt.TypeDefnUnion, error) {
	defn := &dmt.TypeDefnUnion{}
	var reprKeys []string

	for {
		tok, err := p.consumeToken()
		if err != nil {
			return nil, err
		}
		if tok == "}" {
			break
		}
		if tok != "|" {
			return nil, p.errf("expected %q or %q, got %q", "}", "|", tok)
		}
		var member dmt.UnionMember
		name, err := p.consumeName()
		if err != nil {
			return nil, err
		}
		// TODO: inline defn
		member.TypeName = &name

		defn.Members = append(defn.Members, member)

		key, err := p.consumeToken()
		if err != nil {
			return nil, err
		}
		reprKeys = append(reprKeys, key)
	}
	if err := p.consumeRequired("representation"); err != nil {
		return nil, err
	}
	reprName, err := p.consumeName()
	if err != nil {
		return nil, err
	}
	switch reprName {
	case "keyed":
		repr := &dmt.UnionRepresentation_Keyed{}
		for i, keyStr := range reprKeys {
			key, err := strconv.Unquote(keyStr)
			if err != nil {
				return nil, p.forwardError(err)
			}
			mapAppend(repr, key, defn.Members[i])
		}
		defn.Representation.UnionRepresentation_Keyed = repr
	case "kinded":
		repr := &dmt.UnionRepresentation_Kinded{}
		// TODO: verify keys are valid kinds? enum should do it for us?
		for i, key := range reprKeys {
			mapAppend(repr, key, defn.Members[i])
		}
		defn.Representation.UnionRepresentation_Kinded = repr
	default:
		return nil, p.errf("TODO: union repr %q", reprName)
	}
	return defn, nil
}

func (p *parser) typeEnum() (*dmt.TypeDefnEnum, error) {
	defn := &dmt.TypeDefnEnum{}
	var reprKeys []string

	for {
		tok, err := p.consumeToken()
		if err != nil {
			return nil, err
		}
		if tok == "}" {
			break
		}
		if tok != "|" {
			return nil, p.errf("expected %q or %q, got %q", "}", "|", tok)
		}
		name, err := p.consumeToken()
		if err != nil {
			return nil, err
		}
		defn.Members = append(defn.Members, name)

		if tok, err := p.peekToken(); err == nil && tok == "(" {
			p.consumePeeked()
			key, err := p.consumeToken()
			if err != nil {
				return nil, err
			}
			reprKeys = append(reprKeys, key)
			if err := p.consumeRequired(")"); err != nil {
				return defn, err
			}
		} else {
			reprKeys = append(reprKeys, "")
		}
	}

	reprName := "string" // default repr
	if tok, err := p.peekToken(); err == nil && tok == "representation" {
		p.consumePeeked()
		name, err := p.consumeName()
		if err != nil {
			return nil, err
		}
		reprName = name
	}
	switch reprName {
	case "string":
		repr := &dmt.EnumRepresentation_String{}
		for i, key := range reprKeys {
			if key == "" {
				continue // no key; defaults to the name
			}
			if key[0] != '"' {
				return nil, p.errf("enum string representation used with non-string key: %s", key)
			}
			unquoted, err := strconv.Unquote(key)
			if err != nil {
				return nil, p.forwardError(err)
			}
			mapAppend(repr, defn.Members[i], unquoted)
		}
		defn.Representation.EnumRepresentation_String = repr
	case "int":
		repr := &dmt.EnumRepresentation_Int{}
		for i, key := range reprKeys {
			if key[0] != '"' {
				return nil, p.errf("enum int representation used with non-string key: %s", key)
			}
			unquoted, err := strconv.Unquote(key)
			if err != nil {
				return nil, p.forwardError(err)
			}
			parsed, err := strconv.Atoi(unquoted)
			if err != nil {
				return nil, p.forwardError(err)
			}
			mapAppend(repr, defn.Members[i], parsed)
		}
		defn.Representation.EnumRepresentation_Int = repr
	default:
		return nil, p.errf("unknown enum repr: %q", reprName)
	}
	return defn, nil
}
