package tdfs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	tdfsfilesystem "github.com/giobart/2dfs-builder/filesystem"
)

type Partition struct {
	x1 int
	y1 int
	x2 int
	y2 int
}

const (
	//semantic tag partition init char
	partitionInit = `--`
	//semantic tag partition split char
	partitionSplitChar = `.`
	//semantic partition regex patter
	semanticTagPattern = partitionInit + `\d+\` + partitionSplitChar + `\d+\` + partitionSplitChar + `\d+\` + partitionSplitChar + `\d+`
)

// CheckTagPartitions checks if the tag contains semantic partitions and returns the tag and the partitions
func CheckTagPartitions(tag string) (string, []Partition) {
	partitions := []Partition{}
	onlyTag := tag
	re := regexp.MustCompile(semanticTagPattern)
	matches := re.FindAllString(tag, -1)

	if len(matches) > 0 {
		onlyTag = strings.Split(tag, partitionInit)[0]
		//semantic tag with partition
		log.Default().Printf("Semantic tag with partition detected %s\n", tag)
		for _, match := range matches {
			part, err := parsePartition(strings.Replace(match, partitionInit, "", -1))
			if err != nil {
				log.Default().Printf("[WARNING] Invalid partition %s, skipping...\n", match)
				continue
			}
			partitions = append(partitions, part)
			log.Default().Printf("[PARTITIONING...] Added partition %s \n", part)
		}
	}
	return onlyTag, partitions
}

func parsePartition(p string) (Partition, error) {
	parts := strings.Split(p, partitionSplitChar)
	result := Partition{}
	if len(parts) != 4 {
		return result, fmt.Errorf("invalid partition %s", p)
	}
	var err error
	result.x1, err = strconv.Atoi(parts[0])
	if err != nil {
		return result, err
	}
	result.y1, err = strconv.Atoi(parts[1])
	if err != nil {
		return result, err
	}
	result.x2, err = strconv.Atoi(parts[2])
	if err != nil {
		return result, err
	}
	result.y2, err = strconv.Atoi(parts[3])
	if err != nil {
		return result, err
	}
	return result, nil
}

func ConvertTdfsManifestToOciManifest(ctx context.Context, tdfsManifest *ocischema.DeserializedManifest, blobService distribution.BlobService, partitions []Partition) (distribution.Manifest, error) {

	log.Default().Printf("Converting TDFS manifest to OCI manifest\n")
	newLayers := []distribution.Descriptor{}
	partitionAllotment := []tdfsfilesystem.Allotment{}
	layerConfigBlob, err := blobService.Get(ctx, tdfsManifest.Config.Digest)
	if err != nil {
		log.Default().Printf("Error getting config %s\n", tdfsManifest.Config.Digest)
		return nil, err
	}
	var config v1.Image = v1.Image{}
	err = json.Unmarshal(layerConfigBlob, &config)
	if err != nil {
		log.Default().Printf("Error unmarshalling config %s\n", tdfsManifest.Config.Digest)
		return nil, err
	}

	//select partitions
	for _, layer := range tdfsManifest.Layers {
		if layer.MediaType == MediaTypeTdfsLayer {
			log.Default().Printf("Converting tdfs layer %s\n", layer.Digest)
			layerContent, err := blobService.Get(ctx, layer.Digest)
			if err != nil {
				log.Default().Printf("Error getting layer %s\n", layer.Digest)
				return nil, err
			}
			field, err := tdfsfilesystem.GetField().Unmarshal(string(layerContent))
			if err != nil {
				log.Default().Printf("Error unmarshalling layer %s\n", layer.Digest)
				return nil, err
			}
			if len(partitionAllotment) == 0 {
				log.Default().Printf("Adding field!!\n")
				if field != nil {
					for allotment := range field.IterateAllotments() {
						//skip empty allotments
						if allotment.Digest == "" {
							continue
						}
						for _, p := range partitions {
							if allotment.Row >= p.x1 && allotment.Row <= p.x2 && allotment.Col >= p.y1 && allotment.Col <= p.y2 {
								log.Default().Printf("Added partition %d,%d,%d,%d \n", p.x1, p.y1, p.x2, p.y2)
								partitionAllotment = append(partitionAllotment, allotment)
								//TODO remove duplicated
							}
						}
					}
				}
			}
		} else {
			log.Default().Printf("Appended layer %s\n", layer.Digest)
			newLayers = append(newLayers, layer)
		}
	}

	//create new layers
	if len(partitionAllotment) > 0 {
		//adding partitioned layers
		for _, p := range partitionAllotment {
			blob, err := blobService.Stat(ctx, digest.Digest(fmt.Sprintf("sha256:%s", p.Digest)))
			if err != nil {
				log.Default().Printf("Unable to find allotment %s\n", p.Digest)
				return nil, err
			}
			fmt.Printf("Partition %s [CREATING]\n", p.Digest)
			newLayers = append(newLayers, distribution.Descriptor{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    digest.Digest(fmt.Sprintf("sha256:%s", p.Digest)),
				Size:      blob.Size,
			})
			config.RootFS.DiffIDs = append(config.RootFS.DiffIDs, digest.Digest(fmt.Sprintf("sha256:%s", p.DiffID)))
		}
		log.Default().Printf("Allotments added!\n")
	}

	newConfig, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	//create new manifest
	manifestBuilder := ocischema.NewManifestBuilder(blobService, newConfig, tdfsManifest.Annotations)
	manifestBuilder.SetMediaType(v1.MediaTypeImageManifest)
	for _, layer := range newLayers {
		manifestBuilder.AppendReference(layer)
	}
	return manifestBuilder.Build(ctx)
}

func ConvertPartitionedIndexToOciIndex(tdfsManifest *ocischema.DeserializedImageIndex) ([]byte, error) {
	log.Default().Printf("Converting partitioned index to OCI index\n")
	return tdfsManifest.MarshalJSON()
}
