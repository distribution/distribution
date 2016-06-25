// Refer to github.com/vladimirvivien/gowfs

package hdfsweb

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

const (
	webHdfsVer string = "/webhdfs/v1"
)

// FileSystem provides the HTTP client interface to specific HDFS server.
type FileSystem struct {
	NameNodeHost string
	NameNodePort string
	UserName     string
	BlockSize    int64
	BufferSize   int32
	Replication  int16
	Client       *http.Client
}

// Create creates a new file and stores its content in HDFS.
func (fs *FileSystem) Create(reader io.Reader, path string) error {
	v := url.Values{
		"user.name":   []string{fs.UserName},
		"op":          []string{"CREATE"},
		"overwrite":   []string{"true"},
		"blocksize":   []string{strconv.FormatInt(fs.BlockSize, 10)},
		"replication": []string{strconv.FormatInt(int64(fs.Replication), 10)},
		"permission":  []string{"0666"},
		"buffersize":  []string{strconv.FormatInt(int64(fs.BufferSize), 10)},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", u.String(), nil)
	if err != nil {
		return err
	}
	resp1, err := fs.Client.Transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusTemporaryRedirect {
		body, err := ioutil.ReadAll(resp1.Body)
		if err != nil {
			return err
		}
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Create] path(%s) unknown error return (%d)", path, resp1.StatusCode)
		}
	}
	location := resp1.Header.Get("Location")
	u, err = url.ParseRequestURI(location)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("PUT", u.String(), reader)
	if err != nil {
		return err
	}
	resp2, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(resp2.Body)
		if err != nil {
			return err
		}
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Create] path(%s) unknown error return (%d)", path, resp2.StatusCode)
		}
	}
	return nil
}

// Append appends reader content to exist file in HDFS.
func (fs *FileSystem) Append(reader io.Reader, path string) error {
	v := url.Values{
		"user.name":  []string{fs.UserName},
		"op":         []string{"APPEND"},
		"buffersize": []string{strconv.FormatInt(int64(fs.BufferSize), 10)},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return err
	}
	resp1, err := fs.Client.Transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusTemporaryRedirect {
		body, err := ioutil.ReadAll(resp1.Body)
		if err != nil {
			return err
		}
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Create] path(%s) unknown error return (%d)", path, resp1.StatusCode)
		}
	}
	location := resp1.Header.Get("Location")
	u, err = url.ParseRequestURI(location)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("POST", u.String(), reader)
	if err != nil {
		return err
	}
	resp2, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp2.Body)
		if err != nil {
			return err
		}
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Append] path(%s) unknown error return (%d)", path, resp2.StatusCode)
		}
	}
	return nil
}

//Open opens the specificed Path and returns its content to be accessed locally.
func (fs *FileSystem) Open(path string, offset, length int64) (io.ReadCloser, error) {
	v := url.Values{
		"user.name":  []string{fs.UserName},
		"op":         []string{"OPEN"},
		"offset":     []string{strconv.FormatInt(offset, 10)},
		"length":     []string{strconv.FormatInt(length, 10)},
		"buffersize": []string{strconv.FormatInt(int64(fs.BufferSize), 10)},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return nil, err
		} else if re.RemoteException.Exception != "" {
			return nil, re.RemoteException
		} else {
			return nil, fmt.Errorf("[Open] path(%s) unknown error return (%d)", path, resp.StatusCode)
		}
	}
	return resp.Body, nil
}

// Rename renames the specified path resource to a new name.
// See HDFS FileSystem.rename()
func (fs *FileSystem) Rename(source, destination string) error {
	if source == "" || destination == "" {
		return fmt.Errorf("[Rename] empty source or destination object name")
	}

	v := url.Values{
		"user.name":   []string{fs.UserName},
		"op":          []string{"RENAME"},
		"destination": []string{destination},
		"recursive":   []string{"true"},
	}
	u, err := fs.buildRequestURL(source, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Rename] from (%s) to (%s) unknown error return (%d)",
				source,
				destination,
				resp.StatusCode)
		}
	}

	var boolean struct {
		Boolean bool `json:"boolean,omitempty"`
	}
	err = json.Unmarshal(body, &boolean)
	if err != nil {
		return err
	} else if !boolean.Boolean {
		return ErrBoolean
	}
	return nil
}

