package mocks

import "github.com/stretchr/testify/mock"

import "github.com/docker/distribution"
import "github.com/docker/distribution/context"

type TagStore struct {
	mock.Mock
}

func (m *TagStore) GetTag(ctx context.Context, repo distribution.Repository, key string) (distribution.Descriptor, error) {
	ret := m.Called(ctx, repo, key)

	r0 := ret.Get(0).(distribution.Descriptor)
	r1 := ret.Error(1)

	return r0, r1
}
func (m *TagStore) PutTag(ctx context.Context, repo distribution.Repository, key string, val distribution.Descriptor) error {
	ret := m.Called(ctx, repo, key, val)

	r0 := ret.Error(0)

	return r0
}
func (m *TagStore) DeleteTag(ctx context.Context, repo distribution.Repository, key string) error {
	ret := m.Called(ctx, repo, key)

	r0 := ret.Error(0)

	return r0
}
func (m *TagStore) AllTags(ctx context.Context, repo distribution.Repository) ([]string, error) {
	ret := m.Called(ctx, repo)

	var r0 []string
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]string)
	}
	r1 := ret.Error(1)

	return r0, r1
}
func (m *TagStore) LookupTags(ctx context.Context, repo distribution.Repository, digest distribution.Descriptor) ([]string, error) {
	ret := m.Called(ctx, repo, digest)

	var r0 []string
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]string)
	}
	r1 := ret.Error(1)

	return r0, r1
}
