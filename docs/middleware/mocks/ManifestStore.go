package mocks

import "github.com/stretchr/testify/mock"

import "github.com/docker/distribution"
import "github.com/docker/distribution/context"

type ManifestStore struct {
	mock.Mock
}

func (m *ManifestStore) GetManifest(ctx context.Context, key string) ([]byte, error) {
	ret := m.Called(ctx, key)

	var r0 []byte
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]byte)
	}
	r1 := ret.Error(1)

	return r0, r1
}
func (m *ManifestStore) PutManifest(ctx context.Context, repo, digest string, val distribution.Manifest) error {
	ret := m.Called(ctx, repo, digest, val)

	r0 := ret.Error(0)

	return r0
}
func (m *ManifestStore) DeleteManifest(ctx context.Context, key string) error {
	ret := m.Called(ctx, key)

	r0 := ret.Error(0)

	return r0
}
