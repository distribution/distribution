package ufsdk

import (
	"bytes"
	"encoding/base64"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	fourMegabyte = 1 << 22 //4M
)

//FileDataSet  用于 FileListResponse 里面的 DataSet 字段。
type FileDataSet struct {
	BucketName    string `json:"BucketName,omitempty"`
	FileName      string `json:"FileName,omitempty"`
	Hash          string `json:"Hash,omitempty"`
	MimeType      string `json:"MimeType,omitempty"`
	FirstObject   string `json:"first_object,omitempty"`
	Size          int    `json:"Size,omitempty"`
	CreateTime    int    `json:"CreateTime,omitempty"`
	ModifyTime    int    `json:"ModifyTime,omitempty"`
	StorageClass  string `json:"StorageClass,omitempty"`
	RestoreStatus string `json:"RestoreStatus,omitempty"`
}

//FileListResponse 用 PrefixFileList 接口返回的 list 数据。
type FileListResponse struct {
	BucketName string        `json:"BucketName,omitempty"`
	BucketID   string        `json:"BucketId,omitempty"`
	NextMarker string        `json:"NextMarker,omitempty"`
	DataSet    []FileDataSet `json:"DataSet,omitempty"`
}

func (f FileListResponse) String() string {
	return structPrettyStr(f)
}

//ListObjectsResponse 用 ListObjects 接口返回的 list 数据。
//Name Bucket名称
//Prefix 查询结果的前缀
//MaxKeys 查询结果的最大数量
//Delimiter 查询结果的目录分隔符
//IsTruncated 返回结果是否被截断。若值为true，则表示仅返回列表的一部分，NextMarker可作为之后迭代的游标
//NextMarker 可作为查询请求中的的Marker参数，实现迭代查询
//Contents 文件列表
//CommonPrefixes 以Delimiter结尾，并且有共同前缀的目录列表
type ListObjectsResponse struct {
	Name           string          `json:"Name,omitempty"`
	Prefix         string          `json:"Prefix,omitempty"`
	MaxKeys        string          `json:"MaxKeys,omitempty"`
	Delimiter      string          `json:"Delimiter,omitempty"`
	IsTruncated    bool            `json:"IsTruncated,omitempty"`
	NextMarker     string          `json:"NextMarker,omitempty"`
	Contents       []ObjectInfo    `json:"Contents,omitempty"`
	CommonPrefixes []CommonPreInfo `json:"CommonPrefixes,omitempty"`
}

func (f ListObjectsResponse) String() string {
	return structPrettyStr(f)
}

//ObjectInfo 用于 ListObjectsResponse 里面的 Contents 字段
//Key 文件名称
//MimeType 文件mimetype
//LastModified 文件最后修改时间
//CreateTime 文件创建时间
//ETag 标识文件内容
//Size 文件大小
//StorageClass 文件存储类型
//UserMeta 用户自定义元数据
type ObjectInfo struct {
	Key          string            `json:"Key,omitempty"`
	MimeType     string            `json:"MimeType,omitempty"`
	LastModified int               `json:"LastModified,omitempty"`
	CreateTime   int               `json:"CreateTime,omitempty"`
	Etag         string            `json:"Etag,omitempty"`
	Size         string            `json:"Size,omitempty"`
	StorageClass string            `json:"StorageClass,omitempty"`
	UserMeta     map[string]string `json:"UserMeta,omitempty"`
}

//CommonPreInfo 用于 ListObjectsResponse 里面的 CommonPrefixes 字段
//Prefix 以Delimiter结尾的公共前缀目录名
type CommonPreInfo struct {
	Prefix string `json:"Prefix,omitempty"`
}

//UploadHit 文件秒传，它的原理是计算出文件的 etag 值与远端服务器进行对比，如果文件存在就快速返回。
func (u *UFileRequest) UploadHit(filePath, keyName string) (err error) {
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fsize := getFileSize(file)
	etag := calculateEtag(file)

	query := &url.Values{}
	query.Add("Hash", etag)
	query.Add("FileName", keyName)
	query.Add("FileSize", strconv.FormatInt(fsize, 10))
	reqURL := u.genFileURL("uploadhit") + "?" + query.Encode()
	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("POST", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)

	return u.request(req)
}

