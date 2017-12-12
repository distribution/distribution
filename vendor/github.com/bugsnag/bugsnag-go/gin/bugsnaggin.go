package bugsnaggin

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/gin-gonic/gin"
)

const FrameworkName string = "Gin"

// AutoNotify sends any panics to bugsnag, and then re-raises them.
// You should use this after another middleware that
// returns an error page to the client, for example gin.Recovery().
// The arguments can be any RawData to pass to Bugsnag, most usually
// you'll pass a bugsnag.Configuration object.
func AutoNotify(rawData ...interface{}) gin.HandlerFunc {
	// Configure bugsnag with the passed in configuration (for manual notifications)
	for _, datum := range rawData {
		if c, ok := datum.(bugsnag.Configuration); ok {
			bugsnag.Configure(c)
		}
	}

	state := bugsnag.HandledState{
		bugsnag.SeverityReasonUnhandledMiddlewareError,
		bugsnag.SeverityError,
		true,
		FrameworkName,
	}
	rawData = append(rawData, state)
	return func(c *gin.Context) {
		r := c.Copy().Request

		// create a notifier that has the current request bound to it
		notifier := bugsnag.New(append(rawData, r)...)
		defer notifier.AutoNotify(r)
		c.Next()
	}
}
