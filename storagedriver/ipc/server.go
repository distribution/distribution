package ipc

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"reflect"

	"github.com/docker/docker-registry/storagedriver"
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
			if err != nil {
				panic(err)
			}
			go receive(driver, receiver)
		}
		return nil
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
		if err != nil {
			panic(err)
		}
		go handleRequest(driver, request)
	}
}

// handleRequest handles storagedriver.StorageDriver method requests as defined in client.go
// Responds to requests using the Request.ResponseChannel
func handleRequest(driver storagedriver.StorageDriver, request Request) {
	switch request.Type {
	case "GetContent":
		path, _ := request.Parameters["Path"].(string)
		content, err := driver.GetContent(path)
		var response ReadStreamResponse
		if err != nil {
			response = ReadStreamResponse{Error: ResponseError(err)}
		} else {
			response = ReadStreamResponse{Reader: WrapReader(bytes.NewReader(content))}
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
			Error: ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "ReadStream":
		path, _ := request.Parameters["Path"].(string)
		// Depending on serialization method, Offset may be convereted to any int/uint type
		offset := reflect.ValueOf(request.Parameters["Offset"]).Convert(reflect.TypeOf(uint64(0))).Uint()
		reader, err := driver.ReadStream(path, offset)
		var response ReadStreamResponse
		if err != nil {
			response = ReadStreamResponse{Error: ResponseError(err)}
		} else {
			response = ReadStreamResponse{Reader: WrapReader(reader)}
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "WriteStream":
		path, _ := request.Parameters["Path"].(string)
		// Depending on serialization method, Offset may be convereted to any int/uint type
		offset := reflect.ValueOf(request.Parameters["Offset"]).Convert(reflect.TypeOf(uint64(0))).Uint()
		// Depending on serialization method, Size may be convereted to any int/uint type
		size := reflect.ValueOf(request.Parameters["Size"]).Convert(reflect.TypeOf(uint64(0))).Uint()
		reader, _ := request.Parameters["Reader"].(io.ReadCloser)
		err := driver.WriteStream(path, offset, size, reader)
		response := WriteStreamResponse{
			Error: ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "ResumeWritePosition":
		path, _ := request.Parameters["Path"].(string)
		position, err := driver.ResumeWritePosition(path)
		response := ResumeWritePositionResponse{
			Position: position,
			Error:    ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "List":
		prefix, _ := request.Parameters["Prefix"].(string)
		keys, err := driver.List(prefix)
		response := ListResponse{
			Keys:  keys,
			Error: ResponseError(err),
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
			Error: ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "Delete":
		path, _ := request.Parameters["Path"].(string)
		err := driver.Delete(path)
		response := DeleteResponse{
			Error: ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	default:
		panic(request)
	}
}
