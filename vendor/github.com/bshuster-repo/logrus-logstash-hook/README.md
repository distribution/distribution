# Logstash hook for logrus <img src="http://i.imgur.com/hTeVwmJ.png" width="40" height="40" alt=":walrus:" class="emoji" title=":walrus:" />
[![Build Status](https://travis-ci.org/bshuster-repo/logrus-logstash-hook.svg?branch=master)](https://travis-ci.org/bshuster-repo/logrus-logstash-hook)
[![Go Report Status](https://goreportcard.com/badge/github.com/bshuster-repo/logrus-logstash-hook)](https://goreportcard.com/report/github.com/bshuster-repo/logrus-logstash-hook)

Use this hook to send the logs to [Logstash](https://www.elastic.co/products/logstash).

# Usage

```go
package main

import (
        "github.com/bshuster-repo/logrus-logstash-hook"
        "github.com/sirupsen/logrus"
        "net"
)

func main() {
        log := logrus.New()
        conn, err := net.Dial("tcp", "logstash.mycompany.net:8911")
        if err != nil {
                log.Fatal(err)
        }
        hook := logrustash.New(conn, logrustash.DefaultFormatter(logrus.Fields{"type": "myappName"}))

        log.Hooks.Add(hook)
        ctx := log.WithFields(logrus.Fields{
                "method": "main",
        })
        ctx.Info("Hello World!")
}

```

This is how it will look like:

```ruby
{
    "@timestamp" => "2016-02-29T16:57:23.000Z",
      "@version" => "1",
         "level" => "info",
       "message" => "Hello World!",
        "method" => "main",
          "host" => "172.17.0.1",
          "port" => 45199,
          "type" => "myappName"
}
```

# FAQ
Q: I would like to add characters to each line before sending to Logstash?
A: Logrustash gives you the ability to mutate the message before sending it to Logstash. Just follow [this example](https://github.com/bshuster-repo/logrus-logstash-hook/issues/60#issuecomment-604948272).

Q: Is there a way to maintain the connection when it drops
A: It's recommended to use [GoAutoSocket](https://github.com/firstrow/goautosocket) for that. See [here](https://github.com/bshuster-repo/logrus-logstash-hook/issues/48#issuecomment-361938249) how it can be done.

# Maintainers

Name         | Github    |
------------ | --------- |
Boaz Shuster | boaz0     |

# License

MIT.
