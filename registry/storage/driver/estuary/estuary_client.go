package estuary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type EstuaryClient struct {
	baseUrl    string
	shuttleUrl string
	token      string
	client     *http.Client
}

func NewEstuaryClient(baseUrl, shuttleUrl, token string) *EstuaryClient {
	return &EstuaryClient{
		baseUrl:    baseUrl,
		shuttleUrl: shuttleUrl,
		token:      token,
		client:     &http.Client{},
	}
}

func (e *EstuaryClient) AddContentToCollection(fp, collectionId, collectionPath string) (string, error) {
	url := e.shuttleUrl + "/content/add"
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	file, err := os.Open(fp)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer file.Close()
	part1, errFile1 := writer.CreateFormFile("data", filepath.Base(fp))
	_, err = io.Copy(part1, file)
	if err != nil {
		fmt.Println(errFile1)
		return "", nil
	}

	name := filepath.Base(fp)

	_ = writer.WriteField("name", name)
	_ = writer.WriteField("collection", collectionId)
	_ = writer.WriteField("collectionPath", collectionPath)
	err = writer.Close()
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return "", err
	}
	req.Header.Add("Transfer-Encoding", "chunked")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", e.token))

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := e.client.Do(req)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	fmt.Println(string(body))
	return string(body), nil
}

func NewSHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func (e *EstuaryClient) AddContent(contents []byte) (AddContentResponse, error) {
	url := e.shuttleUrl + "/content/add"
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	h := NewSHA256(contents)
	digest := hex.EncodeToString(h)

	contentWriter, err := writer.CreateFormFile("data", digest)
	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, nil
	}
	_, err = contentWriter.Write(contents)
	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, nil
	}

	err = writer.Close()
	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, err
	}

	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, err
	}
	req.Header.Add("Transfer-Encoding", "chunked")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", e.token))

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := e.client.Do(req)
	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return AddContentResponse{}, err
	}

	var addContentResponse AddContentResponse
	json.Unmarshal(body, &addContentResponse)
	return addContentResponse, nil
}

func (e *EstuaryClient) CreateCollection(name, description string) (string, error) {
	url := e.shuttleUrl + "/collections/create"
	method := "POST"

	values := map[string]string{"name": name, "description": description}
	jsonValue, _ := json.Marshal(values)

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonValue))

	if err != nil {
		fmt.Println(err)
		return "", err
	}
	req.Header.Add("Transfer-Encoding", "chunked")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", e.token))
	req.Header.Add("Content-Type", "application/json")

	res, err := e.client.Do(req)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	fmt.Println(string(body))
	return string(body), nil
}

func (e *EstuaryClient) GetContentByCid(cid string) (ContentElement, error) {

	url := e.baseUrl + "/content/by-cid/" + cid
	method := "GET"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("", "")
	err := writer.Close()
	if err != nil {
		fmt.Println(err)
		return ContentElement{}, err
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return ContentElement{}, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", e.token))
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return ContentElement{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return ContentElement{}, err
	}

	var contentElements []ContentElement
	json.Unmarshal(body, &contentElements)

	return contentElements[0], nil
}

func (e *EstuaryClient) GetContentByName(name string) (PinnedElement, error) {

	url := e.baseUrl + "/pinning/pins?name=" + name
	method := "GET"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("", "")
	err := writer.Close()
	if err != nil {
		fmt.Println(err)
		return PinnedElement{}, err
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return PinnedElement{}, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", e.token))
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return PinnedElement{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return PinnedElement{}, err
	}

	var pinningResponse PinningResponse
	json.Unmarshal(body, &pinningResponse)

	if pinningResponse.Count == 0 {
		return PinnedElement{}, fmt.Errorf("no pinning results for name: %s", name)
	}

	return pinningResponse.Results[0], nil
}
