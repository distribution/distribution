package speedy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"
)

//Client is the client for speedy.
type Client struct {
}

//MetaInfoValue is meta info format of speedy.
type MetaInfoValue struct {
	Index   uint64
	Start   uint64
	End     uint64
	IsLast  bool
	IsDir   bool `json:",omitempty"`
	ModTime time.Time
}

//OrderByIndex is Used to sort metainfo by index
//E.g. we may get a metainfo array for a special path, so we need to sort this metainfo array.
type OrderByIndex []*MetaInfoValue

const (
	headerSourcePath = "Source-Path"
	headerDestPath   = "Dest-Path"
	headerPath       = "Path"
	headerIndex      = "Fragment-Index"
	headerRange      = "Bytes-Range"
	headerIsLast     = "Is-Last"
	headerVersion    = "Registry-Version"
	fragmentInfo     = "fragment-info"
	fileList         = "file-list"
	pathDescendant   = "path-descendant"
)

func (a OrderByIndex) Len() int {
	return len(a)
}

func (a OrderByIndex) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a OrderByIndex) Less(i, j int) bool {
	return a[i].Index < a[j].Index
}

func (c *Client) doRequest(req *http.Request) ([]byte, int, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		if resp != nil {
			return nil, resp.StatusCode, err
		}
		return nil, http.StatusNotFound, err
	}

	dataBody, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return dataBody, resp.StatusCode, nil
}

func (c *Client) getMetaInfoValueFromJSON(data []byte) ([]*MetaInfoValue, error) {
	var mapResult map[string]interface{}
	err := json.Unmarshal(data, &mapResult)
	if err != nil {
		return nil, err
	}

	fragmentInfoValue, ok := mapResult[fragmentInfo]
	if !ok {
		return nil, fmt.Errorf("Response format maybe error: %v", mapResult)
	}

	infoArr, ok := fragmentInfoValue.([]interface{})
	if !ok {
		return nil, fmt.Errorf("Response format maybe error, value is not a array: %v", fragmentInfoValue)
	}

	var result []*MetaInfoValue
	for _, info := range infoArr {
		m, ok := info.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, infoArr: %v", infoArr)
		}

		miv := new(MetaInfoValue)
		index, ok := m["Index"].(float64)
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
		}
		miv.Index = uint64(index)

		start, ok := m["Start"].(float64)
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
		}
		miv.Start = uint64(start)

		end, ok := m["End"].(float64)
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
		}
		miv.End = uint64(end)

		isLast, ok := m["IsLast"].(bool)
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
		}
		miv.IsLast = isLast

		isDirTemp, ok := m["IsDir"]
		if ok {
			isDir, ok := isDirTemp.(bool)
			if !ok {
				return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
			}
			miv.IsDir = isDir
		}

		modTimeStr, ok := m["ModTime"].(string)
		if !ok {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v", m)
		}
		modTime, err := time.Parse(time.RFC3339Nano, modTimeStr)
		if err != nil {
			return nil, fmt.Errorf("Response format maybe error, MetaInfoValue: %v, error: %v", m, err)
		}
		miv.ModTime = modTime

		result = append(result, miv)
	}
	return result, nil
}

func (c *Client) sortMetaInfoValue(origin []*MetaInfoValue) ([]*MetaInfoValue, error) {
	sort.Sort(OrderByIndex(origin))
	return origin, nil
}

