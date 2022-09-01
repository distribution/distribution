package ufsdk

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

//MultipartState 用于保存分片上传的中间状态
type MultipartState struct {
	BlkSize  int //服务器返回的分片大小
	uploadID string
	mimeType string
	keyName  string
	etags    map[int]string
	mux      sync.Mutex
}

type Part struct {
	Etag    string
	Size    int
	PartNum int
	// LastModified int // 目前的需求里用不到。如果加上，[]Part 中每个元素的格式可能无法保持一致
}

type UploadIdDataSet struct {
	UploadId     string `json:"UploadId,omitempty"`
	FileName     string `json:"FileName,omitempty"`
	StartTime    int    `json:"StartTime,omitempty"`
	StroageClass string `json:"StroageClass,omitempty"`
}

type UploadIdResponse struct {
	NextMarker       string            `json:"NextMarker,omitempty"`
	UploadIdMarker   string            `json:"UploadIdMarker,omitempty"`
	NextUploadMarker string            `json:"NextUploadMarker,omitempty"`
	Prefix           string            `json:"Prefix,omitempty"`
	Limit            int               `json:"Limit,omitempty"`
	IsTruncated      bool              `json:"IsTruncated,omitempty"`
	DataSet          []UploadIdDataSet `json:"DataSet,omitempty"`
}

type UploadPartResponse struct {
	Key                  string `json:"Key,omitempty"`
	StroageClass         string `json:"UploadIdMarker,omitempty"`
	UploadId             string `json:"UploadId,omitempty"`
	Status               int    `json:"Status,omitempty"`
	IsTruncated          bool   `json:"IsTruncated,omitempty"`
	NextPartNumberMarker int    `json:"NextPartNumberMarker,omitempty"`
	Parts                []Part `json:"Parts,omitempty"`
}

func (m *MultipartState) GenerateMultipartState(blkSize int, uploadID, mimeType, keyName string, parts []*Part) {
	m.BlkSize = blkSize
	m.uploadID = uploadID
	m.mimeType = mimeType
	m.keyName = keyName
	m.etags = make(map[int]string)
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, part := range parts {
		m.etags[part.PartNum] = part.Etag
	}
}

//UnmarshalJSON custom unmarshal json
func (m *MultipartState) UnmarshalJSON(bytes []byte) error {
	tmp := struct {
		BlkSize  int    `json:"BlkSize"`
		UploadID string `json:"UploadId"`
	}{}
	err := json.Unmarshal(bytes, &tmp)
	if err != nil {
		return err
	}
	m.BlkSize = tmp.BlkSize
	m.uploadID = tmp.UploadID
	return nil
}

type uploadChan struct {
	etag string
	err  error
}

//MPut 分片上传一个文件，filePath 是本地文件所在的路径，内部会自动对文件进行分片上传，上传的方式是同步一片一片的上传。
//mimeType 如果为空的话，会调用 net/http 里面的 DetectContentType 进行检测。
//keyName 表示传到 ufile 的文件名。
//大于 100M 的文件推荐使用本接口上传。
func (u *UFileRequest) MPut(filePath, keyName, mimeType string) error {
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if mimeType == "" {
		mimeType = getMimeType(file)
	}

	state, err := u.InitiateMultipartUpload(keyName, mimeType)
	if err != nil {
		return err
	}

	chunk := make([]byte, state.BlkSize)
	var pos int
	for {
		bytesRead, fileErr := file.Read(chunk)
		if fileErr == io.EOF || bytesRead == 0 { //后面直接读到了结尾
			break
		}
		buf := bytes.NewBuffer(chunk[:bytesRead])
		err := u.UploadPart(buf, state, pos)
		if err != nil {
			u.AbortMultipartUpload(state)
			return err
		}
		pos++
	}

	return u.FinishMultipartUpload(state)
}

//AsyncMPut 异步分片上传一个文件，filePath 是本地文件所在的路径，内部会自动对文件进行分片上传，上传的方式是使用异步的方式同时传多个分片的块。
//mimeType 如果为空的话，会调用 net/http 里面的 DetectContentType 进行检测。
//keyName 表示传到 ufile 的文件名。
//大于 100M 的文件推荐使用本接口上传。
//同时并发上传的分片数量为10
func (u *UFileRequest) AsyncMPut(filePath, keyName, mimeType string) error {
	return u.AsyncUpload(filePath, keyName, mimeType, 10)
}

