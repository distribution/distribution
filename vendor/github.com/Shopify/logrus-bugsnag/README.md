## logrus-bugsnag

[![Build Status](https://travis-ci.org/Shopify/logrus-bugsnag.svg)](https://travis-ci.org/Shopify/logrus-bugsnag)

logrus-bugsnag is a hook that allows [Logrus](https://github.com/sirupsen/logrus) to interface with [Bugsnag](https://bugsnag.com).

#### Usage

```go
import (
  log "github.com/sirupsen/logrus"
  "github.com/Shopify/logrus-bugsnag"
  bugsnag "github.com/bugsnag/bugsnag-go"
)

func init() {
  bugsnag.Configure(bugsnag.Configuration{
    APIKey: apiKey,
  })
  hook, err := logrus_bugsnag.NewBugsnagHook()
  logrus.StandardLogger().Hooks.Add(hook)
}
```

