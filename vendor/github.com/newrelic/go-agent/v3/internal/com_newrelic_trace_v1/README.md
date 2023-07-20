# com_newrelic_trace_v1

To generate the `v1.pb.go` code, run the following from the top level
`github.com/newrelic/go-agent` package:

```
protoc --go_out=paths=source_relative,plugins=grpc:. v3/internal/com_newrelic_trace_v1/v1.proto
```

Be mindful which version of `protoc-gen-go` you are using. Upgrade
`protoc-gen-go` to the latest with:

```
go get -u github.com/golang/protobuf/protoc-gen-go
```

## When you regenerate the file

Once you have generated the code, you will need to add a build tag to the file:

 ```go
// +build go1.9
```

This is because the gRPC/Protocol Buffer libraries only support Go 1.9 and
above.