//AsyncUpload AsyncMPut 的升级版, jobs 表示同时并发的数量。
func (u *UFileRequest) AsyncUpload(filePath, keyName, mimeType string, jobs int) error {
	if jobs <= 0 {
		jobs = 1
	}

	if jobs >= 30 {
		jobs = 10
	}

	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if mimeType == "" {
		mimeType = getMimeType(file)
	}

	state, err := u.InitiateMultipartUpload(keyName, mimeType)
	if err != nil {
		return err
	}
	fsize := getFileSize(file)
	chunkCount := divideCeil(fsize, int64(state.BlkSize)) //向上取整
	concurrentChan := make(chan error, jobs)
	for i := 0; i != jobs; i++ {
		concurrentChan <- nil
	}

	wg := &sync.WaitGroup{}
	for i := 0; i != chunkCount; i++ {
		uploadErr := <-concurrentChan //最初允许启动 10 个 goroutine，超出10个后，有分片返回才会开新的goroutine.
		if uploadErr != nil {
			err = uploadErr
			break // 中间如果出现错误立即停止继续上传
		}
		wg.Add(1)
		go func(pos int) {
			defer wg.Done()
			offset := int64(state.BlkSize * pos)
			chunk := make([]byte, state.BlkSize)
			bytesRead, _ := file.ReadAt(chunk, offset)
			e := u.UploadPart(bytes.NewBuffer(chunk[:bytesRead]), state, pos)
			concurrentChan <- e //跑完一个 goroutine 后，发信号表示可以开启新的 goroutine。
		}(i)
	}
	wg.Wait()       //等待所有任务返回
	if err == nil { //再次检查剩余上传完的分片是否有错误
	loopCheck:
		for {
			select {
			case e := <-concurrentChan:
				err = e
				if err != nil {
					break loopCheck
				}
			default:
				break loopCheck
			}
		}
	}
	close(concurrentChan)
	if err != nil {
		u.AbortMultipartUpload(state)
		return err
	}

	return u.FinishMultipartUpload(state)
}

//AbortMultipartUpload 取消分片上传，如果掉用 UploadPart 出现错误，可以调用本函数取消分片上传。
//state 参数是 InitiateMultipartUpload 返回的
func (u *UFileRequest) AbortMultipartUpload(state *MultipartState) error {
	query := &url.Values{}
	query.Add("uploadId", state.uploadID)
	reqURL := u.genFileURL(state.keyName) + "?" + query.Encode()

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("DELETE", u.BucketName, state.keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//InitiateMultipartUpload 初始化分片上传，返回一个 state 用于后续的 UploadPart, FinishMultipartUpload, AbortMultipartUpload 的接口。
//
//keyName 表示传到 ufile 的文件名。
//
//mimeType 表示文件的 mimeType, 传空会报错，你可以使用 GetFileMimeType 方法检测文件的 mimeType。如果您上传的不是文件，您可以使用 http.DetectContentType https://golang.org/src/net/http/sniff.go?s=646:688#L11进行检测。
func (u *UFileRequest) InitiateMultipartUpload(keyName, mimeType string) (*MultipartState, error) {
	reqURL := u.genFileURL(keyName) + "?uploads"
	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return nil, err
	}
	//	if mimeType == "" {
	//		return nil, fmt.Errorf("Mime Type 不能为空！！！")
	//	}
	req.Header.Add("Content-Type", mimeType)
	for k, v := range u.RequestHeader {
		for i := 0; i < len(v); i++ {
			req.Header.Add(k, v[i])
		}
	}

	authorization := u.Auth.Authorization("POST", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)

	err = u.request(req)
	if err != nil {
		return nil, err
	}
	response := new(MultipartState)
	err = json.Unmarshal(u.LastResponseBody, response)
	if err != nil {
		return nil, err
	}
	response.keyName = keyName
	response.etags = make(map[int]string)
	response.mimeType = mimeType

	return response, err
}

//UploadPart 上传一个分片，buf 就是分片数据，buf 的数据块大小必须为 state.BlkSize，否则会报错。
//pardNumber 表示第几个分片，从 0 开始。例如一个文件按 state.BlkSize 分为 5 块，那么分片分别是 0,1,2,3,4。
//state 参数是 InitiateMultipartUpload 返回的
func (u *UFileRequest) UploadPart(buf *bytes.Buffer, state *MultipartState, partNumber int) error {
	query := &url.Values{}
	query.Add("uploadId", state.uploadID)
	query.Add("partNumber", strconv.Itoa(partNumber))

	reqURL := u.genFileURL(state.keyName) + "?" + query.Encode()
	req, err := http.NewRequest("PUT", reqURL, buf)
	if err != nil {
		return err
	}
	if u.verifyUploadMD5 {
		md5Str := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))
		req.Header.Add("Content-MD5", md5Str)
	}

	req.Header.Add("Content-Type", state.mimeType)
	authorization := u.Auth.Authorization("PUT", u.BucketName, state.keyName, req.Header)
	req.Header.Add("Authorization", authorization)
	req.Header.Add("Content-Length", strconv.Itoa(buf.Len()))

	resp, err := u.requestWithResp(req)
	if err != nil {
		return err
	}

	if !VerifyHTTPCode(resp.StatusCode) {
		err = u.responseParse(resp)
		if err != nil {
			return err
		}
		return fmt.Errorf("Remote response code is %d - %s not 2xx call DumpResponse(true) show details",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	defer resp.Body.Close()

	etag := strings.Trim(resp.Header.Get("Etag"), "\"") //为保证线程安全，这里就不保留 lastResponse
	if etag == "" {
		etag = strings.Trim(resp.Header.Get("ETag"), "\"") //为保证线程安全，这里就不保留 lastResponse
	}
	state.mux.Lock()
	state.etags[partNumber] = etag
	state.mux.Unlock()
	return nil
}

// 上传一个分片。构造并返回一个 Part
func (u *UFileRequest) UploadPartRetPart(buf *bytes.Buffer, state *MultipartState, partNumber int) (*Part, error) {
	size := buf.Len()

	query := &url.Values{}
	query.Add("uploadId", state.uploadID)
	query.Add("partNumber", strconv.Itoa(partNumber))

	reqURL := u.genFileURL(state.keyName) + "?" + query.Encode()
	req, err := http.NewRequest("PUT", reqURL, buf)
	if err != nil {
		return nil, err
	}
	if u.verifyUploadMD5 {
		md5Str := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))
		req.Header.Add("Content-MD5", md5Str)
	}

	req.Header.Add("Content-Type", state.mimeType)
	authorization := u.Auth.Authorization("PUT", u.BucketName, state.keyName, req.Header)
	req.Header.Add("Authorization", authorization)
	req.Header.Add("Content-Length", strconv.Itoa(buf.Len()))

	resp, err := u.requestWithResp(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	etag := strings.Trim(resp.Header.Get("Etag"), "\"") //为保证线程安全，这里就不保留 lastResponse
	if etag == "" {
		etag = strings.Trim(resp.Header.Get("ETag"), "\"") //为保证线程安全，这里就不保留 lastResponse
	}
	state.mux.Lock()
	state.etags[partNumber] = etag
	state.mux.Unlock()

	part := &Part{
		Etag:    etag,
		Size:    size,
		PartNum: partNumber,
	}
	return part, nil
}

