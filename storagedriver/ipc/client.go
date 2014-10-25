package ipc

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"syscall"

	"github.com/docker/libchan"
	"github.com/docker/libchan/spdy"
)

type StorageDriverClient struct {
	subprocess *exec.Cmd
	socket     *os.File
	transport  *spdy.Transport
	sender     libchan.Sender
}

func NewDriverClient(name string, parameters map[string]string) (*StorageDriverClient, error) {
	paramsBytes, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}

	driverPath := os.ExpandEnv(path.Join("$GOPATH", "bin", name))
	if _, err := os.Stat(driverPath); os.IsNotExist(err) {
		driverPath = path.Join(path.Dir(os.Args[0]), name)
	}
	if _, err := os.Stat(driverPath); os.IsNotExist(err) {
		driverPath, err = exec.LookPath(name)
		if err != nil {
			return nil, err
		}
	}

	command := exec.Command(driverPath, string(paramsBytes))

	return &StorageDriverClient{
		subprocess: command,
	}, nil
}

func (driver *StorageDriverClient) Start() error {
	fileDescriptors, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}

	childSocket := os.NewFile(uintptr(fileDescriptors[0]), "childSocket")
	parentSocket := os.NewFile(uintptr(fileDescriptors[1]), "parentSocket")

	driver.subprocess.Stdout = os.Stdout
	driver.subprocess.Stderr = os.Stderr
	driver.subprocess.ExtraFiles = []*os.File{childSocket}

	if err = driver.subprocess.Start(); err != nil {
		parentSocket.Close()
		return err
	}

	if err = childSocket.Close(); err != nil {
		parentSocket.Close()
		return err
	}

	connection, err := net.FileConn(parentSocket)
	if err != nil {
		parentSocket.Close()
		return err
	}
	transport, err := spdy.NewClientTransport(connection)
	if err != nil {
		parentSocket.Close()
		return err
	}
	sender, err := transport.NewSendChannel()
	if err != nil {
		transport.Close()
		parentSocket.Close()
		return err
	}

	driver.socket = parentSocket
	driver.transport = transport
	driver.sender = sender

	return nil
}

func (driver *StorageDriverClient) Stop() error {
	closeSenderErr := driver.sender.Close()
	closeTransportErr := driver.transport.Close()
	closeSocketErr := driver.socket.Close()
	killErr := driver.subprocess.Process.Kill()

	if closeSenderErr != nil {
		return closeSenderErr
	} else if closeTransportErr != nil {
		return closeTransportErr
	} else if closeSocketErr != nil {
		return closeSocketErr
	}
	return killErr
}

func (driver *StorageDriverClient) GetContent(path string) ([]byte, error) {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "GetContent", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	var response ReadStreamResponse
	err = receiver.Receive(&response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error
	}

	defer response.Reader.Close()
	contents, err := ioutil.ReadAll(response.Reader)
	if err != nil {
		return nil, err
	}
	return contents, nil
}

func (driver *StorageDriverClient) PutContent(path string, contents []byte) error {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path, "Reader": WrapReader(bytes.NewReader(contents))}
	err := driver.sender.Send(&Request{Type: "PutContent", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	var response WriteStreamResponse
	err = receiver.Receive(&response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error
	}

	return nil
}

func (driver *StorageDriverClient) ReadStream(path string, offset uint64) (io.ReadCloser, error) {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path, "Offset": offset}
	err := driver.sender.Send(&Request{Type: "ReadStream", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	var response ReadStreamResponse
	err = receiver.Receive(&response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error
	}

	return response.Reader, nil
}

func (driver *StorageDriverClient) WriteStream(path string, offset, size uint64, reader io.ReadCloser) error {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path, "Offset": offset, "Size": size, "Reader": WrapReader(reader)}
	err := driver.sender.Send(&Request{Type: "WriteStream", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	var response WriteStreamResponse
	err = receiver.Receive(&response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error
	}

	return nil
}

func (driver *StorageDriverClient) ResumeWritePosition(path string) (uint64, error) {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "ResumeWritePosition", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return 0, err
	}

	var response ResumeWritePositionResponse
	err = receiver.Receive(&response)
	if err != nil {
		return 0, err
	}

	if response.Error != nil {
		return 0, response.Error
	}

	return response.Position, nil
}

func (driver *StorageDriverClient) List(prefix string) ([]string, error) {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Prefix": prefix}
	err := driver.sender.Send(&Request{Type: "List", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	var response ListResponse
	err = receiver.Receive(&response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error
	}

	return response.Keys, nil
}

func (driver *StorageDriverClient) Move(sourcePath string, destPath string) error {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"SourcePath": sourcePath, "DestPath": destPath}
	err := driver.sender.Send(&Request{Type: "Move", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	var response MoveResponse
	err = receiver.Receive(&response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error
	}

	return nil
}

func (driver *StorageDriverClient) Delete(path string) error {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "Delete", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	var response DeleteResponse
	err = receiver.Receive(&response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error
	}

	return nil
}
