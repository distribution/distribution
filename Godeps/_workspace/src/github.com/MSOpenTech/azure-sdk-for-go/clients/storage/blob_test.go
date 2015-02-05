package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const testContainerPrefix = "zzzztest-"

func Test_pathForContainer(t *testing.T) {
	out := pathForContainer("foo")
	if expected := "/foo"; out != expected {
		t.Errorf("Wrong pathForContainer. Expected: '%s', got: '%s'", expected, out)
	}
}

func Test_pathForBlob(t *testing.T) {
	out := pathForBlob("foo", "blob")
	if expected := "/foo/blob"; out != expected {
		t.Errorf("Wrong pathForBlob. Expected: '%s', got: '%s'", expected, out)
	}
}

func Test_blobSASStringToSign(t *testing.T) {
	_, err := blobSASStringToSign("2012-02-12", "CS", "SE", "SP")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	out, err := blobSASStringToSign("2013-08-15", "CS", "SE", "SP")
	if err != nil {
		t.Fatal(err)
	}
	if expected := "SP\n\nSE\nCS\n\n2013-08-15\n\n\n\n\n"; out != expected {
		t.Errorf("Wrong stringToSign. Expected: '%s', got: '%s'", expected, out)
	}
}

func TestGetBlobSASURI(t *testing.T) {
	api, err := NewClient("foo", "YmFy", DefaultBaseUrl, "2013-08-15", true)
	if err != nil {
		t.Fatal(err)
	}
	cli := api.GetBlobService()
	expiry := time.Time{}

	expectedParts := url.URL{
		Scheme: "https",
		Host:   "foo.blob.core.windows.net",
		Path:   "container/name",
		RawQuery: url.Values{
			"sv":  {"2013-08-15"},
			"sig": {"/OXG7rWh08jYwtU03GzJM0DHZtidRGpC6g69rSGm3I0="},
			"sr":  {"b"},
			"sp":  {"r"},
			"se":  {"0001-01-01T00:00:00Z"},
		}.Encode()}

	u, err := cli.GetBlobSASURI("container", "name", expiry, "r")
	if err != nil {
		t.Fatal(err)
	}
	sasParts, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}

	expectedQuery := expectedParts.Query()
	sasQuery := sasParts.Query()

	expectedParts.RawQuery = "" // reset
	sasParts.RawQuery = ""

	if expectedParts.String() != sasParts.String() {
		t.Fatalf("Base URL wrong for SAS. Expected: '%s', got: '%s'", expectedParts, sasParts)
	}

	if len(expectedQuery) != len(sasQuery) {
		t.Fatalf("Query string wrong for SAS URL. Expected: '%d keys', got: '%d keys'", len(expectedQuery), len(sasQuery))
	}

	for k, v := range expectedQuery {
		out, ok := sasQuery[k]
		if !ok {
			t.Fatalf("Query parameter '%s' not found in generated SAS query. Expected: '%s'", k, v)
		}
		if !reflect.DeepEqual(v, out) {
			t.Fatalf("Wrong value for query parameter '%s'. Expected: '%s', got: '%s'", k, v, out)
		}
	}
}

func TestBlobSASURICorrectness(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}
	cnt := randContainer()
	blob := randString(20)
	body := []byte(randString(100))
	expiry := time.Now().UTC().Add(time.Hour)
	permissions := "r"

	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	err = cli.PutBlockBlob(cnt, blob, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	sasUri, err := cli.GetBlobSASURI(cnt, blob, expiry, permissions)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(sasUri)
	if err != nil {
		t.Logf("SAS URI: %s", sasUri)
		t.Fatal(err)
	}

	blobResp, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Non-ok status code: %s", resp.Status)
	}

	if len(blobResp) != len(body) {
		t.Fatalf("Wrong blob size on SAS URI. Expected: %d, Got: %d", len(body), len(blobResp))
	}
}