//FinishMultipartUpload 完成分片上传。分片上传必须要调用的接口。
//state 参数是 InitiateMultipartUpload 返回的
func (u *UFileRequest) FinishMultipartUpload(state *MultipartState) error {
	query := &url.Values{}
	query.Add("uploadId", state.uploadID)
	reqURL := u.genFileURL(state.keyName) + "?" + query.Encode()
	var etagsStr string
	etagLen := len(state.etags)
	for i := 0; i != etagLen; i++ {
		etagsStr += state.etags[i]
		if i != etagLen-1 {
			etagsStr += ","
		}
	}

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(etagsStr))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", state.mimeType)
	authorization := u.Auth.Authorization("POST", u.BucketName, state.keyName, req.Header)
	req.Header.Add("Authorization", authorization)
	req.Header.Add("Content-Length", strconv.Itoa(len(etagsStr)))

	return u.request(req)
}

// 获取当前 bucket 正在进行分片上传的，但未 finish 的 upload 事件（即所有 multiState）
func (u *UFileRequest) GetMultiUploadId(prefix, marker, uploadIdMarker string, limit int) (list UploadIdResponse, err error) {
	query := &url.Values{}
	query.Add("muploadid", "")
	if prefix != "" {
		query.Add("prefix", prefix)
	}
	if marker != "" {
		query.Add("marker", marker)
	}
	if limit != 0 {
		query.Add("limit", strconv.Itoa(limit))
	}
	if uploadIdMarker != "" {
		query.Add("upload-id-marker", uploadIdMarker)
	}

	reqURL := u.baseURL.String() + "?" + query.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return
	}

	authorization := u.Auth.Authorization("GET", u.BucketName, "", req.Header)
	req.Header.Add("authorization", authorization)

	err = u.request(req)
	if err != nil {
		return
	}

	err = json.Unmarshal(u.LastResponseBody, &list)
	return
}

func (u *UFileRequest) GetMultiUploadPart(uploadId string, maxParts, partNumberMarker int) (parts []*Part, err error) {
	query := &url.Values{}
	query.Add("muploadpart", "")
	query.Add("uploadId", uploadId)
	if maxParts < 0 || maxParts > 1000 {
		maxParts = 1000
	}
	query.Add("max-parts", strconv.Itoa(maxParts))
	if partNumberMarker < 0 {
		partNumberMarker = 0
	}
	query.Add("part-number-marker", strconv.Itoa(partNumberMarker))

	reqURL := u.baseURL.String() + "?" + query.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return
	}

	authorization := u.Auth.Authorization("GET", u.BucketName, "", req.Header)
	req.Header.Add("authorization", authorization)

	err = u.request(req)
	if err != nil {
		return
	}

	var uploadPartResponse UploadPartResponse
	err = json.Unmarshal(u.LastResponseBody, &uploadPartResponse)
	if err != nil {
		return
	}

	for _, part := range uploadPartResponse.Parts {
		tmp := part
		parts = append(parts, &tmp)
	}
	return
}

func divideCeil(a, b int64) int {
	div := float64(a) / float64(b)
	c := math.Ceil(div)
	return int(c)
}
