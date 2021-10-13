package extension

import storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"

type Extension interface {
	Name() string
	Components() []string
}

type Store interface {
	StorageDriver() storagedriver.StorageDriver
}
