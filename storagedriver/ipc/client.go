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

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/libchan"
	"github.com/docker/libchan/spdy"
)

// StorageDriverClient is a storagedriver.StorageDriver implementation using a managed child process
// communicating over IPC using libchan with a unix domain socket
type StorageDriverClient struct {
	subprocess *exec.Cmd
	socket     *os.File
	transport  *spdy.Transport
	sender     libchan.Sender
	version    storagedriver.Version
}

// NewDriverClient constructs a new out-of-process storage driver using the driver name and
// configuration parameters
// A user must call Start on this driver client before remote method calls can be made
//
// Looks for drivers in the following locations in order:
// - Storage drivers directory (to be determined, yet not implemented)
// - $GOPATH/bin
// - $PATH
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

// Start starts the designated child process storage driver and binds a socket to this process for
// IPC method calls
func (driver *StorageDriverClient) Start() error {
	fileDescriptors, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}

	childSocket := os.NewFile(uintptr(fileDescriptors[0]), "childSocket")
	driver.socket = os.NewFile(uintptr(fileDescriptors[1]), "parentSocket")

	driver.subprocess.Stdout = os.Stdout
	driver.subprocess.Stderr = os.Stderr
	driver.subprocess.ExtraFiles = []*os.File{childSocket}

	if err = driver.subprocess.Start(); err != nil {
		driver.Stop()
		return err
	}

	if err = childSocket.Close(); err != nil {
		driver.Stop()
		return err
	}

	connection, err := net.FileConn(driver.socket)
	if err != nil {
		driver.Stop()
		return err
	}
	driver.transport, err = spdy.NewClientTransport(connection)
	if err != nil {
		driver.Stop()
		return err
	}
	driver.sender, err = driver.transport.NewSendChannel()
	if err != nil {
		driver.Stop()
		return err
	}

	// Check the driver's version to determine compatibility
	receiver, remoteSender := libchan.Pipe()
	err = driver.sender.Send(&Request{Type: "Version", ResponseChannel: remoteSender})
	if err != nil {
		driver.Stop()
		return err
	}

	var response VersionResponse
	err = receiver.Receive(&response)
	if err != nil {
		driver.Stop()
		return err
	}

	if response.Error != nil {
		return response.Error
	}

	driver.version = response.Version

	if driver.version.Major() != storagedriver.CurrentVersion.Major() || driver.version.Minor() > storagedriver.CurrentVersion.Minor() {
		return IncompatibleVersionError{driver.version}
	}

	return nil
}

// Stop stops the child process storage driver
// storagedriver.StorageDriver methods called after Stop will fail
func (driver *StorageDriverClient) Stop() error {
	var closeSenderErr, closeTransportErr, closeSocketErr, killErr error

	if driver.sender != nil {
		closeSenderErr = driver.sender.Close()
	}
	if driver.transport != nil {
		closeTransportErr = driver.transport.Close()
	}
	if driver.socket != nil {
		closeSocketErr = driver.socket.Close()
	}
	if driver.subprocess != nil {
		killErr = driver.subprocess.Process.Kill()
	}

	if closeSenderErr != nil {
		return closeSenderErr
	} else if closeTransportErr != nil {
		return closeTransportErr
	} else if closeSocketErr != nil {
		return closeSocketErr
	}
	return killErr
}

// Implement the storagedriver.StorageDriver interface over IPC

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

	params := map[string]interface{}{"Path": path, "Reader": ioutil.NopCloser(bytes.NewReader(contents))}
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

	params := map[string]interface{}{"Path": path, "Offset": offset, "Size": size, "Reader": ioutil.NopCloser(reader)}
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

func (driver *StorageDriverClient) List(path string) ([]string, error) {
	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path}
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