//Delete deletes the specified path.
//See HDFS FileSystem.delete()
func (fs *FileSystem) Delete(path string) error {
	if path == "" {
		return fmt.Errorf("[Delete] empty path")
	}
	v := url.Values{
		"user.name": []string{fs.UserName},
		"op":        []string{"DELETE"},
		"recursive": []string{"true"},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Delete] path(%s) unknown error return (%d)", path, resp.StatusCode)
		}
	}

	var boolean struct {
		Boolean bool `json:"boolean,omitempty"`
	}
	err = json.Unmarshal(body, &boolean)
	if err != nil {
		return err
	} else if !boolean.Boolean {
		return ErrBoolean
	}
	return nil
}

//Truncate truncates  the specified path.
func (fs *FileSystem) Truncate(path string, length int64) error {
	if path == "" {
		return fmt.Errorf("[Truncate] empty path")
	}
	v := url.Values{
		"user.name": []string{fs.UserName},
		"op":        []string{"TRUNCATE"},
		"newlength": []string{strconv.FormatInt(length, 10)},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[Truncate] path(%s) unknown error return (%d)", path, resp.StatusCode)
		}
	}

	var boolean struct {
		Boolean bool `json:"boolean,omitempty"`
	}
	err = json.Unmarshal(body, &boolean)
	if err != nil {
		return err
	} else if !boolean.Boolean {
		return ErrBoolean
	}
	return nil
}

// MkDirs creates the specified directory(ies).
func (fs *FileSystem) MkDirs(path string) error {
	v := url.Values{
		"user.name":  []string{fs.UserName},
		"op":         []string{"MKDIRS"},
		"permission": []string{"0755"},
	}

	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return err
		} else if re.RemoteException.Exception != "" {
			return re.RemoteException
		} else {
			return fmt.Errorf("[MkDirs] path(%s)unknown error return (%d)", path, resp.StatusCode)
		}
	}

	var boolean struct {
		Boolean bool `json:"boolean,omitempty"`
	}
	err = json.Unmarshal(body, &boolean)
	if err != nil {
		return err
	} else if !boolean.Boolean {
		return ErrBoolean
	}

	return nil
}

// GetFileStatus returns status for a given file.  The Path must represent a FILE
// on the remote system.
func (fs *FileSystem) GetFileStatus(path string) (FileStatus, error) {
	v := url.Values{
		"user.name": []string{fs.UserName},
		"op":        []string{"GETFILESTATUS"},
	}
	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return FileStatus{}, err
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return FileStatus{}, err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return FileStatus{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return FileStatus{}, err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return FileStatus{}, err
		} else if re.RemoteException.Exception != "" {
			return FileStatus{}, re.RemoteException
		} else {
			return FileStatus{}, fmt.Errorf("[GetFileStatus] path(%s) unknown error return (%d)",
				path,
				resp.StatusCode)
		}
	}

	var fileStatus struct {
		FileStatus FileStatus `json:"FileStatus,omitempty"`
	}
	err = json.Unmarshal(body, &fileStatus)
	if err != nil {
		return FileStatus{}, err
	}
	return fileStatus.FileStatus, nil
}

// ListStatus returns an array of FileStatus for a given file directory.
// For details, see HDFS FileSystem.listStatus()
func (fs *FileSystem) ListStatus(path string) ([]FileStatus, error) {
	v := url.Values{
		"user.name": []string{fs.UserName},
		"op":        []string{"LISTSTATUS"},
	}
	u, err := fs.buildRequestURL(path, v)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		re := RespException{}
		if err := json.Unmarshal(body, &re); err != nil {
			return nil, err
		} else if re.RemoteException.Exception != "" {
			return nil, re.RemoteException
		} else {
			return nil, fmt.Errorf("[ListStatus] path(%s) unknown error return (%d)", path, resp.StatusCode)
		}
	}

	var fileStatuses struct {
		FileStatuses FileStatuses `json:"FileStatuses,omitempty"`
	}
	err = json.Unmarshal(body, &fileStatuses)
	if err != nil {
		return nil, err
	}
	return fileStatuses.FileStatuses.FileStatus, nil
}

// Builds the canonical URL used for remote request
func (fs *FileSystem) buildRequestURL(path string, v url.Values) (*url.URL, error) {
	urlStr := fmt.Sprintf("http://%s:%s%s", fs.NameNodeHost, fs.NameNodePort, webHdfsVer) + "?"
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	//prepare URL - add Path and "op" to URL
	if path[0] == '/' {
		u.Path = u.Path + path
	} else {
		u.Path = u.Path + "/" + path
	}
	u.RawQuery = v.Encode()
	return u, nil
}
