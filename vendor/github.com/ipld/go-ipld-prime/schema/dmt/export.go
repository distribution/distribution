//go:build schemadmtgen
// +build schemadmtgen

package schemadmt

import "github.com/ipld/go-ipld-prime/schema"

func InternalTypeSystem() *schema.TypeSystem {
	return &schemaTypeSystem
}
