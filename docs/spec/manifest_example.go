package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Versioned provides a struct with just the manifest schemaVersion. Incoming
// content with unknown schema version can be decoded against this struct to
// check the version.
type Versioned struct {
	// SchemaVersion is the schema version that this content manifest follows.
	SchemaVersion int `json:"schemaVersion"`
}

// Dependency describes content which other content depends on. It may be a
// manifest for other distribution content or some other generic blob of data
// as described by the `MediaType` field.
type Dependency struct {
	MediaType string `json:"mediaType"`
	Length    uint64 `json:"length"`
	Digest    string `json:"digest"`
}

// Manifest provides the base accessible fields for working with distribution
// content.
type Manifest struct {
	Versioned
	Target       Dependency             `json:"target"`
	Dependencies []Dependency           `json:"dependencies"`
	Labels       map[string]interface{} `json:"labels"`
}

func main() {
	manifest := &Manifest{
		Versioned: Versioned{SchemaVersion: 2},
		Target: Dependency{
			MediaType: "application/vnd.docker.container.image.v1+json",
			Length:    7023,
			Digest:    "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7",
		},
		Dependencies: []Dependency{
			{
				MediaType: "application/vnd.docker.container.image.rootfs.diff+x-tar",
				Length:    32654,
				Digest:    "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
			},
			{
				MediaType: "application/vnd.docker.container.image.rootfs.diff+x-tar",
				Length:    16724,
				Digest:    "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
			},
			{
				MediaType: "application/vnd.docker.container.image.rootfs.diff+x-tar",
				Length:    73109,
				Digest:    "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
			},
		},
		Labels: map[string]interface{}{
			"createdAt": time.Now(),
			"version":   "3.1.4-a159+265",
		},
	}

	jsonManifest, err := json.MarshalIndent(manifest, "", "    ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Manifest Digest: sha256:%x\n", sha256.Sum256(jsonManifest))
	fmt.Println(string(jsonManifest))
}