//GetFileInfo is used to get meta info from speedy by path.
func (c *Client) GetFileInfo(url string, path string) ([]*MetaInfoValue, error) {
	req, err := http.NewRequest("GET", url+"/v1/fileinfo", nil)
	if err != nil {
		return nil, err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	req.Header = header
	dataBody, statusCode, err := c.doRequest(req)
	if err == nil && statusCode == http.StatusOK {
		infoArr, err := c.getMetaInfoValueFromJSON(dataBody)
		if err != nil {
			return nil, err
		}
		infoArr, err = c.sortMetaInfoValue(infoArr)
		if err != nil {
			return nil, err
		}
		return infoArr, nil
	}

	if err == nil && statusCode == http.StatusNotFound {
		return nil, nil
	}

	return nil, fmt.Errorf("GetFileInfo failed, statusCode: %d, err: %v", statusCode, err)
}

//DownloadFile is used to download file from speedy by path and meta info.
func (c *Client) DownloadFile(url string, path string, info *MetaInfoValue) ([]byte, error) {
	req, err := http.NewRequest("GET", url+"/v1/file", nil)
	if err != nil {
		return nil, err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	header.Set(headerIndex, fmt.Sprintf("%d", info.Index))
	header.Set(headerRange, fmt.Sprintf("%d-%d", info.Start, info.End))
	header.Set(headerIsLast, fmt.Sprintf("%v", info.IsLast))
	req.Header = header
	dataBody, statusCode, err := c.doRequest(req)
	if err == nil && statusCode == http.StatusOK {
		return dataBody, nil
	}
	return nil, fmt.Errorf("DownloadFile failed, statusCode: %d, err: %v", statusCode, err)
}

//GetDirectoryInfo is used to get directory info from speedy by path.
func (c *Client) GetDirectoryInfo(url string, path string) ([]string, error) {
	req, err := http.NewRequest("GET", url+"/v1/list_directory", nil)
	if err != nil {
		return nil, err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	req.Header = header
	dataBody, statusCode, err := c.doRequest(req)
	if err == nil && statusCode == http.StatusOK {
		var mapResult map[string][]string
		err := json.Unmarshal(dataBody, &mapResult)
		if err != nil {
			return nil, err
		}

		result, ok := mapResult[fileList]
		if !ok {
			return nil, fmt.Errorf("Response format maybe error: %v", mapResult)
		}

		return result, nil
	}

	if err == nil && statusCode == http.StatusNotFound {
		return nil, nil
	}

	return nil, fmt.Errorf("GetDirectoryInfo failed, statusCode: %d, err: %v", statusCode, err)
}

//GetDirectDescendantPath is used to get direct descendants path from speedy by path.
func (c *Client) GetDirectDescendantPath(url string, path string) ([]string, error) {
	req, err := http.NewRequest("GET", url+"/v1/list_descendant", nil)
	if err != nil {
		return nil, err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	req.Header = header
	dataBody, statusCode, err := c.doRequest(req)
	if err == nil && statusCode == http.StatusOK {
		var mapResult map[string][]string
		err := json.Unmarshal(dataBody, &mapResult)
		if err != nil {
			return nil, err
		}

		tempResult, ok := mapResult[pathDescendant]
		if !ok {
			return nil, fmt.Errorf("Response format maybe error: %v", mapResult)
		}

		result, err := c.directDescendPath(path, tempResult)
		if err != nil {
			return nil, err
		}

		return result, nil
	}

	if err != nil && statusCode == http.StatusNotFound {
		return nil, nil
	}

	return nil, fmt.Errorf("GetDescendantPath failed, statusCode: %d, err: %v", statusCode, err)
}

// directDescendPath will find direct descendants of the prefix and will return their full paths.
// Example: direct descendants of "/" in {"/foo", "/bar/1", "/bar/2"} is
// {"/foo", "/bar"} and direct descendants of "bar" is {"/bar/1", "/bar/2"}
func (c *Client) directDescendPath(prefix string, descendants []string) ([]string, error) {
	if descendants == nil || len(descendants) == 0 {
		return nil, fmt.Errorf("descendants is empty")
	}

	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	out := make(map[string]bool)
	for _, path := range descendants {
		if strings.HasPrefix(path, prefix) {
			rel := path[len(prefix):]
			c := strings.Count(rel, "/")
			if c == 0 {
				out[path] = true
			} else {
				out[prefix+rel[:strings.Index(rel, "/")]] = true
			}
		}
	}

	var keys []string
	for k := range out {
		keys = append(keys, k)
	}
	return keys, nil
}

//UploadFile is used to get file to speedy by path and meta info.
func (c *Client) UploadFile(url string, path string, info *MetaInfoValue, data []byte) error {
	req, err := http.NewRequest("POST", url+"/v1/file", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	header.Set(headerIndex, fmt.Sprintf("%d", info.Index))
	header.Set(headerRange, fmt.Sprintf("%d-%d", info.Start, info.End))
	header.Set(headerIsLast, fmt.Sprintf("%v", info.IsLast))
	req.Header = header
	_, statusCode, err := c.doRequest(req)
	if statusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("UploadFile failed, statusCode: %d, error: %v", statusCode, err)
}

//Ping with speedy used to check the health of speedy.
func (c *Client) Ping(url string) error {
	req, err := http.NewRequest("POST", url+"/v1/_ping", nil)
	if err != nil {
		return err
	}

	_, statusCode, err := c.doRequest(req)
	if statusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("Ping failed, statusCode: %d, error: %v", statusCode, err)
}

//DeleteFile is used to delete file from speedy by path.
func (c *Client) DeleteFile(url string, path string) error {
	req, err := http.NewRequest("DELETE", url+"/v1/file", nil)
	if err != nil {
		return err
	}

	header := make(http.Header)
	header.Set(headerPath, path)
	req.Header = header
	_, statusCode, err := c.doRequest(req)
	if statusCode == http.StatusNoContent {
		return nil
	}

	return fmt.Errorf("DeleteFile failed, statusCode: %d, error: %v", statusCode, err)
}

//MoveFile is used to move file in speedy from source path to destination path.
func (c *Client) MoveFile(url string, sourcePath string, destPath string) error {
	req, err := http.NewRequest("POST", url+"/v1/move", nil)
	if err != nil {
		return err
	}

	header := make(http.Header)
	header.Set(headerSourcePath, sourcePath)
	header.Set(headerDestPath, destPath)
	req.Header = header
	_, statusCode, err := c.doRequest(req)
	if statusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("MoveFile failed, statusCode: %d, error: %v", statusCode, err)
}
