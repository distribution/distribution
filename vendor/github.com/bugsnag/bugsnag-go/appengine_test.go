// +build appengine

package bugsnag

import (
	"appengine/aetest"
)

func init() {
	c, err := aetest.NewContext(nil)
	if err != nil {
		panic(err)
	}

	OnBeforeNotify(func(event *Event, config *Configuration) error {

		event.RawData = append(event.RawData, c)

		return nil
	})
}
