package mocks

import (
	"time"

	"github.com/docker/dhe-deploy/manager/schema"
)

type Store struct {
	*ManifestStore
	*TagStore
}

func NewStore() *Store {
	return &Store{
		&ManifestStore{},
		&TagStore{},
	}
}

func (Store) CreateEvent(event *schema.Event) error { return nil }
func (Store) GetEvents(requestedPageEncoded string, perPage uint, publishedBefore, publishedAfter *time.Time, queryingUserId, actorId, eventType string, isAdmin bool) (events []schema.Event, nextPageEncoded string, err error) {
	return []schema.Event{}, "", nil
}
func (Store) Subscribe(schema.EventReactor) chan bool {
	return nil
}
