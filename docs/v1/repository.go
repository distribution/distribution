package v1

import (
	"time"

	"github.com/docker/engine-api/types"
)

type (
	Tag struct {
		ID   string
		Name string
	}

	ContainerConfig struct {
		types.ContainerJSON
		Cmd []string `json:"Cmd,omitempty"`
	}

	Layer struct {
		ID              string           `json:"id,omitempty"`
		Parent          string           `json:"parent,omitempty"`
		Created         *time.Time       `json:"created,omitempty"`
		Container       string           `json:"container,omitempty"`
		ContainerConfig *ContainerConfig `json:"container_config,omitempty"`
		DockerVersion   string           `json:"docker_version,omitempty"`
		Author          string           `json:"author,omitempty"`
		Architecture    string           `json:"architecture,omitempty"`
		OS              string           `json:"os,omitempty"`
		Size            int64            `json:"size,omitempty"`
		Ancestry        []string         `json:"ancestry,omitempty"`
	}

	Repository struct {
		Description string  `json:"description,omitempty"`
		Name        string  `json:"name,omitempty"`
		Namespace   string  `json:"namespace,omitempty"`
		Repository  string  `json:"repository,omitempty"`
		Tags        []Tag   `json:"tags,omitempty"`
		Layers      []Layer `json:"layers,omitempty"`
		Size        int64   `json:"size,omitempty"`
		// these are only for the official index
		Trusted   bool `json:"is_trusted,omitempty"`
		Official  bool `json:"is_official,omitempty"`
		StarCount int  `json:"star_count,omitempty"`
	}
)