//PostFile 使用 HTTP Form 的方式上传一个文件。
//注意：使用本接口上传文件后，调用 UploadHit 接口会返回 404，因为经过 form 包装的文件，etag 值会不一样，所以会调用失败。
//mimeType 如果为空的话，会调用 net/http 里面的 DetectContentType 进行检测。
//keyName 表示传到 ufile 的文件名。
//小于 100M 的文件推荐使用本接口上传。
func (u *UFileRequest) PostFile(filePath, keyName, mimeType string) (err error) {
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	h := make(http.Header)
	for k, v := range u.RequestHeader {
		for i := 0; i < len(v); i++ {
			h.Add(k, v[i])
		}
	}
	if mimeType == "" {
		mimeType = getMimeType(file)
	}
	h.Add("Content-Type", mimeType)

	var md5Str string
	if u.verifyUploadMD5 {
		f, err := openFile(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		md5Str = fmt.Sprintf("%x", md5.Sum(b))
		fmt.Println("md5Str:", md5Str)
		h.Add("Content-MD5", md5Str)
	}

	authorization := u.Auth.Authorization("POST", u.BucketName, keyName, h)

	boundry := makeBoundry()
	body, err := makeFormBody(authorization, boundry, keyName, mimeType, u.verifyUploadMD5, file)
	if err != nil {
		return err
	}
	//lastLine 一定要写，否则后端解析不到。
	lastLine := fmt.Sprintf("\r\n--%s--\r\n", boundry)
	body.Write([]byte(lastLine))

	reqURL := u.genFileURL("")
	req, err := http.NewRequest("POST", reqURL, body)
	if err != nil {
		return err
	}

	if u.verifyUploadMD5 {
		req.Header.Add("Content-MD5", md5Str)
	}

	req.Header.Add("Content-Type", "multipart/form-data; boundary="+boundry)
	contentLength := body.Len()
	req.Header.Add("Content-Length", strconv.Itoa(contentLength))
	for k, v := range u.RequestHeader {
		for i := 0; i < len(v); i++ {
			req.Header.Add(k, v[i])
		}
	}
	return u.request(req)
}

//PutFile 把文件直接放到 HTTP Body 里面上传，相对 PostFile 接口，这个要更简单，速度会更快（因为不用包装 form）。
//mimeType 如果为空的，会调用 net/http 里面的 DetectContentType 进行检测。
//keyName 表示传到 ufile 的文件名。
//小于 100M 的文件推荐使用本接口上传。
func (u *UFileRequest) PutFile(filePath, keyName, mimeType string) error {
	reqURL := u.genFileURL(keyName)
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	if mimeType == "" {
		mimeType = getMimeType(file)
	}
	req.Header.Add("Content-Type", mimeType)
	for k, v := range u.RequestHeader {
		for i := 0; i < len(v); i++ {
			req.Header.Add(k, v[i])
		}
	}

	if u.verifyUploadMD5 {
		md5Str := fmt.Sprintf("%x", md5.Sum(b))
		req.Header.Add("Content-MD5", md5Str)
	}

	authorization := u.Auth.Authorization("PUT", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	fileSize := getFileSize(file)
	req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))

	return u.request(req)
}

//PutFileWithIopString 支持上传iop, 直接指定iop字符串, 上传iop必须指定saveAs命令做持久化，否则图片处理不会生效
func (u *UFileRequest) PutFileWithIopString(filePath, keyName, mimeType string, iopcmd string) error {
	reqURL := u.genFileURL(keyName)
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	//增加iopcmd
	if iopcmd != "" {
		reqURL += "?" + iopcmd
	}

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	if mimeType == "" {
		mimeType = getMimeType(file)
	}
	req.Header.Add("Content-Type", mimeType)
	for k, v := range u.RequestHeader {
		for i := 0; i < len(v); i++ {
			req.Header.Add(k, v[i])
		}
	}

	if u.verifyUploadMD5 {
		md5Str := fmt.Sprintf("%x", md5.Sum(b))
		req.Header.Add("Content-MD5", md5Str)
	}

	authorization := u.Auth.Authorization("PUT", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	fileSize := getFileSize(file)
	req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))

	return u.request(req)
}

//PutFile 把文件直接放到 HTTP Body 里面上传，相对 PostFile 接口，这个要更简单，速度会更快（因为不用包装 form）。
//mimeType 如果为空的，会调用 net/http 里面的 DetectContentType 进行检测。
//keyName 表示传到 ufile 的文件名。
//小于 100M 的文件推荐使用本接口上传。
//支持带上传回调的参数, policy_json 为json 格式字符串
func (u *UFileRequest) PutFileWithPolicy(filePath, keyName, mimeType string, policy_json string) error {
	reqURL := u.genFileURL(keyName)
	file, err := openFile(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	if mimeType == "" {
		mimeType = getMimeType(file)
	}
	req.Header.Add("Content-Type", mimeType)

	if u.verifyUploadMD5 {
		md5Str := fmt.Sprintf("%x", md5.Sum(b))
		req.Header.Add("Content-MD5", md5Str)
	}

	policy := base64.URLEncoding.EncodeToString([]byte(policy_json))
	authorization := u.Auth.AuthorizationPolicy("PUT", u.BucketName, keyName, policy, req.Header)
	req.Header.Add("authorization", authorization)
	fileSize := getFileSize(file)
	req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))

	return u.request(req)
}


