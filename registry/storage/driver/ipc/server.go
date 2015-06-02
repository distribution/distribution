// +build ignore

package ipc

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"reflect"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/libchan"
	"github.com/docker/libchan/spdy"
)

// StorageDriverServer runs a new IPC server handling requests for the given
// storagedriver.StorageDriver
// This explicitly uses file descriptor 3 for IPC communication, as storage drivers are spawned in
// client.go
//
// To create a new out-of-process driver, create a main package which calls StorageDriverServer with
// a storagedriver.StorageDriver
func StorageDriverServer(driver storagedriver.StorageDriver) error {
	childSocket := os.NewFile(3, "childSocket")
	defer childSocket.Close()
	conn, err := net.FileConn(childSocket)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	if transport, err := spdy.NewServerTransport(conn); err != nil {
		panic(err)
	} else {
		for {
			receiver, err := transport.WaitReceiveChannel()
			if err == io.EOF {
				return nil
			} else if err != nil {
				panic(err)
			}
			go receive(driver, receiver)
		}
	}
}

// receive receives new storagedriver.StorageDriver method requests and creates a new goroutine to
// handle each request
// Requests are expected to be of type ipc.Request as the parameters are unknown until the request
// type is deserialized
func receive(driver storagedriver.StorageDriver, receiver libchan.Receiver) {
	for {
		var request Request
		err := receiver.Receive(&request)
		if err == io.EOF {
			return
		} else if err != nil {
			panic(err)
		}
		go handleRequest(driver, request)
	}
}

// handleRequest handles storagedriver.StorageDriver method requests as defined in client.go
// Responds to requests using the Request.ResponseChannel
func handleRequest(driver storagedriver.StorageDriver, request Request) {
	switch request.Type {
	case "Version":
		err := request.ResponseChannel.Send(&VersionResponse{Version: storagedriver.CurrentVersion})
		if err != nil {
			panic(err)
		}
	case "GetContent":
		path, _ := request.Parameters["Path"].(string)
		content, err := driver.GetContent(path)
		var response ReadStreamResponse
		if err != nil {
			response = ReadStreamResponse{Error: WrapError(err)}
		} else {
			response = ReadStreamResponse{Reader: ioutil.NopCloser(bytes.NewReader(content))}
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "PutContent":
		path, _ := request.Parameters["Path"].(string)
		reader, _ := request.Parameters["Reader"].(io.ReadCloser)
		contents, err := ioutil.ReadAll(reader)
		defer reader.Close()
		if err == nil {
			err = driver.PutContent(path, contents)
		}
		response := WriteStreamResponse{
			Error: WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "ReadStream":
		path, _ := request.Parameters["Path"].(string)
		// Depending on serialization method, Offset may be converted to any int/uint type
		offset := reflect.ValueOf(request.Parameters["Offset"]).Convert(reflect.TypeOf(int64(0))).Int()
		reader, err := driver.ReadStream(path, offset)
		var response ReadStreamResponse
		if err != nil {
			response = ReadStreamResponse{Error: WrapError(err)}
		} else {
			response = ReadStreamResponse{Reader: reader}
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "WriteStream":
		path, _ := request.Parameters["Path"].(string)
		// Depending on serialization method, Offset may be converted to any int/uint type
		offset := reflect.ValueOf(request.Parameters["Offset"]).Convert(reflect.TypeOf(int64(0))).Int()
		// Depending on serialization method, Size may be converted to any int/uint type
		size := reflect.ValueOf(request.Parameters["Size"]).Convert(reflect.TypeOf(int64(0))).Int()
		reader, _ := request.Parameters["Reader"].(io.ReadCloser)
		err := driver.WriteStream(path, offset, size, reader)
		response := WriteStreamResponse{
			Error: WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "CurrentSize":
		path, _ := request.Parameters["Path"].(string)
		position, err := driver.CurrentSize(path)
		response := CurrentSizeResponse{
			Position: position,
			Error:    WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "List":
		path, _ := request.Parameters["Path"].(string)
		keys, err := driver.List(path)
		response := ListResponse{
			Keys:  keys,
			Error: WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "Move":
		sourcePath, _ := request.Parameters["SourcePath"].(string)
		destPath, _ := request.Parameters["DestPath"].(string)
		err := driver.Move(sourcePath, destPath)
		response := MoveResponse{
			Error: WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "Delete":
		path, _ := request.Parameters["Path"].(string)
		err := driver.Delete(path)
		response := DeleteResponse{
			Error: WrapError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	default:
		panic(request)
	}
}
