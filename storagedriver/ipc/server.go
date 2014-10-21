package ipc

import (
	"io"
	"net"
	"os"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/libchan"
	"github.com/docker/libchan/spdy"
)

func Server(driver storagedriver.StorageDriver) error {
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

func handleRequest(driver storagedriver.StorageDriver, request Request) {

	switch request.Type {
	case "GetContent":
		path, _ := request.Parameters["Path"].(string)
		content, err := driver.GetContent(path)
		response := GetContentResponse{
			Content: content,
			Error:   ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "PutContent":
		path, _ := request.Parameters["Path"].(string)
		contents, _ := request.Parameters["Contents"].([]byte)
		err := driver.PutContent(path, contents)
		response := PutContentResponse{
			Error: ResponseError(err),
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "ReadStream":
		var offset uint64

		path, _ := request.Parameters["Path"].(string)
		offset, ok := request.Parameters["Offset"].(uint64)
		if !ok {
			offsetSigned, _ := request.Parameters["Offset"].(int64)
			offset = uint64(offsetSigned)
		}
		reader, err := driver.ReadStream(path, offset)
		var response ReadStreamResponse
		if err != nil {
			response = ReadStreamResponse{Error: ResponseError(err)}
		} else {
			response = ReadStreamResponse{Reader: WrapReadCloser(reader)}
		}
		err = request.ResponseChannel.Send(&response)
		if err != nil {
			panic(err)
		}
	case "WriteStream":
		var offset uint64

		path, _ := request.Parameters["Path"].(string)
		offset, ok := request.Parameters["Offset"].(uint64)
		if !ok {
			offsetSigned, _ := request.Parameters["Offset"].(int64)
			offset = uint64(offsetSigned)
		}
		size, ok := request.Parameters["Size"].(uint64)
		if !ok {
			sizeSigned, _ := request.Parameters["Size"].(int64)
			size = uint64(sizeSigned)
		}
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
