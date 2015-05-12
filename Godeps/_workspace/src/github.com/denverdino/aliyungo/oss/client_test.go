package oss_test

import (
	"bytes"
	"io/ioutil"
	//"net/http"
	"testing"
	"time"

	"github.com/denverdino/aliyungo/oss"
)

var (
	//If you test on ECS, you can set the internal param to true
	client = oss.NewOSSClient(TestRegion, false, TestAccessKeyId, TestAccessKeySecret, false)
)

func TestCreateBucket(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.PutBucket(oss.Private)
	if err != nil {
		t.Errorf("Failed for PutBucket: %v", err)
	}
	t.Log("Wait a while for bucket creation ...")
	time.Sleep(10 * time.Second)
}

func TestHead(t *testing.T) {

	b := client.Bucket(TestBucket)
	_, err := b.Head("name", nil)

	if err == nil {
		t.Errorf("Failed for Head: %v", err)
	}
}

func TestPutObject(t *testing.T) {
	const DISPOSITION = "attachment; filename=\"0x1a2b3c.jpg\""

	b := client.Bucket(TestBucket)
	err := b.Put("name", []byte("content"), "content-type", oss.Private, oss.Options{ContentDisposition: DISPOSITION})
	if err != nil {
		t.Errorf("Failed for Put: %v", err)
	}
}

func TestGet(t *testing.T) {

	b := client.Bucket(TestBucket)
	data, err := b.Get("name")

	if err != nil || string(data) != "content" {
		t.Errorf("Failed for Get: %v", err)
	}
}

func TestURL(t *testing.T) {

	b := client.Bucket(TestBucket)
	url := b.URL("name")

	t.Log("URL: ", url)
	//	/c.Assert(req.URL.Path, check.Equals, "/denverdino_test/name")
}

func TestGetReader(t *testing.T) {

	b := client.Bucket(TestBucket)
	rc, err := b.GetReader("name")
	if err != nil {
		t.Fatalf("Failed for GetReader: %v", err)
	}
	data, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil || string(data) != "content" {
		t.Errorf("Failed for ReadAll: %v", err)
	}
}

func aTestGetNotFound(t *testing.T) {

	b := client.Bucket("non-existent-bucket")
	_, err := b.Get("non-existent")
	if err == nil {
		t.Fatalf("Failed for TestGetNotFound: %v", err)
	}
	ossErr, _ := err.(*oss.Error)
	if ossErr.StatusCode != 404 || ossErr.BucketName != "non-existent-bucket" {
		t.Errorf("Failed for TestGetNotFound: %v", err)
	}

}

func TestPutCopy(t *testing.T) {
	b := client.Bucket(TestBucket)
	t.Log("Source: ", b.Path("name"))
	res, err := b.PutCopy("newname", oss.Private, oss.CopyOptions{},
		b.Path("name"))
	if err == nil {
		t.Logf("Copy result: %v", res)
	} else {
		t.Errorf("Failed for PutCopy: %v", err)
	}
}

func TestList(t *testing.T) {

	b := client.Bucket(TestBucket)

	data, err := b.List("n", "", "", 0)
	if err != nil || len(data.Contents) != 2 {
		t.Errorf("Failed for List: %v", err)
	} else {
		t.Logf("Contents = %++v", data)
	}
}

func TestListWithDelimiter(t *testing.T) {

	b := client.Bucket(TestBucket)

	data, err := b.List("photos/2006/", "/", "some-marker", 1000)
	if err != nil || len(data.Contents) != 0 {
		t.Errorf("Failed for List: %v", err)
	} else {
		t.Logf("Contents = %++v", data)
	}

}

func TestPutReader(t *testing.T) {

	b := client.Bucket(TestBucket)
	buf := bytes.NewBufferString("content")
	err := b.PutReader("name", buf, int64(buf.Len()), "content-type", oss.Private, oss.Options{})
	if err != nil {
		t.Errorf("Failed for PutReader: %v", err)
	}
	TestGetReader(t)
}

func TestExists(t *testing.T) {

	b := client.Bucket(TestBucket)
	result, err := b.Exists("name")
	if err != nil || result != true {
		t.Errorf("Failed for Exists: %v", err)
	}
}

func TestLocation(t *testing.T) {
	b := client.Bucket(TestBucket)
	result, err := b.Location()

	if err != nil || result != string(TestRegion) {
		t.Errorf("Failed for Location: %v %s", err, result)
	}
}

func TestACL(t *testing.T) {
	b := client.Bucket(TestBucket)
	result, err := b.ACL()

	if err != nil {
		t.Errorf("Failed for ACL: %v", err)
	} else {
		t.Logf("AccessControlPolicy: %++v", result)
	}
}

func TestDelObject(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.Del("name")
	if err != nil {
		t.Errorf("Failed for Del: %v", err)
	}
}

func TestDelMultiObjects(t *testing.T) {

	b := client.Bucket(TestBucket)
	objects := []oss.Object{oss.Object{Key: "newname"}}
	err := b.DelMulti(oss.Delete{
		Quiet:   false,
		Objects: objects,
	})
	if err != nil {
		t.Errorf("Failed for DelMulti: %v", err)
	}
}

func TestGetService(t *testing.T) {
	bucketList, err := client.GetService()
	if err != nil {
		t.Errorf("Unable to get service: %v", err)
	} else {
		t.Logf("GetService: %++v", bucketList)
	}
}

func TestDelBucket(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.DelBucket()
	if err != nil {
		t.Errorf("Failed for DelBucket: %v", err)
	}
}
