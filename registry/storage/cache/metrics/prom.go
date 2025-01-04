package metrics

import (
	"context"
	"time"

	"github.com/distribution/distribution/v3"
	prometheus "github.com/distribution/distribution/v3/metrics"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/docker/go-metrics"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type prometheusCacheProvider struct {
	cache.BlobDescriptorCacheProvider
	latencyTimer metrics.LabeledTimer
}

func NewPrometheusCacheProvider(wrap cache.BlobDescriptorCacheProvider, name, help string) cache.BlobDescriptorCacheProvider {
	return &prometheusCacheProvider{
		wrap,
		// TODO: May want to have fine grained buckets since redis calls are generally <1ms and the default minimum bucket is 5ms.
		prometheus.StorageNamespace.NewLabeledTimer(name, help, "operation"),
	}
}

func (p *prometheusCacheProvider) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	start := time.Now()
	d, e := p.BlobDescriptorCacheProvider.Stat(ctx, dgst)
	p.latencyTimer.WithValues("Stat").UpdateSince(start)
	return d, e
}

func (p *prometheusCacheProvider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	start := time.Now()
	e := p.BlobDescriptorCacheProvider.SetDescriptor(ctx, dgst, desc)
	p.latencyTimer.WithValues("SetDescriptor").UpdateSince(start)
	return e
}

type prometheusRepoCacheProvider struct {
	distribution.BlobDescriptorService
	latencyTimer metrics.LabeledTimer
}

func (p *prometheusRepoCacheProvider) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	start := time.Now()
	d, e := p.BlobDescriptorService.Stat(ctx, dgst)
	p.latencyTimer.WithValues("RepoStat").UpdateSince(start)
	return d, e
}

func (p *prometheusRepoCacheProvider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	start := time.Now()
	e := p.BlobDescriptorService.SetDescriptor(ctx, dgst, desc)
	p.latencyTimer.WithValues("RepoSetDescriptor").UpdateSince(start)
	return e
}

func (p *prometheusCacheProvider) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	s, err := p.BlobDescriptorCacheProvider.RepositoryScoped(repo)
	if err != nil {
		return nil, err
	}
	return &prometheusRepoCacheProvider{
		s,
		p.latencyTimer,
	}, nil
}