func TestListContainersPagination(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = deleteTestContainers(cli)
	if err != nil {
		t.Fatal(err)
	}

	const n = 5
	const pageSize = 2

	// Create test containers
	created := []string{}
	for i := 0; i < n; i++ {
		name := randContainer()
		err := cli.CreateContainer(name, ContainerAccessTypePrivate)
		if err != nil {
			t.Fatalf("Error creating test container: %s", err)
		}
		created = append(created, name)
	}
	sort.Strings(created)

	// Defer test container deletions
	defer func() {
		var wg sync.WaitGroup
		for _, cnt := range created {
			wg.Add(1)
			go func(name string) {
				err := cli.DeleteContainer(name)
				if err != nil {
					t.Logf("Error while deleting test container: %s", err)
				}
				wg.Done()
			}(cnt)
		}
		wg.Wait()
	}()

	// Paginate results
	seen := []string{}
	marker := ""
	for {
		resp, err := cli.ListContainers(ListContainersParameters{
			Prefix:     testContainerPrefix,
			MaxResults: pageSize,
			Marker:     marker})

		if err != nil {
			t.Fatal(err)
		}

		containers := resp.Containers

		if len(containers) > pageSize {
			t.Fatalf("Got a bigger page. Expected: %d, got: %d", pageSize, len(containers))
		}

		for _, c := range containers {
			seen = append(seen, c.Name)
		}

		marker = resp.NextMarker
		if marker == "" || len(containers) == 0 {
			break
		}
	}

	// Compare
	if !reflect.DeepEqual(created, seen) {
		t.Fatalf("Wrong pagination results:\nExpected:\t\t%v\nGot:\t\t%v", created, seen)
	}
}

func TestContainerExists(t *testing.T) {
	cnt := randContainer()

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	ok, err := cli.ContainerExists(cnt)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("Non-existing container returned as existing: %s", cnt)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypeBlob)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	ok, err = cli.ContainerExists(cnt)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("Existing container returned as non-existing: %s", cnt)
	}
}

func TestCreateDeleteContainer(t *testing.T) {
	cnt := randContainer()

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	err = cli.DeleteContainer(cnt)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateContainerIfNotExists(t *testing.T) {
	cnt := randContainer()

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	// First create
	ok, err := cli.CreateContainerIfNotExists(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	if expected := true; ok != expected {
		t.Fatalf("Wrong creation status. Expected: %v; Got: %v", expected, ok)
	}

	// Second create, should not give errors
	ok, err = cli.CreateContainerIfNotExists(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	if expected := false; ok != expected {
		t.Fatalf("Wrong creation status. Expected: %v; Got: %v", expected, ok)
	}

	defer cli.DeleteContainer(cnt)
}

func TestDeleteContainerIfExists(t *testing.T) {
	cnt := randContainer()

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	// Nonexisting container
	err = cli.DeleteContainer(cnt)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	ok, err := cli.DeleteContainerIfExists(cnt)
	if err != nil {
		t.Fatalf("Not supposed to return error, got: %s", err)
	}
	if expected := false; ok != expected {
		t.Fatalf("Wrong deletion status. Expected: %v; Got: %v", expected, ok)
	}

	// Existing container
	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	ok, err = cli.DeleteContainerIfExists(cnt)
	if err != nil {
		t.Fatalf("Not supposed to return error, got: %s", err)
	}
	if expected := true; ok != expected {
		t.Fatalf("Wrong deletion status. Expected: %v; Got: %v", expected, ok)
	}
}

func TestBlobExists(t *testing.T) {
	cnt := randContainer()
	blob := randString(20)

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypeBlob)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)
	err = cli.PutBlockBlob(cnt, blob, strings.NewReader("Hello!"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, blob)

	ok, err := cli.BlobExists(cnt, blob+".foo")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("Non-existing blob returned as existing: %s/%s", cnt, blob)
	}

	ok, err = cli.BlobExists(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("Existing blob returned as non-existing: %s/%s", cnt, blob)
	}
}

func TestGetBlobUrl(t *testing.T) {
	api, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}
	cli := api.GetBlobService()

	out := cli.GetBlobUrl("c", "nested/blob")
	if expected := "https://foo.blob.core.windows.net/c/nested/blob"; out != expected {
		t.Fatalf("Wrong blob URL. Expected: '%s', got:'%s'", expected, out)
	}

	out = cli.GetBlobUrl("", "blob")
	if expected := "https://foo.blob.core.windows.net/$root/blob"; out != expected {
		t.Fatalf("Wrong blob URL. Expected: '%s', got:'%s'", expected, out)
	}

	out = cli.GetBlobUrl("", "nested/blob")
	if expected := "https://foo.blob.core.windows.net/$root/nested/blob"; out != expected {
		t.Fatalf("Wrong blob URL. Expected: '%s', got:'%s'", expected, out)
	}
}

func TestBlobCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping blob copy in short mode, no SLA on async operation")
	}

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	src := randString(20)
	dst := randString(20)
	body := []byte(randString(1024))

	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	err = cli.PutBlockBlob(cnt, src, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, src)

	err = cli.CopyBlob(cnt, dst, cli.GetBlobUrl(cnt, src))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, dst)

	blobBody, err := cli.GetBlob(cnt, dst)
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(blobBody)
	defer blobBody.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(body, b) {
		t.Fatalf("Copied blob is wrong. Expected: %d bytes, got: %d bytes\n%s\n%s", len(body), len(b), body, b)
	}
}

func TestDeleteBlobIfExists(t *testing.T) {
	cnt := randContainer()
	blob := randString(20)

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.DeleteBlob(cnt, blob)
	if err == nil {
		t.Fatal("Nonexisting blob did not return error")
	}

	ok, err := cli.DeleteBlobIfExists(cnt, blob)
	if err != nil {
		t.Fatalf("Not supposed to return error: %s", err)
	}
	if expected := false; ok != expected {
		t.Fatalf("Wrong deletion status. Expected: %v; Got: %v", expected, ok)
	}
}

func TestGetBlobProperties(t *testing.T) {
	cnt := randContainer()
	blob := randString(20)
	contents := randString(64)

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	// Nonexisting blob
	_, err = cli.GetBlobProperties(cnt, blob)
	if err == nil {
		t.Fatal("Did not return error for non-existing blob")
	}

	// Put the blob
	err = cli.PutBlockBlob(cnt, blob, strings.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}

	// Get blob properties
	props, err := cli.GetBlobProperties(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}

	if props.ContentLength != int64(len(contents)) {
		t.Fatalf("Got wrong Content-Length: '%d', expected: %d", props.ContentLength, len(contents))
	}
	if props.BlobType != BlobTypeBlock {
		t.Fatalf("Got wrong BlobType. Expected:'%s', got:'%s'", BlobTypeBlock, props.BlobType)
	}
}

func TestListBlobsPagination(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	err = cli.CreateContainer(cnt, ContainerAccessTypePrivate)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	blobs := []string{}
	const n = 5
	const pageSize = 2
	for i := 0; i < n; i++ {
		name := randString(20)
		err := cli.PutBlockBlob(cnt, name, strings.NewReader("Hello, world!"))
		if err != nil {
			t.Fatal(err)
		}
		blobs = append(blobs, name)
	}
	sort.Strings(blobs)

	// Paginate
	seen := []string{}
	marker := ""
	for {
		resp, err := cli.ListBlobs(cnt, ListBlobsParameters{
			MaxResults: pageSize,
			Marker:     marker})
		if err != nil {
			t.Fatal(err)
		}

		for _, v := range resp.Blobs {
			seen = append(seen, v.Name)
		}

		marker = resp.NextMarker
		if marker == "" || len(resp.Blobs) == 0 {
			break
		}
	}

	// Compare
	if !reflect.DeepEqual(blobs, seen) {
		t.Fatalf("Got wrong list of blobs. Expected: %s, Got: %s", blobs, seen)
	}

	err = cli.DeleteContainer(cnt)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPutEmptyBlockBlob(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	err = cli.PutBlockBlob(cnt, blob, bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatal(err)
	}

	props, err := cli.GetBlobProperties(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}
	if props.ContentLength != 0 {
		t.Fatalf("Wrong content length for empty blob: %d", props.ContentLength)
	}
}

func TestPutSingleBlockBlob(t *testing.T) {
	cnt := randContainer()
	blob := randString(20)
	body := []byte(randString(1024))

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypeBlob)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	err = cli.PutBlockBlob(cnt, blob, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, blob)

	resp, err := cli.GetBlob(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}

	// Verify contents
	respBody, err := ioutil.ReadAll(resp)
	defer resp.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(body, respBody) {
		t.Fatalf("Wrong blob contents.\nExpected: %d bytes, Got: %d byes", len(body), len(respBody))
	}

	// Verify block list
	blocks, err := cli.GetBlockList(cnt, blob, BlockListTypeAll)
	if err != nil {
		t.Fatal(err)
	}
	if expected := 1; len(blocks.CommittedBlocks) != expected {
		t.Fatalf("Wrong committed block count. Expected: %d, Got: %d", expected, len(blocks.CommittedBlocks))
	}
	if expected := 0; len(blocks.UncommittedBlocks) != expected {
		t.Fatalf("Wrong unccommitted block count. Expected: %d, Got: %d", expected, len(blocks.UncommittedBlocks))
	}
	thatBlock := blocks.CommittedBlocks[0]
	if expected := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%011d", 0))); thatBlock.Name != expected {
		t.Fatalf("Wrong block name. Expected: %s, Got: %s", expected, thatBlock.Name)
	}
}

