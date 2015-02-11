// +build ignore

package ipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"syscall"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/libchan"
	"github.com/docker/libchan/spdy"
)

// StorageDriverExecutablePrefix is the prefix which the IPC storage driver
// loader expects driver executables to begin with. For example, the s3 driver
// should be named "registry-storagedriver-s3".
const StorageDriverExecutablePrefix = "registry-storagedriver-"

// StorageDriverClient is a storagedriver.StorageDriver implementation using a
// managed child process communicating over IPC using libchan with a unix domain
// socket
type StorageDriverClient struct {
	subprocess *exec.Cmd
	exitChan   chan error
	exitErr    error
	stopChan   chan struct{}
	socket     *os.File
	transport  *spdy.Transport
	sender     libchan.Sender
	version    storagedriver.Version
}

// NewDriverClient constructs a new out-of-process storage driver using the
// driver name and configuration parameters
// A user must call Start on this driver client before remote method calls can
// be made
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

	driverExecName := StorageDriverExecutablePrefix + name
	driverPath, err := exec.LookPath(driverExecName)
	if err != nil {
		return nil, err
	}

	command := exec.Command(driverPath, string(paramsBytes))

	return &StorageDriverClient{
		subprocess: command,
	}, nil
}

// Start starts the designated child process storage driver and binds a socket
// to this process for IPC method calls
func (driver *StorageDriverClient) Start() error {
	driver.exitErr = nil
	driver.exitChan = make(chan error)
	driver.stopChan = make(chan struct{})

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

	go driver.handleSubprocessExit()

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
		return response.Error.Unwrap()
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
	if driver.stopChan != nil {
		close(driver.stopChan)
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

// GetContent retrieves the content stored at "path" as a []byte.
func (driver *StorageDriverClient) GetContent(path string) ([]byte, error) {
	if err := driver.exited(); err != nil {
		return nil, err
	}

	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "GetContent", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	response := new(ReadStreamResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error.Unwrap()
	}

	defer response.Reader.Close()
	contents, err := ioutil.ReadAll(response.Reader)
	if err != nil {
		return nil, err
	}
	return contents, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (driver *StorageDriverClient) PutContent(path string, contents []byte) error {
	if err := driver.exited(); err != nil {
		return err
	}

	receiver, remoteSender := libchan.Pipe()

	params := map[string]interface{}{"Path": path, "Reader": ioutil.NopCloser(bytes.NewReader(contents))}
	err := driver.sender.Send(&Request{Type: "PutContent", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	response := new(WriteStreamResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error.Unwrap()
	}

	return nil
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (driver *StorageDriverClient) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	if err := driver.exited(); err != nil {
		return nil, err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"Path": path, "Offset": offset}
	err := driver.sender.Send(&Request{Type: "ReadStream", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	response := new(ReadStreamResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error.Unwrap()
	}

	return response.Reader, nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a location
// designated by the given path.
func (driver *StorageDriverClient) WriteStream(path string, offset, size int64, reader io.ReadCloser) error {
	if err := driver.exited(); err != nil {
		return err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"Path": path, "Offset": offset, "Size": size, "Reader": reader}
	err := driver.sender.Send(&Request{Type: "WriteStream", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	response := new(WriteStreamResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error.Unwrap()
	}

	return nil
}

// CurrentSize retrieves the curernt size in bytes of the object at the given
// path.
func (driver *StorageDriverClient) CurrentSize(path string) (uint64, error) {
	if err := driver.exited(); err != nil {
		return 0, err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "CurrentSize", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return 0, err
	}

	response := new(CurrentSizeResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return 0, err
	}

	if response.Error != nil {
		return 0, response.Error.Unwrap()
	}

	return response.Position, nil
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (driver *StorageDriverClient) List(path string) ([]string, error) {
	if err := driver.exited(); err != nil {
		return nil, err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "List", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return nil, err
	}

	response := new(ListResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, response.Error.Unwrap()
	}

	return response.Keys, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (driver *StorageDriverClient) Move(sourcePath string, destPath string) error {
	if err := driver.exited(); err != nil {
		return err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"SourcePath": sourcePath, "DestPath": destPath}
	err := driver.sender.Send(&Request{Type: "Move", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	response := new(MoveResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error.Unwrap()
	}

	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (driver *StorageDriverClient) Delete(path string) error {
	if err := driver.exited(); err != nil {
		return err
	}

	receiver, remoteSender := libchan.Pipe()
	params := map[string]interface{}{"Path": path}
	err := driver.sender.Send(&Request{Type: "Delete", Parameters: params, ResponseChannel: remoteSender})
	if err != nil {
		return err
	}

	response := new(DeleteResponse)
	err = driver.receiveResponse(receiver, response)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return response.Error.Unwrap()
	}

	return nil
}

// handleSubprocessExit populates the exit channel until we have explicitly
// stopped the storage driver subprocess
// Requests can select on driver.exitChan and response receiving and not hang if
// the process exits
func (driver *StorageDriverClient) handleSubprocessExit() {
	exitErr := driver.subprocess.Wait()
	if exitErr == nil {
		exitErr = fmt.Errorf("Storage driver subprocess already exited cleanly")
	} else {
		exitErr = fmt.Errorf("Storage driver subprocess exited with error: %s", exitErr)
	}

	driver.exitErr = exitErr

	for {
		select {
		case driver.exitChan <- exitErr:
		case <-driver.stopChan:
			close(driver.exitChan)
			return
		}
	}
}

// receiveResponse populates the response value with the next result from the
// given receiver, or returns an error if receiving failed or the driver has
// stopped
func (driver *StorageDriverClient) receiveResponse(receiver libchan.Receiver, response interface{}) error {
	receiveChan := make(chan error, 1)
	go func(receiver libchan.Receiver, receiveChan chan<- error) {
		receiveChan <- receiver.Receive(response)
	}(receiver, receiveChan)

	var err error
	var ok bool
	select {
	case err = <-receiveChan:
	case err, ok = <-driver.exitChan:
		if !ok {
			err = driver.exitErr
		}
	}

	return err
}

// exited returns an exit error if the driver has exited or nil otherwise
func (driver *StorageDriverClient) exited() error {
	select {
	case err, ok := <-driver.exitChan:
		if !ok {
			return driver.exitErr
		}
		return err
	default:
		return nil
	}
}
