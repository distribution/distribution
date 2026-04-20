package challenge

import (
	"net/http"
	"net/url"
)

// FilteringManager decorates another Manager and drops challenges that do not
// satisfy the configured predicate.
type FilteringManager struct {
	base Manager
	keep func(Challenge) bool
}

// NewFilteringManager returns a Manager that delegates storage to base and
// filters challenges on reads. If keep is nil, the base manager is returned.
func NewFilteringManager(base Manager, keep func(Challenge) bool) Manager {
	if keep == nil {
		return base
	}

	return FilteringManager{
		base: base,
		keep: keep,
	}
}

func (m FilteringManager) GetChallenges(endpoint url.URL) ([]Challenge, error) {
	challenges, err := m.base.GetChallenges(endpoint)
	if err != nil {
		return nil, err
	}

	filtered := make([]Challenge, 0, len(challenges))
	for _, c := range challenges {
		if m.keep(c) {
			filtered = append(filtered, c)
		}
	}

	return filtered, nil
}

func (m FilteringManager) AddResponse(resp *http.Response) error {
	return m.base.AddResponse(resp)
}