func TestGetBlobRange(t *testing.T) {
	cnt := randContainer()
	blob := randString(20)
	body := "0123456789"

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypeBlob)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	err = cli.PutBlockBlob(cnt, blob, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, blob)

	// Read 1-3
	for _, r := range []struct {
		rangeStr string
		expected string
	}{
		{"0-", body},
		{"1-3", body[1 : 3+1]},
		{"3-", body[3:]},
	} {
		resp, err := cli.GetBlobRange(cnt, blob, r.rangeStr)
		if err != nil {
			t.Fatal(err)
		}
		blobBody, err := ioutil.ReadAll(resp)
		if err != nil {
			t.Fatal(err)
		}
		str := string(blobBody)
		if str != r.expected {
			t.Fatalf("Got wrong range. Expected: '%s'; Got:'%s'", r.expected, str)
		}
	}
}

func TestPutBlock(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	chunk := []byte(randString(1024))
	blockId := base64.StdEncoding.EncodeToString([]byte("foo"))
	err = cli.PutBlock(cnt, blob, blockId, chunk)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPutMultiBlockBlob(t *testing.T) {
	var (
		cnt       = randContainer()
		blob      = randString(20)
		blockSize = 32 * 1024                                     // 32 KB
		body      = []byte(randString(blockSize*2 + blockSize/2)) // 3 blocks
	)

	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	err = cli.CreateContainer(cnt, ContainerAccessTypeBlob)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteContainer(cnt)

	err = cli.putBlockBlob(cnt, blob, bytes.NewReader(body), blockSize)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.DeleteBlob(cnt, blob)

	resp, err := cli.GetBlob(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}

	// Verify contents
	respBody, err := ioutil.ReadAll(resp)
	defer resp.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body, respBody) {
		t.Fatalf("Wrong blob contents.\nExpected: %d bytes, Got: %d byes", len(body), len(respBody))
	}

	err = cli.DeleteBlob(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}

	err = cli.DeleteContainer(cnt)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetBlockList_PutBlockList(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	chunk := []byte(randString(1024))
	blockId := base64.StdEncoding.EncodeToString([]byte("foo"))

	// Put one block
	err = cli.PutBlock(cnt, blob, blockId, chunk)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.deleteBlob(cnt, blob)

	// Get committed blocks
	committed, err := cli.GetBlockList(cnt, blob, BlockListTypeCommitted)
	if err != nil {
		t.Fatal(err)
	}

	if len(committed.CommittedBlocks) > 0 {
		t.Fatal("There are committed blocks")
	}

	// Get uncommitted blocks
	uncommitted, err := cli.GetBlockList(cnt, blob, BlockListTypeUncommitted)
	if err != nil {
		t.Fatal(err)
	}

	if expected := 1; len(uncommitted.UncommittedBlocks) != expected {
		t.Fatalf("Uncommitted blocks wrong. Expected: %d, got: %d", expected, len(uncommitted.UncommittedBlocks))
	}

	// Commit block list
	err = cli.PutBlockList(cnt, blob, []Block{{blockId, BlockStatusUncommitted}})
	if err != nil {
		t.Fatal(err)
	}

	// Get all blocks
	all, err := cli.GetBlockList(cnt, blob, BlockListTypeAll)
	if err != nil {
		t.Fatal(err)
	}

	if expected := 1; len(all.CommittedBlocks) != expected {
		t.Fatalf("Uncommitted blocks wrong. Expected: %d, got: %d", expected, len(uncommitted.CommittedBlocks))
	}
	if expected := 0; len(all.UncommittedBlocks) != expected {
		t.Fatalf("Uncommitted blocks wrong. Expected: %d, got: %d", expected, len(uncommitted.UncommittedBlocks))
	}

	// Verify the block
	thatBlock := all.CommittedBlocks[0]
	if expected := blockId; expected != thatBlock.Name {
		t.Fatalf("Wrong block name. Expected: %s, got: %s", expected, thatBlock.Name)
	}
	if expected := int64(len(chunk)); expected != thatBlock.Size {
		t.Fatalf("Wrong block name. Expected: %d, got: %d", expected, thatBlock.Size)
	}
}

func TestCreateBlockBlob(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	if err := cli.CreateBlockBlob(cnt, blob); err != nil {
		t.Fatal(err)
	}

	// Verify
	blocks, err := cli.GetBlockList(cnt, blob, BlockListTypeAll)
	if err != nil {
		t.Fatal(err)
	}
	if expected, got := 0, len(blocks.CommittedBlocks); expected != got {
		t.Fatalf("Got wrong committed block count. Expected: %v, Got:%v ", expected, got)
	}
	if expected, got := 0, len(blocks.UncommittedBlocks); expected != got {
		t.Fatalf("Got wrong uncommitted block count. Expected: %v, Got:%v ", expected, got)
	}
}

func TestPutPageBlob(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	size := int64(10 * 1024 * 1024)
	if err := cli.PutPageBlob(cnt, blob, size); err != nil {
		t.Fatal(err)
	}

	// Verify
	props, err := cli.GetBlobProperties(cnt, blob)
	if err != nil {
		t.Fatal(err)
	}
	if expected := size; expected != props.ContentLength {
		t.Fatalf("Got wrong Content-Length. Expected: %v, Got:%v ", expected, props.ContentLength)
	}
	if expected := BlobTypePage; expected != props.BlobType {
		t.Fatalf("Got wrong x-ms-blob-type. Expected: %v, Got:%v ", expected, props.BlobType)
	}
}

func TestPutPagesUpdate(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	size := int64(10 * 1024 * 1024) // larger than we'll use
	if err := cli.PutPageBlob(cnt, blob, size); err != nil {
		t.Fatal(err)
	}

	chunk1 := []byte(randString(1024))
	chunk2 := []byte(randString(512))
	// Append chunks
	if err := cli.PutPage(cnt, blob, 0, int64(len(chunk1)-1), PageWriteTypeUpdate, chunk1); err != nil {
		t.Fatal(err)
	}
	if err := cli.PutPage(cnt, blob, int64(len(chunk1)), int64(len(chunk1)+len(chunk2)-1), PageWriteTypeUpdate, chunk2); err != nil {
		t.Fatal(err)
	}

	// Verify contents
	out, err := cli.GetBlobRange(cnt, blob, fmt.Sprintf("%v-%v", 0, len(chunk1)+len(chunk2)))
	if err != nil {
		t.Fatal(err)
	}
	blobContents, err := ioutil.ReadAll(out)
	defer out.Close()
	if err != nil {
		t.Fatal(err)
	}
	if expected := append(chunk1, chunk2...); reflect.DeepEqual(blobContents, expected) {
		t.Fatalf("Got wrong blob.\nGot:%d bytes, Expected:%d bytes", len(blobContents), len(expected))
	}
	out.Close()

	// Overwrite first half of chunk1
	chunk0 := []byte(randString(512))
	if err := cli.PutPage(cnt, blob, 0, int64(len(chunk0)-1), PageWriteTypeUpdate, chunk0); err != nil {
		t.Fatal(err)
	}

	// Verify contents
	out, err = cli.GetBlobRange(cnt, blob, fmt.Sprintf("%v-%v", 0, len(chunk1)+len(chunk2)))
	if err != nil {
		t.Fatal(err)
	}
	blobContents, err = ioutil.ReadAll(out)
	defer out.Close()
	if err != nil {
		t.Fatal(err)
	}
	if expected := append(append(chunk0, chunk1[512:]...), chunk2...); reflect.DeepEqual(blobContents, expected) {
		t.Fatalf("Got wrong blob.\nGot:%d bytes, Expected:%d bytes", len(blobContents), len(expected))
	}
}

func TestPutPagesClear(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	size := int64(10 * 1024 * 1024) // larger than we'll use

	if err := cli.PutPageBlob(cnt, blob, size); err != nil {
		t.Fatal(err)
	}

	// Put 0-2047
	chunk := []byte(randString(2048))
	if err := cli.PutPage(cnt, blob, 0, 2047, PageWriteTypeUpdate, chunk); err != nil {
		t.Fatal(err)
	}

	// Clear 512-1023
	if err := cli.PutPage(cnt, blob, 512, 1023, PageWriteTypeClear, nil); err != nil {
		t.Fatal(err)
	}

	// Get blob contents
	if out, err := cli.GetBlobRange(cnt, blob, "0-2048"); err != nil {
		t.Fatal(err)
	} else {
		contents, err := ioutil.ReadAll(out)
		defer out.Close()
		if err != nil {
			t.Fatal(err)
		}

		if expected := append(append(chunk[:512], make([]byte, 512)...), chunk[1024:]...); reflect.DeepEqual(contents, expected) {
			t.Fatalf("Cleared blob is not the same. Expected: (%d) %v; got: (%d) %v", len(expected), expected, len(contents), contents)
		}
	}
}

func TestGetPageRanges(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	cnt := randContainer()
	if err := cli.CreateContainer(cnt, ContainerAccessTypePrivate); err != nil {
		t.Fatal(err)
	}
	defer cli.deleteContainer(cnt)

	blob := randString(20)
	size := int64(10 * 1024 * 1024) // larger than we'll use

	if err := cli.PutPageBlob(cnt, blob, size); err != nil {
		t.Fatal(err)
	}

	// Get page ranges on empty blob
	if out, err := cli.GetPageRanges(cnt, blob); err != nil {
		t.Fatal(err)
	} else if len(out.PageList) != 0 {
		t.Fatal("Blob has pages")
	}

	// Add 0-512 page
	err = cli.PutPage(cnt, blob, 0, 511, PageWriteTypeUpdate, []byte(randString(512)))
	if err != nil {
		t.Fatal(err)
	}

	if out, err := cli.GetPageRanges(cnt, blob); err != nil {
		t.Fatal(err)
	} else if expected := 1; len(out.PageList) != expected {
		t.Fatalf("Expected %d pages, got: %d -- %v", expected, len(out.PageList), out.PageList)
	}

	// Add 1024-2048
	err = cli.PutPage(cnt, blob, 1024, 2047, PageWriteTypeUpdate, []byte(randString(1024)))
	if err != nil {
		t.Fatal(err)
	}

	if out, err := cli.GetPageRanges(cnt, blob); err != nil {
		t.Fatal(err)
	} else if expected := 2; len(out.PageList) != expected {
		t.Fatalf("Expected %d pages, got: %d -- %v", expected, len(out.PageList), out.PageList)
	}
}

func deleteTestContainers(cli *BlobStorageClient) error {
	for {
		resp, err := cli.ListContainers(ListContainersParameters{Prefix: testContainerPrefix})
		if err != nil {
			return err
		}
		if len(resp.Containers) == 0 {
			break
		}
		for _, c := range resp.Containers {
			err = cli.DeleteContainer(c.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getBlobClient() (*BlobStorageClient, error) {
	name := os.Getenv("ACCOUNT_NAME")
	if name == "" {
		return nil, errors.New("ACCOUNT_NAME not set, need an empty storage account to test")
	}
	key := os.Getenv("ACCOUNT_KEY")
	if key == "" {
		return nil, errors.New("ACCOUNT_KEY not set")
	}
	cli, err := NewBasicClient(name, key)
	if err != nil {
		return nil, err
	}
	return cli.GetBlobService(), nil
}

func randContainer() string {
	return testContainerPrefix + randString(32-len(testContainerPrefix))
}

func randString(n int) string {
	if n <= 0 {
		panic("negative number")
	}
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}