//DeleteFile 删除一个文件，如果删除成功 statuscode 会返回 204，否则会返回 404 表示文件不存在。
//keyName 表示传到 ufile 的文件名。
func (u *UFileRequest) DeleteFile(keyName string) error {
	reqURL := u.genFileURL(keyName)
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("DELETE", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//HeadFile 获取一个文件的基本信息，返回的信息全在 header 里面。包含 mimeType, content-length（文件大小）, etag, Last-Modified:。
//keyName 表示传到 ufile 的文件名。
func (u *UFileRequest) HeadFile(keyName string) error {
	reqURL := u.genFileURL(keyName)
	req, err := http.NewRequest("HEAD", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("HEAD", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//PrefixFileList 获取文件列表。
//prefix 表示匹配文件前缀。
//marker 标志字符串
//limit 列表数量限制，传 0 会默认设置为 20.
func (u *UFileRequest) PrefixFileList(prefix, marker string, limit int) (list FileListResponse, err error) {
	query := &url.Values{}
	query.Add("prefix", prefix)
	query.Add("marker", marker)
	if limit == 0 {
		limit = 20
	}
	query.Add("limit", strconv.Itoa(limit))
	reqURL := u.genFileURL("") + "?list&" + query.Encode()

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

//GetPublicURL 获取公有空间的文件下载 URL
//keyName 表示传到 ufile 的文件名。
func (u *UFileRequest) GetPublicURL(keyName string) string {
	return u.genFileURL(keyName)
}

//GetPrivateURL 获取私有空间的文件下载 URL。
//keyName 表示传到 ufile 的文件名。
//expiresDuation 表示下载链接的过期时间，从现在算起，24 * time.Hour 表示过期时间为一天。
func (u *UFileRequest) GetPrivateURL(keyName string, expiresDuation time.Duration) string {
	t := time.Now()
	t = t.Add(expiresDuation)
	expires := strconv.FormatInt(t.Unix(), 10)
	signature, publicKey := u.Auth.AuthorizationPrivateURL("GET", u.BucketName, keyName, expires, http.Header{})
	query := url.Values{}
	query.Add("UCloudPublicKey", publicKey)
	query.Add("Signature", signature)
	query.Add("Expires", expires)
	reqURL := u.genFileURL(keyName)
	return reqURL + "?" + query.Encode()
}

//Download 把文件下载到 HTTP Body 里面，这里只能用来下载小文件，建议使用 DownloadFile 来下载大文件。
func (u *UFileRequest) Download(reqURL string) error {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	return u.request(req)
}

//Download 文件下载接口, 对下载大文件比较友好；支持流式下载
func (u *UFileRequest) DownloadFile(writer io.Writer, keyName string) error {

	reqURL := u.GetPrivateURL(keyName, 24*time.Hour)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.requestWithResp(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	u.LastResponseStatus = resp.StatusCode
	u.LastResponseHeader = resp.Header
	u.LastResponseBody = nil //流式下载无body存储在u里
	u.lastResponse = resp
	if !VerifyHTTPCode(resp.StatusCode) {
		return fmt.Errorf("Remote response code is %d - %s not 2xx call DumpResponse(true) show details",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	size := u.LastResponseHeader.Get("Content-Length")
	fileSize, err := strconv.ParseInt(size, 10, 0)
	if err != nil || fileSize < 0 {
		return fmt.Errorf("Parse content-lengt returned error")
	}
	_, err = io.Copy(writer, resp.Body)
	return err
}

func (u *UFileRequest) DownloadFileRetRespBody(keyName string, offset int64) (io.ReadCloser, error) {
	reqURL := u.GetPrivateURL(keyName, 24*time.Hour)
	req, err := http.NewRequest("GET", reqURL, nil)
	req.Header.Add("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	if err != nil {
		return nil, err
	}

	resp, err := u.requestWithResp(req)
	if err != nil {
		return nil, err
	}

	u.LastResponseStatus = resp.StatusCode
	u.LastResponseHeader = resp.Header
	u.LastResponseBody = nil // 不要保存到内存！！！超过 128MB 的 body 会撑爆内存（其实似乎是因为 []byte 的最大容量为 128MB）
	u.lastResponse = resp

	if !VerifyHTTPCode(resp.StatusCode) {
		// 如果 req 出错，此时可以将 resp.Body 保存到内存里，因为 resp.Body 里就只有 RetCode ErrMsg 等信息
		resBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		u.LastResponseBody = resBody
		return nil, fmt.Errorf("Remote response code is %d - %s not 2xx call DumpResponse(true) show details",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	size := u.LastResponseHeader.Get("Content-Length")
	fileSize, err := strconv.ParseInt(size, 10, 0)
	if err != nil || fileSize < 0 {
		return nil, fmt.Errorf("Parse content-lengt returned error")
	}
	return resp.Body, nil
}

//DownloadFileWithIopString 支持下载iop，直接指定iop命令字符串
func (u *UFileRequest) DownloadFileWithIopString(writer io.Writer, keyName string, iopcmd string) error {

	reqURL := u.GetPrivateURL(keyName, 24*time.Hour)

	//增加iopcmd，因为获取到下载链接已经带了query，所以这里使用&连接
	if iopcmd != "" {
		reqURL += "&" + iopcmd
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.requestWithResp(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	u.LastResponseStatus = resp.StatusCode
	u.LastResponseHeader = resp.Header
	u.LastResponseBody = nil //流式下载无body存储在u里
	u.lastResponse = resp
	if !VerifyHTTPCode(resp.StatusCode) {
		return fmt.Errorf("Remote response code is %d - %s not 2xx call DumpResponse(true) show details",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	size := u.LastResponseHeader.Get("Content-Length")
	fileSize, err := strconv.ParseInt(size, 10, 0)
	if err != nil || fileSize < 0 {
		return fmt.Errorf("Parse content-lengt returned error")
	}
	_, err = io.Copy(writer, resp.Body)
	return err
}

//CompareFileEtag 检查远程文件的 etag 和本地文件的 etag 是否一致
func (u *UFileRequest) CompareFileEtag(remoteKeyName, localFilePath string) bool {
	err := u.HeadFile(remoteKeyName)
	if err != nil {
		return false
	}
	remoteEtag := strings.Trim(u.LastResponseHeader.Get("Etag"), "\"")
	localEtag := GetFileEtag(localFilePath)
	return remoteEtag == localEtag
}

func (u *UFileRequest) genFileURL(keyName string) string {
	return u.baseURL.String() + keyName
}

//Restore 用于解冻冷存类型的文件
func (u *UFileRequest) Restore(keyName string) (err error) {
	reqURL := u.genFileURL(keyName) + "?restore"
	req, err := http.NewRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("PUT", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//ClassSwitch 存储类型转换接口
//keyName 文件名称
//storageClass 所要转换的新文件存储类型，目前支持的类型分别是标准:"STANDARD"、低频:"IA"、冷存:"ARCHIVE"
func (u *UFileRequest) ClassSwitch(keyName string, storageClass string) (err error) {
	query := &url.Values{}
	query.Add("storageClass", storageClass)
	reqURL := u.genFileURL(keyName) + "?" + query.Encode()
	req, err := http.NewRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("PUT", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//Rename 重命名指定文件
//keyName 需要被重命名的源文件
//newKeyName 修改后的新文件名
//force 如果已存在同名文件，值为"true"则覆盖，否则会操作失败
func (u *UFileRequest) Rename(keyName, newKeyName, force string) (err error) {

	query := url.Values{}
	query.Add("newFileName", newKeyName)
	query.Add("force", force)
	reqURL := u.genFileURL(keyName) + "?" + query.Encode()

	req, err := http.NewRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	authorization := u.Auth.Authorization("PUT", u.BucketName, keyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//Copy 从同组织下的源Bucket中拷贝指定文件到目的Bucket中，并以新文件名命名
//dstkeyName 拷贝到目的Bucket后的新文件名
//srcBucketName 待拷贝文件所在的源Bucket名称
//srcKeyName 待拷贝文件名称
func (u *UFileRequest) Copy(dstkeyName, srcBucketName, srcKeyName string) (err error) {

	reqURL := u.genFileURL(dstkeyName)

	req, err := http.NewRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Add("X-Ufile-Copy-Source", "/" + srcBucketName + "/" + srcKeyName)

	authorization := u.Auth.Authorization("PUT", u.BucketName, dstkeyName, req.Header)
	req.Header.Add("authorization", authorization)
	return u.request(req)
}

//ListObjects 获取目录文件列表。
//prefix 返回以Prefix作为前缀的目录文件列表
//marker 返回以字母排序后，大于Marker的目录文件列表
//delimiter 目录分隔符，当前只支持"/"和""，当Delimiter设置为"/"时，返回目录形式的文件列表，当Delimiter设置为""时，返回非目录层级文件列表
//maxkeys 指定返回目录文件列表的最大数量，默认值为100
func (u *UFileRequest) ListObjects(prefix, marker, delimiter string, maxkeys int) (list ListObjectsResponse, err error) {
	query := &url.Values{}
	query.Add("prefix", prefix)
	query.Add("marker", marker)
	query.Add("delimiter", delimiter)
	if maxkeys == 0 {
		maxkeys = 100
	}
	query.Add("max-keys", strconv.Itoa(maxkeys))
	reqURL := u.genFileURL("") + "?listobjects&" + query.Encode()

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
