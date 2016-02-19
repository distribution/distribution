package proxy

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
)

// proxyTagService supports local and remote lookup of tags.
type proxyTagService struct {
	ctx        context.Context
	localTags  distribution.TagService
	remoteTags distribution.TagService
}

var _ distribution.TagService = proxyTagService{}

// Get attempts to get the most recent digest for the tag by checking the remote
// tag service first and then caching it locally.  If the remote is unavailable
// the local association is returned
func (pt proxyTagService) Get(tag string) (distribution.Descriptor, error) {
	desc, err := pt.remoteTags.Get(tag)
	if err == nil {
		err := pt.localTags.Tag(tag, desc)
		if err != nil {
			return distribution.Descriptor{}, err
		}
		return desc, nil
	}

	desc, err = pt.localTags.Get(tag)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	return desc, nil
}

func (pt proxyTagService) Tag(tag string, desc distribution.Descriptor) error {
	return distribution.ErrUnsupported
}

func (pt proxyTagService) Untag(tag string) error {
	err := pt.localTags.Untag(tag)
	if err != nil {
		return err
	}
	return nil
}

func (pt proxyTagService) All() ([]string, error) {
	tags, err := pt.remoteTags.All()
	if err == nil {
		return tags, err
	}
	return pt.localTags.All()
}

func (pt proxyTagService) Lookup(digest distribution.Descriptor) ([]string, error) {
	return []string{}, distribution.ErrUnsupported
}
