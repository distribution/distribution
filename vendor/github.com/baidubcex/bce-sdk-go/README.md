# BCE SDK for Go

bce-sdk-go is the official BOS SDK for the Go programming language.

## Installing

If you are using Go 1.5 with the `GO15VENDOREXPERIMENT=1` vendoring flag, or 1.6 and higher you can use the following command to retrieve the SDK. The SDK's non-testing dependencies will be included and are vendored in the `vendor` folder.

    go get -u github.com/baidubcex/bce-sdk-go

Otherwise if your Go environment does not have vendoring support enabled, or you do not want to include the vendored SDK's dependencies you can use the following command to retrieve the SDK and its non-testing dependencies using `go get`.

    go get -u github.com/baidubcex/bce-sdk-go/baidubce/...

If you're looking to retrieve just the SDK without any dependencies use the following command.

    go get -d github.com/baidubcex/bce-sdk-go

These two processes will still include the `vendor` folder and it should be deleted if its not going to be used by your environment.

    rm -rf $GOPATH/src/github.com/baidubcex/bce-sdk-go/vendor

